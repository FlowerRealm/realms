// Package middleware 提供基于 token 的并发护栏，避免单个 token 把服务打爆。
package middleware

import (
	"net/http"

	"realms/internal/auth"
	"realms/internal/limits"
)

func TokenInflightLimiter(l *limits.TokenLimits) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := auth.PrincipalFromContext(r.Context())
			if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
				http.Error(w, "鉴权信息缺失", http.StatusUnauthorized)
				return
			}
			if !l.AcquireInflight(*p.TokenID) {
				http.Error(w, "并发超限", http.StatusTooManyRequests)
				return
			}
			defer l.ReleaseInflight(*p.TokenID)
			next.ServeHTTP(w, r)
		})
	}
}
