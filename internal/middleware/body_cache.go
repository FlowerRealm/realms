// Package middleware 提供请求体缓存，使 handler 可以多次读取 body（解析/校验→转发/重试）。
package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

type bodyKey int

const cachedBodyKey bodyKey = 1

func BodyCache(maxBytes int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil {
				next.ServeHTTP(w, r)
				return
			}
			defer r.Body.Close()

			lr := io.LimitReader(r.Body, maxBytes+1)
			b, err := io.ReadAll(lr)
			if err != nil {
				http.Error(w, "读取请求体失败", http.StatusBadRequest)
				return
			}
			if int64(len(b)) > maxBytes {
				http.Error(w, "请求体过大", http.StatusRequestEntityTooLarge)
				return
			}
			ctx := context.WithValue(r.Context(), cachedBodyKey, b)
			r.Body = io.NopCloser(bytes.NewReader(b))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func CachedBody(ctx context.Context) []byte {
	v := ctx.Value(cachedBodyKey)
	if v == nil {
		return nil
	}
	b, _ := v.([]byte)
	return b
}
