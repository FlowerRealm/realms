package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccessLog_DoesNotLogAuthorization(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	secret := "cx_secret_should_not_appear"
	req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+secret)

	rr := httptest.NewRecorder()
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), RequestID, AccessLog)

	h.ServeHTTP(rr, req)

	out := buf.String()
	if strings.Contains(out, secret) {
		t.Fatalf("log contains secret token: %s", out)
	}
}
