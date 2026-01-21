// Package middleware 提供最小 RBAC：当前管理面仅允许 root 访问。
package middleware

import (
	"net/http"

	"realms/internal/auth"
)

func RequireRoles(roles ...string) Middleware {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := auth.PrincipalFromContext(r.Context())
			if !ok || p.ActorType != auth.ActorTypeSession {
				http.Error(w, "未登录", http.StatusUnauthorized)
				return
			}
			if _, ok := allowed[p.Role]; !ok {
				http.Error(w, "无权限", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
