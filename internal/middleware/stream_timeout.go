// Package middleware 提供“流式感知”的请求超时：对 SSE 请求放宽/取消固定 deadline，避免误杀长连接。
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func StreamAwareRequestTimeout(nonStream, stream time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			d := nonStream
			if isStreamCapablePath(r.URL.Path) && requestWantsSSE(r) {
				d = stream
			}
			if d <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func isStreamCapablePath(path string) bool {
	switch path {
	case "/v1/responses":
		return true
	default:
		return false
	}
}

func requestWantsSSE(r *http.Request) bool {
	if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		return true
	}
	b := CachedBody(r.Context())
	if len(b) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(b, &payload); err != nil {
		return false
	}
	switch v := payload["stream"].(type) {
	case bool:
		return v
	case string:
		v = strings.TrimSpace(v)
		return v == "1" || strings.EqualFold(v, "true")
	default:
		return false
	}
}
