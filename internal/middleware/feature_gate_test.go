package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeFeatureDisabledGetter struct {
	disabled bool
}

func (f fakeFeatureDisabledGetter) FeatureDisabledEffective(ctx context.Context, selfMode bool, key string) bool {
	_ = ctx
	_ = selfMode
	_ = key
	return f.disabled
}

func TestFeatureGateEffective_Disabled_ReturnsNotFound(t *testing.T) {
	mw := FeatureGateEffective(fakeFeatureDisabledGetter{disabled: true}, false, "feature_disable_x")

	nextCalled := 0
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled++
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/x", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d", http.StatusNotFound, rr.Code)
	}
	if nextCalled != 0 {
		t.Fatalf("next handler should not be called")
	}
}

func TestFeatureGateEffective_Enabled_PassesThrough(t *testing.T) {
	mw := FeatureGateEffective(fakeFeatureDisabledGetter{disabled: false}, false, "feature_disable_x")

	nextCalled := 0
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled++
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/x", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rr.Code)
	}
	if nextCalled != 1 {
		t.Fatalf("next handler should be called once")
	}
}

func TestFeatureGateEffective_EmptyKey_PassesThrough(t *testing.T) {
	mw := FeatureGateEffective(fakeFeatureDisabledGetter{disabled: true}, false, "   ")

	nextCalled := 0
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled++
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/x", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rr.Code)
	}
	if nextCalled != 1 {
		t.Fatalf("next handler should be called once")
	}
}

func TestFeatureGateEffective_NilGetter_PassesThrough(t *testing.T) {
	mw := FeatureGateEffective(nil, false, "feature_disable_x")

	nextCalled := 0
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled++
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/x", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rr.Code)
	}
	if nextCalled != 1 {
		t.Fatalf("next handler should be called once")
	}
}
