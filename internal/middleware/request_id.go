// Package middleware 负责 request_id 的生成与透传，便于链路追踪与审计关联。
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"sync/atomic"
	"time"
)

type ctxKey int

const requestIDKey ctxKey = 1

const RequestIDHeader = "X-Request-Id"

var randRead = rand.Read

var requestIDFallbackCounter atomic.Uint64

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get(RequestIDHeader)
		if rid == "" {
			rid = newRequestID()
		}
		w.Header().Set(RequestIDHeader, rid)
		ctx := context.WithValue(r.Context(), requestIDKey, rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetRequestID(ctx context.Context) string {
	v := ctx.Value(requestIDKey)
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func newRequestID() string {
	var b [16]byte
	if _, err := randRead(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}

	// 极端情况下 crypto/rand 可能不可用；退化到“时间 + 计数器”确保进程内唯一性，避免全 0 碰撞。
	binary.BigEndian.PutUint64(b[:8], uint64(time.Now().UnixNano()))
	binary.BigEndian.PutUint64(b[8:], requestIDFallbackCounter.Add(1))
	return hex.EncodeToString(b[:])
}
