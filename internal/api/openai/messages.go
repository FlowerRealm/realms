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
	rawBody := body

	payload, err := sanitizeMessagesPayload(body, 0)
	if err != nil {
		if errors.Is(err, errInvalidJSON) {
			writeAnthropicError(w, http.StatusBadRequest, "请求体不是有效 JSON")
			return
		}
		writeAnthropicError(w, http.StatusBadRequest, err.Error())
		return
	}

	rawBody, serviceTier, err := normalizeRequestServiceTier(rawBody, payload)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "service_tier 非法")
		return
	}

	stream := boolFromAny(payload["stream"])
	publicModel := strings.TrimSpace(stringFromAny(payload["model"]))

	maxOut := intFromAny(payload["max_tokens"])

	freeMode := false
	modelPassthrough := false
	if h.features != nil {
		fs := h.features.FeatureStateEffective(r.Context())
		freeMode = fs.BillingDisabled
		modelPassthrough = fs.ModelsDisabled
	}

	if publicModel == "" {
		writeAnthropicError(w, http.StatusBadRequest, "model 不能为空")
		return
	}
	if h.models == nil {
		writeAnthropicError(w, http.StatusBadGateway, "服务未配置模型目录")
		return
	}

	var cons scheduler.Constraints
	cons.RequireChannelType = store.UpstreamTypeAnthropic
	ags := allowGroupsFromPrincipal(p)
	allowSet := ags.Set
	if len(ags.Order) == 0 {
		writeAnthropicError(w, http.StatusBadRequest, "Token 未配置渠道组")
		return
	}
	cons.AllowGroups = allowSet
	cons.AllowGroupOrder = ags.Order
	// 用户侧 token 默认按绑定顺序做 channel 级 failover；sticky 只影响“从哪里继续”，不决定是否启用该语义。
	cons.SequentialChannelFailover = true

	var rewriteBody func(sel scheduler.Selection) ([]byte, error)
	var resolvedBindings resolvedChannelModelBindings

	if modelPassthrough {
		// 非 free_mode 下仍要求模型定价存在（用于配额预留与计费口径），但不要求“启用”。
		if !freeMode || isPriorityServiceTier(serviceTier) {
			mm, err := h.models.GetManagedModelByPublicID(r.Context(), publicModel)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeAnthropicError(w, http.StatusBadRequest, "模型不存在")
					return
				}
				writeAnthropicError(w, http.StatusBadGateway, "查询模型失败")
				return
			}
			if allowSet != nil {
				if _, ok := allowSet[managedModelGroupName(mm)]; !ok {
					writeAnthropicError(w, http.StatusBadRequest, "无权限使用该模型")
					return
				}
			}
			if err := validateManagedModelServiceTier(mm, serviceTier); err != nil {
				writeAnthropicError(w, http.StatusBadRequest, serviceTierBadRequestMessage(err))
				return
			}
		}
		// passthrough 模式下仍尝试使用“渠道绑定模型”做 model 转发（best-effort）；
		// 但不强制要求存在绑定（无绑定时直接透传 model）。
		if bindings, err := h.models.ListEnabledChannelModelBindingsByPublicID(r.Context(), publicModel); err == nil {
			resolvedBindings = resolveChannelModelBindings(bindings, cons.RequireChannelType)
			if !resolvedBindings.Empty() {
				resolvedBindings.ApplyToConstraints(&cons)
			}
		}
		rewriteBody = func(sel scheduler.Selection) ([]byte, error) {
			if sel.PassThroughBodyEnabled {
				return rawBody, nil
			}
			upstreamModel := resolvedBindings.UpstreamModel(sel.ChannelID, publicModel)
			out := clonePayload(payload)
			out["model"] = upstreamModel
			applyChannelSystemPromptToMessagesPayload(out, sel)
			raw, err := json.Marshal(out)
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
			ctx := buildParamOverrideContext(sel, publicModel, upstreamModel, r.URL.Path)
			raw, err = applyChannelParamOverride(raw, sel, ctx)
			if err != nil {
				return nil, err
			}
			raw, err = normalizeMaxTokensInBody(raw)
			if err != nil {
				return nil, err
			}
			return raw, nil
		}
	} else {
		mm, err := h.models.GetEnabledManagedModelByPublicID(r.Context(), publicModel)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeAnthropicError(w, http.StatusBadRequest, "模型未启用")
				return
			}
			writeAnthropicError(w, http.StatusBadGateway, "查询模型失败")
			return
		}
		if allowSet != nil {
			if _, ok := allowSet[managedModelGroupName(mm)]; !ok {
				writeAnthropicError(w, http.StatusBadRequest, "无权限使用该模型")
				return
			}
		}
		if err := validateManagedModelServiceTier(mm, serviceTier); err != nil {
			writeAnthropicError(w, http.StatusBadRequest, serviceTierBadRequestMessage(err))
			return
		}
		bindings, err := h.models.ListEnabledChannelModelBindingsByPublicID(r.Context(), publicModel)
		if err != nil {
			writeAnthropicError(w, http.StatusBadGateway, "查询模型绑定失败")
			return
		}

		resolvedBindings = resolveChannelModelBindings(bindings, cons.RequireChannelType)
		// 统一使用“渠道绑定模型”配置：无绑定即不可用（避免 legacy 字段导致的调度歧义）。
		if resolvedBindings.Empty() {
			writeAnthropicError(w, http.StatusBadGateway, "模型未配置可用上游")
			return
		}
		resolvedBindings.ApplyToConstraints(&cons)

		rewriteBody = func(sel scheduler.Selection) ([]byte, error) {
			if sel.PassThroughBodyEnabled {
				return rawBody, nil
			}
			up := resolvedBindings.UpstreamModel(sel.ChannelID, "")
			if strings.TrimSpace(up) == "" {
				return nil, errors.New("选中渠道未配置该模型")
			}
			out := clonePayload(payload)
			out["model"] = up
			applyChannelSystemPromptToMessagesPayload(out, sel)
			raw, err := json.Marshal(out)
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
			ctx := buildParamOverrideContext(sel, publicModel, up, r.URL.Path)
			raw, err = applyChannelParamOverride(raw, sel, ctx)
			if err != nil {
				return nil, err
			}
			raw, err = normalizeMaxTokensInBody(raw)
			if err != nil {
				return nil, err
			}
			return raw, nil
		}
	}

	applyServiceTierConstraints(&cons, serviceTier)

	routeKey := extractRouteKeyFromPayload(payload)
	if routeKey == "" {
		routeKey = extractRouteKeyFromRawBody(rawBody)
	}
	if routeKey == "" {
		routeKey = extractRouteKey(r)
	}
	routeKeyHash := h.sched.RouteKeyHash(routeKey)

	usageID := int64(0)
	userRelease, userSlotErr := h.acquireUserSlot(r.Context(), p.UserID)
	if userSlotErr != nil {
		fail := classifyConcurrencyAcquireFailure(userSlotErr)
		resp := h.buildFailoverExhaustedResponse("anthropic", fail)
		cw := &countingResponseWriter{ResponseWriter: w}
		writeAnthropicGatewayError(cw, resp)
		if !resp.SkipMonitoring {
			h.maybeLogProxyFailure(r.Context(), r, p, nil, optionalString(publicModel), resp.Status, resp.ErrorClass, resp.Message, time.Since(reqStart), stream)
		}
		return
	}
	if userRelease != nil {
		defer userRelease()
	}
	if h.quota != nil {
		res, err := h.quota.Reserve(r.Context(), quota.ReserveInput{
			RequestID:       middleware.GetRequestID(r.Context()),
			UserID:          p.UserID,
			TokenID:         *p.TokenID,
			Model:           optionalString(publicModel),
			ServiceTier:     serviceTier,
			MaxOutputTokens: maxOut,
		})
		if err != nil {
			if msg := reserveBadRequestMessage(err); msg != "" {
				writeAnthropicError(w, http.StatusBadRequest, msg)
				return
			}
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
		resp := h.buildFailoverExhaustedResponse("anthropic", proxyFailureInfo{})
		if !resp.SkipMonitoring {
			h.auditUpstreamError(r.Context(), r.URL.Path, p, nil, optionalString(publicModel), resp.Status, resp.ErrorClass, 0)
		}
		cw := &countingResponseWriter{ResponseWriter: w}
		writeAnthropicGatewayError(cw, resp)
		if !resp.SkipMonitoring {
			h.maybeLogProxyFailure(r.Context(), r, p, nil, optionalString(publicModel), resp.Status, resp.ErrorClass, resp.Message, time.Since(reqStart), stream)
		}
		finalClass := resp.ErrorClass
		if resp.SkipMonitoring {
			finalClass = ""
		}
		h.finalizeUsageEvent(r, usageID, nil, resp.Status, finalClass, resp.UsageMessage, time.Since(reqStart), 0, stream, reqBytes, cw.bytes)
		return
	}

	router := scheduler.NewGroupRouter(h.groups, h.sched, p.UserID, routeKeyHash, cons)
	bestFailure := proxyFailureInfo{}
	loopStart := time.Now()
	switches := 0
	backoff := h.initialBackoff()
	for {
		if h.failoverExhausted(loopStart, switches) {
			break
		}
		sel, err := router.Next(r.Context())
		if err != nil {
			if h.finalizeIfCanceled(r, usageID, nil, reqStart, stream, reqBytes) {
				return
			}
			if msg := serviceTierSelectionBadRequestMessage(err); msg != "" {
				if usageID != 0 && h.quota != nil {
					bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_ = h.quota.Void(bookCtx, usageID)
					cancel()
				}
				writeAnthropicError(w, http.StatusBadRequest, msg)
				h.finalizeUsageEvent(r, usageID, nil, http.StatusBadRequest, "service_tier", msg, time.Since(reqStart), 0, stream, reqBytes, 0)
				return
			}
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
			h.finalizeUsageEvent(r, usageID, &sel, http.StatusInternalServerError, "rewrite_body", "请求体处理失败", time.Since(reqStart), 0, stream, reqBytes, cw.bytes)
			return
		}
		bindingID := resolvedBindings.BindingID(sel.ChannelID)
		if h.tryWithSelection(w, r, p, sel, rewritten, stream, optionalString(publicModel), extractTopLevelModel(rewritten), bindingID, usageID, reqStart, reqBytes, loopStart, 1, &bestFailure) {
			return
		}
		switches++
		if h.failoverExhausted(loopStart, switches) {
			break
		}
		if !h.waitBackoffWithinRetryElapsed(r.Context(), loopStart, backoff) {
			if h.finalizeIfCanceled(r, usageID, nil, reqStart, stream, reqBytes) {
				return
			}
			break
		}
		backoff = h.nextBackoff(backoff)
	}

	if usageID != 0 && h.quota != nil {
		bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = h.quota.Void(bookCtx, usageID)
		cancel()
	}
	resp := h.buildFailoverExhaustedResponse("anthropic", bestFailure)
	if !resp.SkipMonitoring {
		h.auditUpstreamError(r.Context(), r.URL.Path, p, nil, optionalString(publicModel), resp.Status, resp.ErrorClass, 0)
	}
	cw := &countingResponseWriter{ResponseWriter: w}
	writeAnthropicGatewayError(cw, resp)
	if !resp.SkipMonitoring {
		h.maybeLogProxyFailure(r.Context(), r, p, nil, optionalString(publicModel), resp.Status, resp.ErrorClass, resp.Message, time.Since(reqStart), stream)
	}
	finalClass := resp.ErrorClass
	if resp.SkipMonitoring {
		finalClass = ""
	}
	h.finalizeUsageEvent(r, usageID, nil, resp.Status, finalClass, resp.UsageMessage, time.Since(reqStart), 0, stream, reqBytes, cw.bytes)
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
