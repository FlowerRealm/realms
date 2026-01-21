// Package middleware 提供“功能禁用（Feature Bans）”的路由级保护。
package middleware

import (
	"context"
	"net/http"
	"strings"
)

type boolSettingGetter interface {
	GetBoolAppSetting(ctx context.Context, key string) (bool, bool, error)
}

type featureDisabledGetter interface {
	FeatureDisabledEffective(ctx context.Context, selfMode bool, key string) bool
}

// FeatureGate 在运行时基于 app_settings 的 bool key 决定是否拒绝访问。
// 约定：当 key 对应的值为 true 时，视为禁用并返回 404。
func FeatureGate(st boolSettingGetter, key string) Middleware {
	k := strings.TrimSpace(key)
	if k == "" || st == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			disabled, ok, err := st.GetBoolAppSetting(r.Context(), k)
			if err == nil && ok && disabled {
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// FeatureGateEffective 在运行时基于“最终禁用状态”（包含配置文件默认值与 self_mode 硬禁用）决定是否拒绝访问。
// 约定：当 key 对应的值为 true 时，视为禁用并返回 404。
func FeatureGateEffective(st featureDisabledGetter, selfMode bool, key string) Middleware {
	k := strings.TrimSpace(key)
	if k == "" || st == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if st.FeatureDisabledEffective(r.Context(), selfMode, k) {
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
