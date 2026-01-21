package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeBoolSettingGetter struct {
	val bool
	ok  bool
	err error
}

func (f fakeBoolSettingGetter) GetBoolAppSetting(ctx context.Context, key string) (bool, bool, error) {
	return f.val, f.ok, f.err
}

func TestFeatureGate_Disabled_ReturnsNotFound(t *testing.T) {
	mw := FeatureGate(fakeBoolSettingGetter{val: true, ok: true}, "feature_disable_x")

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

func TestFeatureGate_NotSet_PassesThrough(t *testing.T) {
	mw := FeatureGate(fakeBoolSettingGetter{val: true, ok: false}, "feature_disable_x")

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

func TestFeatureGate_Error_PassesThrough(t *testing.T) {
	mw := FeatureGate(fakeBoolSettingGetter{val: true, ok: true, err: errors.New("boom")}, "feature_disable_x")

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

func TestFeatureGate_EmptyKey_PassesThrough(t *testing.T) {
	mw := FeatureGate(fakeBoolSettingGetter{val: true, ok: true}, "   ")

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

