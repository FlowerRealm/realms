package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"realms/internal/store"
)

func TestSessionAuth_RedirectsToLoginWithNext_ForGET(t *testing.T) {
	st := store.New(nil)
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), SessionAuth(st, "session"))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/dashboard?x=1", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, rr.Code)
	}
	if got := rr.Header().Get("Location"); got != "/login" {
		t.Fatalf("expected Location %q, got %q", "/login", got)
	}

	var next string
	for _, c := range rr.Result().Cookies() {
		if c.Name == "rlm_next" {
			next = c.Value
			break
		}
	}
	if next != "/dashboard" {
		t.Fatalf("expected next cookie %q, got %q", "/dashboard", next)
	}
}

func TestSessionAuth_RedirectsToLoginWithoutNext_ForPOST(t *testing.T) {
	st := store.New(nil)
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), SessionAuth(st, "session"))

	req := httptest.NewRequest(http.MethodPost, "http://example.com/logout", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, rr.Code)
	}
	if got := rr.Header().Get("Location"); got != "/login" {
		t.Fatalf("expected Location %q, got %q", "/login", got)
	}

	for _, c := range rr.Result().Cookies() {
		if c.Name == "rlm_next" {
			t.Fatalf("expected no next cookie, got %q", c.Value)
		}
	}
}
