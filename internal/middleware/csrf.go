// Package middleware 提供最小 CSRF 防护：对有副作用的方法校验 session 绑定的 csrf_token。
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"realms/internal/auth"
)

func CSRF() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			default:
			}

			p, ok := auth.PrincipalFromContext(r.Context())
			if !ok || p.ActorType != auth.ActorTypeSession || p.CSRFToken == nil {
				http.Error(w, "未登录", http.StatusUnauthorized)
				return
			}
			expected := strings.TrimSpace(*p.CSRFToken)
			if expected == "" {
				http.Error(w, "会话无效，请重新登录", http.StatusUnauthorized)
				return
			}

			headerToken := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
			if headerToken != "" && headerToken == expected {
				next.ServeHTTP(w, r)
				return
			}

			// 兼容 form/multipart：优先使用表单字段 _csrf（同时避免“意外注入 header”导致误判）。
			contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
			var err error
			if strings.HasPrefix(contentType, "multipart/form-data") {
				err = r.ParseMultipartForm(32 << 20)
			} else {
				err = r.ParseForm()
			}
			if err != nil {
				if strings.Contains(err.Error(), "request body too large") {
					http.Error(w, "上传超过大小限制", http.StatusRequestEntityTooLarge)
					return
				}
				var mbe *http.MaxBytesError
				if errors.As(err, &mbe) {
					http.Error(w, "上传超过大小限制", http.StatusRequestEntityTooLarge)
					return
				}
				http.Error(w, "表单解析失败", http.StatusBadRequest)
				return
			}
			formToken := strings.TrimSpace(r.FormValue("_csrf"))
			if formToken == "" || formToken != expected {
				http.Error(w, "CSRF 校验失败（请刷新页面后重试）", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
