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

// ChatCompletions 提供 OpenAI Chat Completions 兼容入口：POST /v1/chat/completions。
func (h *Handler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	h.proxyChatCompletionsJSON(w, r)
}

func (h *Handler) proxyChatCompletionsJSON(w http.ResponseWriter, r *http.Request) {
	reqStart := time.Now()
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	body := middleware.CachedBody(r.Context())
	if len(body) == 0 {
		http.Error(w, "请求体为空", http.StatusBadRequest)
		return
	}

	payload, err := sanitizeChatCompletionsPayload(body, 0)
	if err != nil {
		if errors.Is(err, errInvalidJSON) {
			http.Error(w, "请求体不是有效 JSON", http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stream := boolFromAny(payload["stream"])
	publicModel := strings.TrimSpace(stringFromAny(payload["model"]))
	maxOut := intFromAny(payload["max_completion_tokens"])
	if maxOut == nil {
		maxOut = intFromAny(payload["max_tokens"])
	}

	freeMode := h.selfMode
	modelPassthrough := false
	if h.features != nil {
		fs := h.features.FeatureStateEffective(r.Context(), h.selfMode)
		freeMode = fs.BillingDisabled
		modelPassthrough = fs.ModelsDisabled
	}

	if publicModel == "" {
		http.Error(w, "model 不能为空", http.StatusBadRequest)
		return
	}
	if h.models == nil {
		http.Error(w, "服务未配置模型目录", http.StatusBadGateway)
		return
	}

	var cons scheduler.Constraints
	cons.RequireChannelType = store.UpstreamTypeOpenAICompatible

	var rewriteBody func(sel scheduler.Selection) ([]byte, error)
	var upstreamByChannel map[int64]string

	if modelPassthrough {
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
		rewriteBody = func(sel scheduler.Selection) ([]byte, error) {
			out := clonePayload(payload)
			raw, err := json.Marshal(out)
			if err != nil {
				return nil, err
			}
			raw, err = applyChatCompletionsModelSuffixTransforms(raw, sel, publicModel)
			if err != nil {
				return nil, err
			}
			raw, err = applyChatCompletionsModelRules(raw)
			if err != nil {
				return nil, err
			}
			raw, err = applyChannelRequestPolicy(raw, sel)
			if err != nil {
				return nil, err
			}
			raw, err = applyChannelBodyFilters(raw, sel)
			if err != nil {
				return nil, err
			}
			upstreamModel := strings.TrimSpace(gjson.GetBytes(raw, "model").String())
			ctx := buildParamOverrideContext(sel, publicModel, upstreamModel, r.URL.Path)
			raw, err = applyChannelParamOverride(raw, sel, ctx)
			if err != nil {
				return nil, err
			}
			return raw, nil
		}
	} else {
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

		rewriteBody = func(sel scheduler.Selection) ([]byte, error) {
			up, ok := upstreamByChannel[sel.ChannelID]
			if !ok {
				return nil, errors.New("选中渠道未配置该模型")
			}
			out := clonePayload(payload)
			out["model"] = up
			raw, err := json.Marshal(out)
			if err != nil {
				return nil, err
			}
			raw, err = applyChatCompletionsModelSuffixTransforms(raw, sel, publicModel)
			if err != nil {
				return nil, err
			}
			raw, err = applyChatCompletionsModelRules(raw)
			if err != nil {
				return nil, err
			}
			raw, err = applyChannelRequestPolicy(raw, sel)
			if err != nil {
				return nil, err
			}
			raw, err = applyChannelBodyFilters(raw, sel)
			if err != nil {
				return nil, err
			}
			upstreamModel := strings.TrimSpace(gjson.GetBytes(raw, "model").String())
			ctx := buildParamOverrideContext(sel, publicModel, upstreamModel, r.URL.Path)
			raw, err = applyChannelParamOverride(raw, sel, ctx)
			if err != nil {
				return nil, err
			}
			return raw, nil
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

	routeKey := extractRouteKeyFromPayload(payload)
	if routeKey == "" {
		routeKey = extractRouteKey(r)
	}
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
		h.maybeLogProxyFailure(r.Context(), r, p, nil, optionalString(publicModel), http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), stream)
		h.finalizeUsageEvent(r, usageID, nil, http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), stream, reqBytes, cw.bytes)
		return
	}

	router := scheduler.NewGroupRouter(h.groups, h.sched, p.UserID, routeKeyHash, cons)
	const absoluteMaxAttempts = 1000
	for i := 0; i < absoluteMaxAttempts; i++ {
		sel, err := router.Next(r.Context())
		if err != nil {
			break
		}
		rewritten, err := rewriteBody(sel)
		if err != nil {
			if usageID != 0 && h.quota != nil {
				bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = h.quota.Void(bookCtx, usageID)
				cancel()
			}
			cw := &countingResponseWriter{ResponseWriter: w}
			http.Error(cw, "请求体处理失败", http.StatusInternalServerError)
			h.finalizeUsageEvent(r, usageID, &sel, http.StatusInternalServerError, "rewrite_body", "请求体处理失败", time.Since(reqStart), stream, reqBytes, cw.bytes)
			return
		}
		if h.tryWithSelection(w, r, p, sel, rewritten, stream, optionalString(publicModel), usageID, reqStart, reqBytes, 1) {
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
	h.maybeLogProxyFailure(r.Context(), r, p, nil, optionalString(publicModel), http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), stream)
	h.finalizeUsageEvent(r, usageID, nil, http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), stream, reqBytes, cw.bytes)
}
