// Package middleware 提供 Web 会话鉴权：Cookie 会话 + CSRF token 绑定。
package middleware

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"

	"realms/internal/auth"
	"realms/internal/store"
)

func loginRedirectTarget(r *http.Request, base string) string {
	if r == nil || r.URL == nil {
		return base
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
	default:
		return base
	}
	next := strings.TrimSpace(r.URL.RequestURI())
	if next == "" || !strings.HasPrefix(next, "/") {
		return base
	}
	if strings.HasPrefix(next, "//") {
		return base
	}
	return base + "?next=" + url.QueryEscape(next)
}

func SessionAuth(st *store.Store, cookieName string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookieName)
			if err != nil || c.Value == "" {
				http.Redirect(w, r, loginRedirectTarget(r, "/login"), http.StatusFound)
				return
			}
			sess, err := st.GetSessionByRaw(r.Context(), c.Value)
			if err != nil {
				if err == sql.ErrNoRows {
					http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
					http.Redirect(w, r, loginRedirectTarget(r, "/login"), http.StatusFound)
					return
				}
				http.Error(w, "会话查询失败", http.StatusInternalServerError)
				return
			}
			user, err := st.GetUserByID(r.Context(), sess.UserID)
			if err != nil {
				http.Error(w, "用户查询失败", http.StatusInternalServerError)
				return
			}
			if user.Status != 1 {
				_ = st.DeleteSessionByRaw(r.Context(), c.Value)
				http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
				http.Redirect(w, r, loginRedirectTarget(r, "/login"), http.StatusFound)
				return
			}
			csrf := strings.TrimSpace(sess.CSRFToken)
			if csrf == "" {
				tok, err := auth.NewRandomToken("csrf_", 32)
				if err != nil {
					http.Error(w, "会话初始化失败", http.StatusInternalServerError)
					return
				}
				if err := st.UpdateSessionCSRFToken(r.Context(), sess.ID, tok); err != nil {
					http.Error(w, "会话初始化失败", http.StatusInternalServerError)
					return
				}
				csrf = tok
			}
			p := auth.Principal{
				ActorType: auth.ActorTypeSession,
				UserID:    user.ID,
				Role:      user.Role,
				Groups:    user.Groups,
				CSRFToken: &csrf,
			}
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
		})
	}
}
