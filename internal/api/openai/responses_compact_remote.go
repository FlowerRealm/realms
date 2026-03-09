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

func extractRouteGroupForCompact(headers http.Header) string {
	for _, name := range []string{"X-Realms-Route-Group", "X-Route-Group", "route_group"} {
		if v := strings.TrimSpace(headers.Get(name)); v != "" {
			return v
		}
	}
	return ""
}

func resolveCompactRouteGroup(groupHeader string, principal auth.Principal) (*string, error) {
	groupName := strings.TrimSpace(groupHeader)
	if groupName != "" {
		return &groupName, nil
	}
	ags := allowGroupsFromPrincipal(principal)
	if len(ags.Order) == 1 {
		only := strings.TrimSpace(ags.Order[0])
		if only != "" {
			return &only, nil
		}
	}
	if len(ags.Order) == 0 {
		return nil, errors.New("compact route_group missing and token has no effective channel groups")
	}
	return nil, errors.New("compact route_group missing and token has multiple effective channel groups")
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
	if h.models != nil && isPriorityServiceTier(serviceTier) {
		mm, err := h.models.GetManagedModelByPublicID(r.Context(), reqModel)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", serviceTierBadRequestMessage(err))
			} else {
				writeOpenAIError(w, http.StatusBadGateway, "api_error", "failed to query model")
			}
			return
		}
		if err := validateManagedModelServiceTier(mm, serviceTier); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", serviceTierBadRequestMessage(err))
			return
		}
	}

	if sessionID := extractSessionIDForCompact(r.Header, payload); sessionID == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "session_id is required")
		return
	}

	if h.compactGateway == nil || !h.compactGateway.Configured() {
		writeOpenAIError(w, http.StatusInternalServerError, "api_error", "compact gateway is not configured")
		return
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

	resp, err := h.compactGateway.ForwardResponsesCompact(r.Context(), r, body, middleware.GetRequestID(r.Context()))
	if err != nil {
		voidUsage()

		if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			timeoutMs := int64(h.compactGateway.Timeout() / time.Millisecond)
			if timeoutMs < 0 {
				timeoutMs = 0
			}
			writeOpenAIError(w, http.StatusGatewayTimeout, "upstream_error", "Upstream timeout after "+strconv.FormatInt(timeoutMs, 10)+"ms")
			h.finalizeUsageEvent(r, usageID, nil, http.StatusGatewayTimeout, "upstream_timeout", "upstream_timeout", time.Since(reqStart), 0, false, reqBytes, 0)
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
			writeOpenAIError(w, 499, "api_error", "Client disconnected before upstream completed")
			h.finalizeUsageEvent(r, usageID, nil, 499, "client_disconnect", "client_disconnect", time.Since(reqStart), 0, false, reqBytes, 0)
			return
		}

		writeOpenAIError(w, http.StatusBadGateway, "upstream_error", "Failed to reach upstream service")
		h.finalizeUsageEvent(r, usageID, nil, http.StatusBadGateway, "upstream_network_error", "upstream_network_error", time.Since(reqStart), 0, false, reqBytes, 0)
		return
	}
	if resp == nil {
		voidUsage()
		writeOpenAIError(w, http.StatusBadGateway, "upstream_error", "Failed to reach upstream service")
		h.finalizeUsageEvent(r, usageID, nil, http.StatusBadGateway, "upstream_network_error", "upstream_network_error", time.Since(reqStart), 0, false, reqBytes, 0)
		return
	}
	defer resp.Body.Close()

	var routeGroup *string
	if usageID != 0 && h.quota != nil {
		var routeGroupErr error
		routeGroup, routeGroupErr = resolveCompactRouteGroup(extractRouteGroupForCompact(resp.Header), p)
		if routeGroupErr != nil {
			voidUsage()
			writeOpenAIError(w, http.StatusBadGateway, "billing_error", routeGroupErr.Error())
			h.maybeLogProxyFailure(r.Context(), r, p, nil, modelPtr, http.StatusBadGateway, "compact_gateway_route_group_missing", routeGroupErr.Error(), time.Since(reqStart), false)
			h.finalizeUsageEvent(r, usageID, nil, http.StatusBadGateway, "compact_gateway_route_group_missing", routeGroupErr.Error(), time.Since(reqStart), 0, false, reqBytes, 0)
			return
		}
	}

	cw := &countingResponseWriter{ResponseWriter: w}
	copyResponseHeaders(cw.Header(), resp.Header)
	cw.WriteHeader(resp.StatusCode)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var capBuf limitedPrefixBuffer
		capBuf.maxBytes = upstreamErrorBodyMaxBytes
		_, _ = io.Copy(cw, io.TeeReader(resp.Body, &capBuf))
		respBytes := cw.bytes
		voidUsage()
		msg := ""
		if !capBuf.exceeded {
			msg = summarizeUpstreamErrorBody(capBuf.buf.Bytes())
		}
		h.maybeLogProxyFailure(r.Context(), r, p, nil, modelPtr, resp.StatusCode, "upstream_status", msg, time.Since(reqStart), false)
		h.finalizeUsageEvent(r, usageID, nil, resp.StatusCode, "upstream_status", msg, time.Since(reqStart), 0, false, reqBytes, respBytes)
		return
	}

	var capBuf limitedPrefixBuffer
	capBuf.maxBytes = upstreamNonStreamExtractMaxBytes
	_, copyErr := io.Copy(cw, io.TeeReader(resp.Body, &capBuf))
	respBytes := cw.bytes
	if copyErr != nil {
		voidUsage()
		h.maybeLogProxyFailure(r.Context(), r, p, nil, modelPtr, resp.StatusCode, "proxy_copy", copyErr.Error(), time.Since(reqStart), false)
		h.finalizeUsageEvent(r, usageID, nil, resp.StatusCode, "proxy_copy", "", time.Since(reqStart), 0, false, reqBytes, respBytes)
		return
	}

	var inTok, outTok, cachedInTok, cachedOutTok *int64
	if !capBuf.exceeded {
		inTok, outTok, cachedInTok, cachedOutTok = extractUsageTokens(capBuf.buf.Bytes())
	}
	if usageID != 0 && h.quota != nil {
		bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.quota.Commit(bookCtx, quota.CommitInput{
			UsageEventID:       usageID,
			Model:              modelPtr,
			ServiceTier:        serviceTier,
			RouteGroup:         routeGroup,
			InputTokens:        inTok,
			CachedInputTokens:  cachedInTok,
			OutputTokens:       outTok,
			CachedOutputTokens: cachedOutTok,
		})
	}

	h.finalizeUsageEvent(r, usageID, nil, resp.StatusCode, "", "", time.Since(reqStart), 0, false, reqBytes, respBytes)
}
