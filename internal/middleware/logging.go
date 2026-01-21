// Package middleware 提供最小访问日志（结构化），默认脱敏，不记录请求体与任何明文凭据。
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"realms/internal/auth"
)

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

func (w *statusWriter) Flush() {
	if fl, ok := w.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}

func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(sw, r)
		lat := time.Since(start)

		var userID any
		var actor any
		if p, ok := auth.PrincipalFromContext(r.Context()); ok {
			userID = p.UserID
			actor = p.ActorType
		}
		slog.Info("access",
			"request_id", GetRequestID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"bytes", sw.bytes,
			"latency_ms", lat.Milliseconds(),
			"user_id", userID,
			"actor_type", actor,
		)
	})
}
