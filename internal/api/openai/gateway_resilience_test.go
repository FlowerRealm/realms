package openai

import (
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
