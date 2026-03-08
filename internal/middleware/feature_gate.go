// Package middleware 提供“功能禁用（Feature Bans）”的路由级保护。
package middleware

import (
	"context"
	"net/http"
	"strings"
)

type featureDisabledGetter interface {
	FeatureDisabledEffective(ctx context.Context, key string) bool
}

// FeatureGateEffective 在运行时基于“最终禁用状态”决定是否拒绝访问。
// 约定：当 key 对应的值为 true 时，视为禁用并返回 404。
func FeatureGateEffective(st featureDisabledGetter, key string) Middleware {
	k := strings.TrimSpace(key)
	if k == "" || st == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if st.FeatureDisabledEffective(r.Context(), k) {
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
