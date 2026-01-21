// Package middleware 提供数据面 Token 鉴权：Authorization: Bearer <token> 或 x-api-key。
package middleware

import (
	"database/sql"
	"net/http"
	"strings"

	"realms/internal/auth"
	"realms/internal/store"
)

func TokenAuth(st *store.Store) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := extractBearer(r.Header.Get("Authorization"))
			if raw == "" {
				raw = r.Header.Get("x-api-key")
			}
			if raw == "" {
				http.Error(w, "未提供 Token", http.StatusUnauthorized)
				return
			}
			ta, err := st.GetTokenAuthByRawToken(r.Context(), raw)
			if err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "Token 无效", http.StatusUnauthorized)
					return
				}
				http.Error(w, "鉴权失败", http.StatusInternalServerError)
				return
			}
			tokenID := ta.TokenID
			p := auth.Principal{
				ActorType: auth.ActorTypeToken,
				UserID:    ta.UserID,
				TokenID:   &tokenID,
				Role:      ta.Role,
				Groups:    ta.Groups,
			}
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
		})
	}
}

func extractBearer(v string) string {
	if v == "" {
		return ""
	}
	parts := strings.SplitN(v, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
