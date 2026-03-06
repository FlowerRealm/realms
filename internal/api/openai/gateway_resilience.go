package openai

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"realms/internal/concurrency"
	"realms/internal/scheduler"
)

type concurrencyManager interface {
	AcquireUserSlotWithWait(ctx context.Context, userID int64, maxConcurrency int, onPing func() error) (func(), error)
	AcquireCredentialSlotWithWait(ctx context.Context, credentialKey string, maxConcurrency int) (func(), error)
}

type errorPassthroughMatcher interface {
	Match(platform string, upstreamStatus int, body []byte) (status int, message string, skipMonitoring bool, matched bool)
}

type gatewayOptions struct {
	maxRetryAttempts         int
	retryBaseDelay           time.Duration
	retryMaxDelay            time.Duration
	maxRetryElapsed          time.Duration
	maxFailoverSwitches      int
	userMaxConcurrency       int
	credentialMaxConcurrency int
}

type GatewayPolicy struct {
	MaxRetryAttempts         int
	RetryBaseDelay           time.Duration
	RetryMaxDelay            time.Duration
	MaxRetryElapsed          time.Duration
	MaxFailoverSwitches      int
	UserMaxConcurrency       int
	CredentialMaxConcurrency int
}

type gatewayErrorResponse struct {
	Status            int
	ErrType           string
	Message           string
	RetryAfterSeconds int
	ErrorClass        string
	UsageMessage      string
	SkipMonitoring    bool
}

func defaultGatewayOptions() gatewayOptions {
	// 默认保持历史行为（基本不限制）：
	// - tests/直接 NewHandler 不会被新策略改变
	// - 生产环境由 app.go 显式注入配置
	return gatewayOptions{
		maxRetryAttempts:         0,
		retryBaseDelay:           0,
		retryMaxDelay:            0,
		maxRetryElapsed:          0,
		maxFailoverSwitches:      0,
		userMaxConcurrency:       0,
		credentialMaxConcurrency: 0,
	}
}

func (h *Handler) SetGatewayPolicy(policy GatewayPolicy) {
	if h == nil {
		return
	}
	h.gatewayConfigured = true
	opts := gatewayOptions{
		maxRetryAttempts:         policy.MaxRetryAttempts,
		retryBaseDelay:           policy.RetryBaseDelay,
		retryMaxDelay:            policy.RetryMaxDelay,
		maxRetryElapsed:          policy.MaxRetryElapsed,
		maxFailoverSwitches:      policy.MaxFailoverSwitches,
		userMaxConcurrency:       policy.UserMaxConcurrency,
		credentialMaxConcurrency: policy.CredentialMaxConcurrency,
	}
	if opts.maxRetryAttempts < 0 {
		opts.maxRetryAttempts = 0
	}
	if opts.maxRetryAttempts > 20 {
		opts.maxRetryAttempts = 20
	}
	if opts.retryBaseDelay < 0 {
		opts.retryBaseDelay = 0
	}
	if opts.retryMaxDelay < 0 {
		opts.retryMaxDelay = 0
	}
	if opts.retryBaseDelay > 0 && opts.retryMaxDelay > 0 && opts.retryMaxDelay < opts.retryBaseDelay {
		opts.retryMaxDelay = opts.retryBaseDelay
	}
	if opts.maxRetryElapsed < 0 {
		opts.maxRetryElapsed = 0
	}
	if opts.maxFailoverSwitches < 0 {
		opts.maxFailoverSwitches = 0
	}
	if opts.userMaxConcurrency < 0 {
		opts.userMaxConcurrency = 0
	}
	if opts.credentialMaxConcurrency < 0 {
		opts.credentialMaxConcurrency = 0
	}
	h.gateway = opts
}

func (h *Handler) SetConcurrencyManager(mgr concurrencyManager) {
	if h == nil {
		return
	}
	h.concurrency = mgr
}

func (h *Handler) SetErrorPassthroughMatcher(matcher errorPassthroughMatcher) {
	if h == nil {
		return
	}
	h.errorPassthrough = matcher
}

func (h *Handler) sameSelectionRetries(defaultRetries int) int {
	if defaultRetries <= 0 {
		defaultRetries = 1
	}
	if h == nil || !h.gatewayConfigured {
		return defaultRetries
	}
	if h.gateway.maxRetryAttempts <= 0 {
		return 1
	}
	return h.gateway.maxRetryAttempts + 1
}

func (h *Handler) failoverExhausted(start time.Time, switches int) bool {
	if h == nil {
		return false
	}
	if h.gatewayConfigured && switches > h.gateway.maxFailoverSwitches {
		return true
	}
	if h.gateway.maxRetryElapsed > 0 && !start.IsZero() && time.Since(start) >= h.gateway.maxRetryElapsed {
		return true
	}
	return false
}

func (h *Handler) initialBackoff() time.Duration {
	if h == nil {
		return 0
	}
	if h.gateway.retryBaseDelay <= 0 {
		return 0
	}
	return h.gateway.retryBaseDelay
}

