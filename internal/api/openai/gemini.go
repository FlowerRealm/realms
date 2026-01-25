package openai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"realms/internal/auth"
	"realms/internal/middleware"
	"realms/internal/quota"
	"realms/internal/scheduler"
	"realms/internal/store"
)

type geminiModelItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

// GeminiModels 提供 Gemini Models 兼容入口：GET /v1beta/models。
func (h *Handler) GeminiModels(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	if h.models == nil {
		http.Error(w, "服务未配置模型目录", http.StatusBadGateway)
		return
	}

	ms, err := h.models.ListEnabledManagedModelsWithBindings(r.Context())
	if err != nil {
		http.Error(w, "查询模型目录失败", http.StatusBadGateway)
		return
	}

	items := make([]geminiModelItem, 0, len(ms))
	for _, m := range ms {
		if strings.TrimSpace(m.PublicID) == "" {
			continue
		}
		items = append(items, geminiModelItem{
			Name:        m.PublicID,
			DisplayName: m.PublicID,
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"models":        items,
		"nextPageToken": nil,
	})
}

// GeminiProxy 提供 Gemini 兼容入口：POST /v1beta/models/*path。
func (h *Handler) GeminiProxy(w http.ResponseWriter, r *http.Request) {
	reqStart := time.Now()
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}

	pathTail := strings.TrimSpace(r.PathValue("path"))
	if pathTail == "" {
		http.Error(w, "模型路径为空", http.StatusBadRequest)
		return
	}
	publicModel := strings.TrimSpace(strings.SplitN(pathTail, ":", 2)[0])
	if publicModel == "" {
		http.Error(w, "model 不能为空", http.StatusBadRequest)
		return
	}

	body := middleware.CachedBody(r.Context())
	if len(body) == 0 {
		http.Error(w, "请求体为空", http.StatusBadRequest)
		return
	}

	sanitizedBody, err := sanitizeGeminiBody(body, r.URL.Path)
	if err != nil {
		if errors.Is(err, errInvalidJSON) {
			http.Error(w, "请求体不是有效 JSON", http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	wantStream := strings.EqualFold(r.URL.Query().Get("alt"), "sse") || strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream")

	maxOut := extractGeminiMaxOutputTokens(sanitizedBody)

	freeMode := h.selfMode
	modelPassthrough := false
	if h.features != nil {
		fs := h.features.FeatureStateEffective(r.Context(), h.selfMode)
		freeMode = fs.BillingDisabled
		modelPassthrough = fs.ModelsDisabled
	}

	if h.models == nil {
		http.Error(w, "服务未配置模型目录", http.StatusBadGateway)
		return
	}

	var cons scheduler.Constraints
	cons.RequireChannelType = store.UpstreamTypeOpenAICompatible

	var upstreamByChannel map[int64]string
	if !modelPassthrough {
		_, err := h.models.GetEnabledManagedModelByPublicID(r.Context(), publicModel)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "模型未启用", http.StatusBadRequest)
				return
			}
			http.Error(w, "查询模型失败", http.StatusBadGateway)
			return
		}
		bindings, err := h.models.ListEnabledChannelModelBindingsByPublicID(r.Context(), publicModel)
		if err != nil {
			http.Error(w, "查询模型绑定失败", http.StatusBadGateway)
			return
		}
		upstreamByChannel = make(map[int64]string, len(bindings))
		for _, b := range bindings {
			if strings.TrimSpace(b.UpstreamModel) == "" {
				continue
			}
			upstreamByChannel[b.ChannelID] = b.UpstreamModel
		}
		if len(upstreamByChannel) == 0 {
			http.Error(w, "模型未配置可用上游", http.StatusBadGateway)
			return
		}
		cons.AllowChannelIDs = make(map[int64]struct{}, len(upstreamByChannel))
		for id := range upstreamByChannel {
			cons.AllowChannelIDs[id] = struct{}{}
		}
	} else {
		// 非 free_mode 下仍要求模型定价存在（用于配额预留与计费口径），但不要求“启用”。
		if !freeMode {
			if _, err := h.models.GetManagedModelByPublicID(r.Context(), publicModel); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					http.Error(w, "模型不存在", http.StatusBadRequest)
					return
				}
				http.Error(w, "查询模型失败", http.StatusBadGateway)
				return
			}
		}
	}

	allowGroups := p.Groups
	if len(allowGroups) == 0 {
		allowGroups = []string{"default"}
	}
	cons.AllowGroups = make(map[string]struct{}, len(allowGroups))
	for _, g := range allowGroups {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		cons.AllowGroups[g] = struct{}{}
	}

	routeKey := extractRouteKey(r)
	routeKeyHash := h.sched.RouteKeyHash(routeKey)

	usageID := int64(0)
	if h.quota != nil {
		res, err := h.quota.Reserve(r.Context(), quota.ReserveInput{
			RequestID:       middleware.GetRequestID(r.Context()),
			UserID:          p.UserID,
			TokenID:         *p.TokenID,
			Model:           optionalString(publicModel),
			MaxOutputTokens: maxOut,
		})
		if err != nil {
			if errors.Is(err, quota.ErrSubscriptionRequired) || errors.Is(err, quota.ErrQuotaExceeded) {
				http.Error(w, err.Error(), http.StatusTooManyRequests)
				return
			}
			if errors.Is(err, quota.ErrInsufficientBalance) {
				http.Error(w, err.Error(), http.StatusPaymentRequired)
				return
			}
			http.Error(w, "配额预留失败", http.StatusTooManyRequests)
			return
		}
		usageID = res.UsageEventID
	}
	reqBytes := int64(len(body))

	if h.groups == nil {
		if usageID != 0 && h.quota != nil {
			bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = h.quota.Void(bookCtx, usageID)
			cancel()
		}
		h.auditUpstreamError(r.Context(), r.URL.Path, p, nil, optionalString(publicModel), http.StatusBadGateway, "upstream_unavailable", 0)
		cw := &countingResponseWriter{ResponseWriter: w}
		http.Error(cw, "上游不可用", http.StatusBadGateway)
		h.maybeLogProxyFailure(r.Context(), r, p, nil, optionalString(publicModel), http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), wantStream)
		h.finalizeUsageEvent(r, usageID, nil, http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), wantStream, reqBytes, cw.bytes)
		return
	}

	router := scheduler.NewGroupRouter(h.groups, h.sched, p.UserID, routeKeyHash, cons)
	const absoluteMaxAttempts = 1000
	for i := 0; i < absoluteMaxAttempts; i++ {
		sel, err := router.Next(r.Context())
		if err != nil {
			break
		}

		upstreamModel := publicModel
		if upstreamByChannel != nil {
			up, ok := upstreamByChannel[sel.ChannelID]
			if !ok {
				continue
			}
			upstreamModel = up
		}

		// 重写 path：/v1beta/models/{public}:{action} -> /v1beta/models/{upstream}:{action}
		req2 := r.Clone(r.Context())
		req2.URL.Path = "/v1beta/models/" + upstreamModel + strings.TrimPrefix(pathTail, publicModel)

		rewritten := sanitizedBody
		rewritten, err = applyChannelRequestPolicy(rewritten, sel)
		if err != nil {
			continue
		}
		rewritten, err = applyChannelBodyFilters(rewritten, sel)
		if err != nil {
			continue
		}
		ctx := buildParamOverrideContext(sel, publicModel, upstreamModel, req2.URL.Path)
		rewritten, err = applyChannelParamOverride(rewritten, sel, ctx)
		if err != nil {
			continue
		}

		if h.proxyOnce(w, req2, sel, rewritten, wantStream, optionalString(publicModel), p, usageID, reqStart, reqBytes) {
			return
		}
	}

	if usageID != 0 && h.quota != nil {
		bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = h.quota.Void(bookCtx, usageID)
		cancel()
	}
	h.auditUpstreamError(r.Context(), r.URL.Path, p, nil, optionalString(publicModel), http.StatusBadGateway, "upstream_unavailable", 0)
	cw := &countingResponseWriter{ResponseWriter: w}
	http.Error(cw, "上游不可用", http.StatusBadGateway)
	h.maybeLogProxyFailure(r.Context(), r, p, nil, optionalString(publicModel), http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), wantStream)
	h.finalizeUsageEvent(r, usageID, nil, http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), wantStream, reqBytes, cw.bytes)
}

func extractGeminiMaxOutputTokens(body []byte) *int64 {
	v := gjson.GetBytes(body, "generationConfig.maxOutputTokens")
	if v.Exists() {
		n := v.Int()
		if n > 0 {
			out := int64(n)
			return &out
		}
	}
	v = gjson.GetBytes(body, "generation_config.max_output_tokens")
	if v.Exists() {
		n := v.Int()
		if n > 0 {
			out := int64(n)
			return &out
		}
	}
	return nil
}
