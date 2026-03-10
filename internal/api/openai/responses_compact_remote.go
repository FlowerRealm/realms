package openai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"realms/internal/auth"
	"realms/internal/middleware"
	"realms/internal/quota"
	"realms/internal/upstream"
)

func extractSessionIDForCompact(headers http.Header, body map[string]any) string {
	fromHeaders := ""
	for _, name := range []string{"session_id", "session-id"} {
		if v := strings.TrimSpace(headers.Get(name)); v != "" {
			fromHeaders = v
			break
		}
	}
	if fromHeaders != "" {
		return fromHeaders
	}
	if body == nil {
		return ""
	}
	if v, ok := body["session_id"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v, ok := body["sessionId"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return ""
}

func compactGatewayErrorClass(headers http.Header) string {
	if headers == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(headers.Get(upstream.CompactGatewayErrorClassHeader)))
}

func stripCompactGatewayInternalHeaders(headers http.Header) {
	if headers == nil {
		return
	}
	for _, name := range []string{
		upstream.CompactGatewayErrorClassHeader,
		"X-Realms-Route-Group",
		"X-Route-Group",
		"route_group",
	} {
		headers.Del(name)
	}
}

func compactRouteKeySource(payload map[string]any, r *http.Request) string {
	if normalizeRouteKey(extractRouteKeyFromPayload(payload)) != "" {
		return "payload"
	}
	if r != nil && normalizeRouteKey(extractRouteKey(r)) != "" {
		return "header"
	}
	if normalizeRouteKey(deriveRouteKeyFromConversationPayload(payload)) != "" {
		return "derived"
	}
	return "missing"
}

func compactRouteKeyHash(payload map[string]any, r *http.Request, routeKeySource string, sched routeKeyHasher) string {
	if sched == nil {
		return ""
	}
	routeKey := normalizeRouteKey(extractRouteKeyFromPayload(payload))
	if routeKey == "" && r != nil {
		routeKey = normalizeRouteKey(extractRouteKey(r))
	}
	if routeKey == "" {
		routeKey = normalizeRouteKey(deriveRouteKeyFromConversationPayload(payload))
	}
	if routeKey == "" || !shouldEnableStickyRouting(payload, r, routeKeySource) {
		return ""
	}
	return sched.RouteKeyHash(routeKey)
}

type routeKeyHasher interface {
	RouteKeyHash(routeKey string) string
}

func (h *Handler) ResponsesCompact(w http.ResponseWriter, r *http.Request) {
	reqStart := time.Now()
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}

	body := middleware.CachedBody(r.Context())
	if len(body) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}
	reqBytes := int64(len(body))

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	body, serviceTier, err := normalizeRequestServiceTier(body, payload)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "service_tier is invalid")
		return
	}

	reqModel := ""
	if v, ok := payload["model"].(string); ok {
		reqModel = strings.TrimSpace(v)
	}
	if reqModel == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if sessionID := extractSessionIDForCompact(r.Header, payload); sessionID == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "session_id is required")
		return
	}
	ags := allowGroupsFromPrincipal(p)
	if len(ags.Order) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Token 未配置渠道组")
		return
	}
	allowSet := ags.Set

	if h.models == nil {
		writeOpenAIError(w, http.StatusBadGateway, "api_error", "failed to query model")
		return
	}
	mm, err := h.models.GetEnabledManagedModelByPublicID(r.Context(), reqModel)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "模型未启用")
		} else {
			writeOpenAIError(w, http.StatusBadGateway, "api_error", "failed to query model")
		}
		return
	}
	groupName := managedModelGroupName(mm)
	if allowSet != nil {
		if _, ok := allowSet[groupName]; !ok {
			writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "无权限使用该模型")
			return
		}
	}
	if err := validateManagedModelServiceTier(mm, serviceTier); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", serviceTierBadRequestMessage(err))
		return
	}

	if h.compactGateway == nil || !h.compactGateway.Configured() {
		writeOpenAIError(w, http.StatusInternalServerError, "api_error", "compact gateway is not configured")
		return
	}
	routeKeySource := compactRouteKeySource(payload, r)
	w.Header().Set("X-Realms-Route-Key-Source", routeKeySource)
	routeKeyHash := ""
	if h.sched != nil {
		routeKeyHash = compactRouteKeyHash(payload, r, routeKeySource, h.sched)
	}

	freeMode := false
	if h.features != nil {
		fs := h.features.FeatureStateEffective(r.Context())
		freeMode = fs.BillingDisabled
	}

	maxOut := intFromAny(payload["max_output_tokens"])
	if maxOut == nil {
		maxOut = intFromAny(payload["max_tokens"])
	}
	if maxOut == nil {
		maxOut = intFromAny(payload["max_completion_tokens"])
	}

	usageID := int64(0)
	modelPtr := optionalString(reqModel)
	if !freeMode && h.quota != nil {
		res, err := h.quota.Reserve(r.Context(), quota.ReserveInput{
			RequestID:       middleware.GetRequestID(r.Context()),
			UserID:          p.UserID,
			TokenID:         *p.TokenID,
			Model:           modelPtr,
			ServiceTier:     serviceTier,
			MaxOutputTokens: maxOut,
		})
		if err != nil {
			if msg := reserveBadRequestMessage(err); msg != "" {
				writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", msg)
				return
			}
			if errors.Is(err, quota.ErrSubscriptionRequired) || errors.Is(err, quota.ErrQuotaExceeded) {
				writeOpenAIError(w, http.StatusTooManyRequests, "rate_limit_error", err.Error())
				return
			}
			if errors.Is(err, quota.ErrInsufficientBalance) {
				writeOpenAIError(w, http.StatusPaymentRequired, "billing_error", err.Error())
				return
			}
			writeOpenAIError(w, http.StatusTooManyRequests, "rate_limit_error", "quota reserve failed")
			return
		}
		usageID = res.UsageEventID
	}

	voidUsage := func() {
		if usageID == 0 || h.quota == nil {
			return
		}
		bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.quota.Void(bookCtx, usageID)
	}
	bestFailure := proxyFailureInfo{}
	for _, groupName := range ags.Order {
		targetGroup := strings.TrimSpace(groupName)
		if targetGroup == "" {
			continue
		}
		resp, err := h.compactGateway.ForwardResponsesCompact(r.Context(), r, body, middleware.GetRequestID(r.Context()), upstream.CompactGatewayRequestOptions{
			TargetGroup:  targetGroup,
			RouteKeyHash: routeKeyHash,
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
				voidUsage()
				writeOpenAIError(w, 499, "api_error", "Client disconnected before upstream completed")
				h.finalizeUsageEventWithModelCheck(r, usageID, nil, 499, "client_disconnect", "client_disconnect", time.Since(reqStart), 0, false, reqBytes, 0, modelPtr, nil)
				return
			}
			if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
				voidUsage()
				timeoutMs := int64(h.compactGateway.Timeout() / time.Millisecond)
				if timeoutMs < 0 {
					timeoutMs = 0
				}
				writeOpenAIError(w, http.StatusGatewayTimeout, "upstream_error", "Upstream timeout after "+strconv.FormatInt(timeoutMs, 10)+"ms")
				h.finalizeUsageEventWithModelCheck(r, usageID, nil, http.StatusGatewayTimeout, "upstream_timeout", "upstream_timeout", time.Since(reqStart), 0, false, reqBytes, 0, modelPtr, nil)
				return
			}
			if errors.Is(err, context.DeadlineExceeded) {
				timeoutMs := int64(h.compactGateway.Timeout() / time.Millisecond)
				if timeoutMs < 0 {
					timeoutMs = 0
				}
				recordProxyFailure(&bestFailure, proxyFailureInfo{
					Valid:      true,
					Class:      "upstream_timeout",
					StatusCode: http.StatusGatewayTimeout,
					Message:    "Upstream timeout after " + strconv.FormatInt(timeoutMs, 10) + "ms",
				})
				continue
			}
			recordProxyFailure(&bestFailure, proxyFailureInfo{
				Valid:   true,
				Class:   "network",
				Message: trimSummary(err.Error()),
			})
			continue
		}
		if resp == nil {
			recordProxyFailure(&bestFailure, proxyFailureInfo{
				Valid:   true,
				Class:   "network",
				Message: "empty upstream response",
			})
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			errClass := compactGatewayErrorClass(resp.Header)
			if errClass == "group_unavailable" {
				bodyBytes := readPrefixBestEffort(resp.Body, upstreamErrorBodyMaxBytes)
				recordProxyFailure(&bestFailure, proxyFailureInfo{
					Valid:      true,
					Class:      "upstream_group_unavailable",
					StatusCode: resp.StatusCode,
					Message:    summarizeUpstreamErrorBody(bodyBytes),
					Body:       bodyBytes,
				})
				_ = resp.Body.Close()
				continue
			}

			cw := &countingResponseWriter{ResponseWriter: w}
			copyResponseHeaders(cw.Header(), resp.Header)
			stripCompactGatewayInternalHeaders(cw.Header())
			cw.WriteHeader(resp.StatusCode)

			var capBuf limitedPrefixBuffer
			capBuf.maxBytes = upstreamErrorBodyMaxBytes
			_, _ = io.Copy(cw, io.TeeReader(resp.Body, &capBuf))
			respBytes := cw.bytes
			_ = resp.Body.Close()
			voidUsage()
			msg := ""
			if !capBuf.exceeded {
				msg = summarizeUpstreamErrorBody(capBuf.buf.Bytes())
			}
			h.maybeLogProxyFailure(r.Context(), r, p, nil, modelPtr, resp.StatusCode, "upstream_status", msg, time.Since(reqStart), false)
			h.finalizeUsageEventWithModelCheck(r, usageID, nil, resp.StatusCode, "upstream_status", msg, time.Since(reqStart), 0, false, reqBytes, respBytes, modelPtr, nil)
			return
		}

		cw := &countingResponseWriter{ResponseWriter: w}
		copyResponseHeaders(cw.Header(), resp.Header)
		stripCompactGatewayInternalHeaders(cw.Header())
		cw.WriteHeader(resp.StatusCode)

		var capBuf limitedPrefixBuffer
		capBuf.maxBytes = upstreamNonStreamExtractMaxBytes
		_, copyErr := io.Copy(cw, io.TeeReader(resp.Body, &capBuf))
		respBytes := cw.bytes
		_ = resp.Body.Close()
		if copyErr != nil {
			voidUsage()
			h.maybeLogProxyFailure(r.Context(), r, p, nil, modelPtr, resp.StatusCode, "proxy_copy", copyErr.Error(), time.Since(reqStart), false)
			h.finalizeUsageEventWithModelCheck(r, usageID, nil, resp.StatusCode, "proxy_copy", "", time.Since(reqStart), 0, false, reqBytes, respBytes, modelPtr, nil)
			return
		}

		var inTok, outTok, cachedInTok, cachedOutTok *int64
		var responseModel *string
		if !capBuf.exceeded {
			inTok, outTok, cachedInTok, cachedOutTok = extractUsageTokens(capBuf.buf.Bytes())
			responseModel = extractTopLevelModel(capBuf.buf.Bytes())
		}
		if usageID != 0 && h.quota != nil {
			bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.quota.Commit(bookCtx, quota.CommitInput{
				UsageEventID:       usageID,
				Model:              modelPtr,
				ServiceTier:        serviceTier,
				RouteGroup:         &targetGroup,
				InputTokens:        inTok,
				CachedInputTokens:  cachedInTok,
				OutputTokens:       outTok,
				CachedOutputTokens: cachedOutTok,
			})
		}

		h.finalizeUsageEventWithModelCheck(r, usageID, nil, resp.StatusCode, "", "", time.Since(reqStart), 0, false, reqBytes, respBytes, modelPtr, responseModel)
		return
	}

	voidUsage()
	failResp := h.buildFailoverExhaustedResponse("openai", bestFailure)
	cw := &countingResponseWriter{ResponseWriter: w}
	writeOpenAIErrorWithRetryAfter(cw, failResp.Status, failResp.ErrType, failResp.Message, failResp.RetryAfterSeconds)
	if !failResp.SkipMonitoring {
		h.maybeLogProxyFailure(r.Context(), r, p, nil, modelPtr, failResp.Status, failResp.ErrorClass, failResp.Message, time.Since(reqStart), false)
	}
	finalClass := failResp.ErrorClass
	if failResp.SkipMonitoring {
		finalClass = ""
	}
	h.finalizeUsageEventWithModelCheck(r, usageID, nil, failResp.Status, finalClass, failResp.UsageMessage, time.Since(reqStart), 0, false, reqBytes, cw.bytes, modelPtr, nil)
}
