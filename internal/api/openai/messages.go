package openai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"realms/internal/auth"
	"realms/internal/middleware"
	"realms/internal/quota"
	"realms/internal/scheduler"
	"realms/internal/store"
)

// Messages 提供 Anthropic Messages 兼容入口：POST /v1/messages。
func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	h.proxyMessagesJSON(w, r)
}

func (h *Handler) proxyMessagesJSON(w http.ResponseWriter, r *http.Request) {
	reqStart := time.Now()
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		writeAnthropicError(w, http.StatusUnauthorized, "未鉴权")
		return
	}
	body := middleware.CachedBody(r.Context())
	if len(body) == 0 {
		writeAnthropicError(w, http.StatusBadRequest, "请求体为空")
		return
	}

	payload := make(map[string]any)
	if err := json.Unmarshal(body, &payload); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "请求体不是有效 JSON")
		return
	}

	stream := boolFromAny(payload["stream"])
	publicModel := strings.TrimSpace(stringFromAny(payload["model"]))

	// Anthropic Messages 要求 max_tokens；这里提供一个保守默认值，避免缺省导致的上游拒绝。
	if _, ok := payload["max_tokens"]; !ok && h.defaultMaxOutputTokens > 0 {
		payload["max_tokens"] = int64(h.defaultMaxOutputTokens)
	}
	maxOut := intFromAny(payload["max_tokens"])

	freeMode := h.selfMode
	modelPassthrough := false
	if h.features != nil {
		fs := h.features.FeatureStateEffective(r.Context(), h.selfMode)
		freeMode = fs.BillingDisabled
		modelPassthrough = fs.ModelsDisabled
	}

	if publicModel == "" {
		writeAnthropicError(w, http.StatusBadRequest, "model 不能为空")
		return
	}
	if maxOut == nil || *maxOut <= 0 {
		writeAnthropicError(w, http.StatusBadRequest, "max_tokens 不能为空")
		return
	}
	if h.models == nil {
		writeAnthropicError(w, http.StatusBadGateway, "服务未配置模型目录")
		return
	}

	var cons scheduler.Constraints
	cons.RequireChannelType = store.UpstreamTypeAnthropic

	var rewriteBody func(sel scheduler.Selection) ([]byte, error)
	var upstreamByChannel map[int64]string

	if modelPassthrough {
		// 非 free_mode 下仍要求模型定价存在（用于配额预留与计费口径），但不要求“启用”。
		if !freeMode {
			if _, err := h.models.GetManagedModelByPublicID(r.Context(), publicModel); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeAnthropicError(w, http.StatusBadRequest, "模型不存在")
					return
				}
				writeAnthropicError(w, http.StatusBadGateway, "查询模型失败")
				return
			}
		}
		rewriteBody = func(_ scheduler.Selection) ([]byte, error) {
			return json.Marshal(payload)
		}
	} else {
		_, err := h.models.GetEnabledManagedModelByPublicID(r.Context(), publicModel)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeAnthropicError(w, http.StatusBadRequest, "模型未启用")
				return
			}
			writeAnthropicError(w, http.StatusBadGateway, "查询模型失败")
			return
		}
		bindings, err := h.models.ListEnabledChannelModelBindingsByPublicID(r.Context(), publicModel)
		if err != nil {
			writeAnthropicError(w, http.StatusBadGateway, "查询模型绑定失败")
			return
		}

		upstreamByChannel = make(map[int64]string, len(bindings))
		for _, b := range bindings {
			if strings.TrimSpace(b.UpstreamModel) == "" {
				continue
			}
			upstreamByChannel[b.ChannelID] = b.UpstreamModel
		}
		// 统一使用“渠道绑定模型”配置：无绑定即不可用（避免 legacy 字段导致的调度歧义）。
		if len(upstreamByChannel) == 0 {
			writeAnthropicError(w, http.StatusBadGateway, "模型未配置可用上游")
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
			payload["model"] = up
			return json.Marshal(payload)
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
				writeAnthropicError(w, http.StatusTooManyRequests, err.Error())
				return
			}
			if errors.Is(err, quota.ErrInsufficientBalance) {
				writeAnthropicError(w, http.StatusPaymentRequired, err.Error())
				return
			}
			writeAnthropicError(w, http.StatusTooManyRequests, "配额预留失败")
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
		writeAnthropicError(cw, http.StatusBadGateway, "上游不可用")
		h.maybeLogProxyFailure(r.Context(), r, p, nil, optionalString(publicModel), http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), stream)
		h.finalizeUsageEvent(r, usageID, nil, http.StatusBadGateway, "upstream_unavailable", time.Since(reqStart), stream, reqBytes, cw.bytes)
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
			writeAnthropicError(cw, http.StatusInternalServerError, "请求体处理失败")
			h.finalizeUsageEvent(r, usageID, &sel, http.StatusInternalServerError, "rewrite_body", time.Since(reqStart), stream, reqBytes, cw.bytes)
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
	writeAnthropicError(cw, http.StatusBadGateway, "上游不可用")
	h.maybeLogProxyFailure(r.Context(), r, p, nil, optionalString(publicModel), http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), stream)
	h.finalizeUsageEvent(r, usageID, nil, http.StatusBadGateway, "upstream_unavailable", time.Since(reqStart), stream, reqBytes, cw.bytes)
}

func writeAnthropicError(w http.ResponseWriter, status int, message string) {
	errType := anthropicErrorTypeForStatus(status)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": strings.TrimSpace(message),
		},
	})
}

func anthropicErrorTypeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	default:
		if status >= 500 {
			return "api_error"
		}
		return "invalid_request_error"
	}
}
