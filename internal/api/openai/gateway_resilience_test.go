package openai

import (
	"bytes"
	"net/http"
	"testing"
	"time"
)

func TestSameSelectionRetries_DefaultPolicyPreservesLegacyAttempts(t *testing.T) {
	h := &Handler{gateway: defaultGatewayOptions()}
	if got := h.sameSelectionRetries(2); got != 2 {
		t.Fatalf("sameSelectionRetries(default) = %d, want 2", got)
	}
}

func TestSameSelectionRetries_ConfiguredZeroDisablesRetries(t *testing.T) {
	h := &Handler{}
	h.SetGatewayPolicy(GatewayPolicy{MaxRetryAttempts: 0})
	if got := h.sameSelectionRetries(2); got != 1 {
		t.Fatalf("sameSelectionRetries(configured zero) = %d, want 1", got)
	}
}

func TestSameSelectionRetries_ConfiguredOneMeansRetryOnce(t *testing.T) {
	h := &Handler{}
	h.SetGatewayPolicy(GatewayPolicy{MaxRetryAttempts: 1})
	if got := h.sameSelectionRetries(1); got != 2 {
		t.Fatalf("sameSelectionRetries(configured one) = %d, want 2", got)
	}
}

func TestFailoverExhausted_DefaultPolicyDoesNotLimitSwitches(t *testing.T) {
	h := &Handler{gateway: defaultGatewayOptions()}
	if h.failoverExhausted(time.Now(), 100) {
		t.Fatal("default policy should not cap failover switches")
	}
}

func TestFailoverExhausted_ConfiguredSwitchLimitIsInclusive(t *testing.T) {
	h := &Handler{}
	h.SetGatewayPolicy(GatewayPolicy{MaxFailoverSwitches: 1})
	if h.failoverExhausted(time.Now(), 0) {
		t.Fatal("zero switches should be allowed")
	}
	if h.failoverExhausted(time.Now(), 1) {
		t.Fatal("one switch should still be allowed")
	}
	if !h.failoverExhausted(time.Now(), 2) {
		t.Fatal("second switch should exhaust limit 1")
	}
}

func TestFailoverExhausted_ConfiguredZeroDisablesFailover(t *testing.T) {
	h := &Handler{}
	h.SetGatewayPolicy(GatewayPolicy{MaxFailoverSwitches: 0})
	if h.failoverExhausted(time.Now(), 0) {
		t.Fatal("initial attempt should still be allowed")
	}
	if !h.failoverExhausted(time.Now(), 1) {
		t.Fatal("first switch should exhaust zero-switch policy")
	}
}

func TestBackoffWithinRetryElapsed_AllowsDelayInsideBudget(t *testing.T) {
	h := &Handler{}
	h.SetGatewayPolicy(GatewayPolicy{MaxRetryElapsed: 100 * time.Millisecond})

	got, ok := h.backoffWithinRetryElapsed(time.Now().Add(-40*time.Millisecond), 20*time.Millisecond)
	if !ok {
		t.Fatal("expected backoff within budget to be allowed")
	}
	if got != 20*time.Millisecond {
		t.Fatalf("backoff = %s, want %s", got, 20*time.Millisecond)
	}
}

func TestBackoffWithinRetryElapsed_RejectsDelayOutsideBudget(t *testing.T) {
	h := &Handler{}
	h.SetGatewayPolicy(GatewayPolicy{MaxRetryElapsed: 100 * time.Millisecond})

	got, ok := h.backoffWithinRetryElapsed(time.Now().Add(-95*time.Millisecond), 20*time.Millisecond)
	if ok {
		t.Fatal("expected backoff beyond budget to be rejected")
	}
	if got != 0 {
		t.Fatalf("backoff = %s, want 0", got)
	}
}

func TestRecordProxyFailure_TruncatesStoredBody(t *testing.T) {
	huge := bytes.Repeat([]byte("a"), failoverErrorBodyMaxBytes+256)
	var best proxyFailureInfo

	recordProxyFailure(&best, proxyFailureInfo{
		Valid:      true,
		Class:      "upstream_status",
		StatusCode: http.StatusTooManyRequests,
		Message:    "upstream throttled",
		Body:       huge,
	})

	if len(best.Body) != failoverErrorBodyMaxBytes {
		t.Fatalf("stored body len = %d, want %d", len(best.Body), failoverErrorBodyMaxBytes)
	}
	huge[0] = 'b'
	if best.Body[0] != 'a' {
		t.Fatal("expected stored body to be copied")
	}
}

type passthroughMatcherRecorder struct {
	called bool
}

func (m *passthroughMatcherRecorder) Match(_ string, _ int, _ []byte) (int, string, bool, bool) {
	m.called = true
	return http.StatusTeapot, "status-only passthrough", false, true
}

func TestBuildFailoverExhaustedResponse_AllowsStatusOnlyPassthrough(t *testing.T) {
	matcher := &passthroughMatcherRecorder{}
	h := &Handler{errorPassthrough: matcher}

	resp := h.buildFailoverExhaustedResponse("openai", proxyFailureInfo{
		Valid:      true,
		StatusCode: http.StatusTooManyRequests,
		Class:      "upstream_throttled",
	})

	if !matcher.called {
		t.Fatal("expected passthrough matcher to be called for status-only failure")
	}
	if resp.Status != http.StatusTeapot {
		t.Fatalf("response status = %d, want %d", resp.Status, http.StatusTeapot)
	}
	if resp.Message != "status-only passthrough" {
		t.Fatalf("response message = %q, want %q", resp.Message, "status-only passthrough")
	}
}

func TestBuildFailoverExhaustedResponse_DoesNotPassthroughLocalConcurrencyFailure(t *testing.T) {
	matcher := &passthroughMatcherRecorder{}
	h := &Handler{errorPassthrough: matcher}

	resp := h.buildFailoverExhaustedResponse("openai", proxyFailureInfo{
		Valid:      true,
		StatusCode: http.StatusTooManyRequests,
		Class:      "local_throttled",
		Message:    "并发等待队列已满",
	})

	if matcher.called {
		t.Fatal("expected passthrough matcher not to be called for local concurrency failure")
	}
	if resp.Status != http.StatusTooManyRequests {
		t.Fatalf("response status = %d, want %d", resp.Status, http.StatusTooManyRequests)
	}
	if resp.Message != "请求过于频繁，请稍后重试" {
		t.Fatalf("response message = %q, want %q", resp.Message, "请求过于频繁，请稍后重试")
	}
}