func (h *Handler) nextBackoff(current time.Duration) time.Duration {
	if h == nil {
		return 0
	}
	base := h.gateway.retryBaseDelay
	max := h.gateway.retryMaxDelay
	if base <= 0 {
		return 0
	}
	if current <= 0 {
		current = base
	}
	next := time.Duration(float64(current) * 1.5)
	if next < base {
		next = base
	}
	if max > 0 && next > max {
		next = max
	}
	jitter := 0.8 + rand.Float64()*0.4
	next = time.Duration(float64(next) * jitter)
	if next < base {
		next = base
	}
	if max > 0 && next > max {
		next = max
	}
	return next
}

func (h *Handler) waitBackoff(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (h *Handler) acquireUserSlot(ctx context.Context, userID int64) (func(), error) {
	if h == nil || h.concurrency == nil || h.gateway.userMaxConcurrency <= 0 || userID <= 0 {
		return func() {}, nil
	}
	return h.concurrency.AcquireUserSlotWithWait(ctx, userID, h.gateway.userMaxConcurrency, nil)
}

func (h *Handler) acquireCredentialSlot(ctx context.Context, sel scheduler.Selection) (func(), error) {
	if h == nil || h.concurrency == nil || h.gateway.credentialMaxConcurrency <= 0 {
		return func() {}, nil
	}
	credKey := strings.TrimSpace(sel.CredentialKey())
	if credKey == "" {
		return func() {}, nil
	}
	return h.concurrency.AcquireCredentialSlotWithWait(ctx, credKey, h.gateway.credentialMaxConcurrency)
}

func classifyConcurrencyAcquireFailure(err error) proxyFailureInfo {
	switch {
	case err == nil:
		return proxyFailureInfo{}
	case errors.Is(err, concurrency.ErrQueueFull):
		return proxyFailureInfo{
			Valid:      true,
			Class:      "local_throttled",
			StatusCode: http.StatusTooManyRequests,
			Message:    "并发等待队列已满",
		}
	case errors.Is(err, concurrency.ErrWaitTimeout):
		return proxyFailureInfo{
			Valid:      true,
			Class:      "local_throttled",
			StatusCode: http.StatusTooManyRequests,
			Message:    "并发等待超时",
		}
	default:
		return proxyFailureInfo{
			Valid:   true,
			Class:   "network",
			Message: trimSummary(err.Error()),
		}
	}
}

func (h *Handler) buildFailoverExhaustedResponse(platform string, best proxyFailureInfo) gatewayErrorResponse {
	resp := mapFailoverFailure(best)
	if h == nil || h.errorPassthrough == nil || best.StatusCode <= 0 || !strings.HasPrefix(strings.TrimSpace(best.Class), "upstream_") {
		return resp
	}
	status, msg, skip, matched := h.errorPassthrough.Match(platform, best.StatusCode, best.Body)
	if !matched {
		return resp
	}
	if status > 0 {
		resp.Status = status
	}
	msg = strings.TrimSpace(msg)
	if msg != "" {
		resp.Message = msg
		resp.UsageMessage = msg
	}
	resp.ErrType = gatewayErrorTypeForStatus(resp.Status)
	resp.ErrorClass = "upstream_passthrough"
	resp.SkipMonitoring = skip
	if resp.Status == http.StatusTooManyRequests {
		resp.RetryAfterSeconds = 30
	} else {
		resp.RetryAfterSeconds = 0
	}
	return resp
}

func mapFailoverFailure(best proxyFailureInfo) gatewayErrorResponse {
	resp := gatewayErrorResponse{
		Status:         http.StatusBadGateway,
		ErrType:        "upstream_error",
		Message:        "上游不可用，请稍后重试",
		ErrorClass:     "upstream_unavailable",
		UsageMessage:   upstreamUnavailableUsageMessage(best),
		SkipMonitoring: false,
	}

	switch {
	case best.StatusCode == http.StatusTooManyRequests || strings.EqualFold(best.Class, "upstream_throttled"):
		resp.Status = http.StatusTooManyRequests
		resp.ErrType = "rate_limit_error"
		resp.Message = "请求过于频繁，请稍后重试"
		resp.RetryAfterSeconds = 30
		resp.ErrorClass = "upstream_throttled"
	case best.StatusCode == 529:
		resp.Status = http.StatusServiceUnavailable
		resp.ErrType = "upstream_error"
		resp.Message = "上游服务繁忙，请稍后重试"
	case best.StatusCode >= 500:
		resp.Status = http.StatusBadGateway
		resp.ErrType = "upstream_error"
		resp.Message = "上游不可用，请稍后重试"
	case strings.EqualFold(best.Class, "network"):
		resp.Status = http.StatusBadGateway
		resp.ErrType = "upstream_error"
		resp.Message = "上游网络异常，请稍后重试"
	case best.StatusCode == http.StatusUnauthorized || best.StatusCode == http.StatusForbidden:
		resp.Status = http.StatusBadGateway
		resp.ErrType = "upstream_error"
		resp.Message = "上游鉴权异常，请稍后重试"
	default:
		// 保持默认 502。
	}
	return resp
}

func gatewayErrorTypeForStatus(status int) string {
	switch status {
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	default:
		if status >= 500 {
			return "upstream_error"
		}
		return "api_error"
	}
}

func writeAnthropicGatewayError(w http.ResponseWriter, resp gatewayErrorResponse) {
	if w == nil {
		return
	}
	if resp.RetryAfterSeconds > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(resp.RetryAfterSeconds))
	}
	writeAnthropicError(w, resp.Status, resp.Message)
}
