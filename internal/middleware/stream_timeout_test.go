package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStreamAwareRequestTimeout_NonStreamTimesOut(t *testing.T) {
	t.Parallel()

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			w.WriteHeader(499)
		case <-time.After(50 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		}
	})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	Chain(h, BodyCache(1<<20), StreamAwareRequestTimeout(10*time.Millisecond, 0)).ServeHTTP(rr, req)
	if rr.Code != 499 {
		t.Fatalf("expected status 499, got %d", rr.Code)
	}
}

func TestStreamAwareRequestTimeout_StreamSkipsNonStreamDeadline(t *testing.T) {
	t.Parallel()

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			w.WriteHeader(499)
		case <-time.After(50 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		}
	})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"stream":true}`)))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	Chain(h, BodyCache(1<<20), StreamAwareRequestTimeout(10*time.Millisecond, 0)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

