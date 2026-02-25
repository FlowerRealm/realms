// Package middleware 提供数据面 Token 鉴权：Authorization: Bearer <token> 或 x-api-key。
package middleware

import (
	"crypto/subtle"
	"database/sql"
	"net/http"
	"strings"

	"realms/internal/auth"
	rlmcrypto "realms/internal/crypto"
	"realms/internal/store"
)

const (
	selfModeVirtualUserID  int64 = 1
	selfModeVirtualTokenID int64 = 1
)

func TokenAuth(st *store.Store, selfMode bool) Middleware {
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

			if selfMode {
				if st == nil {
					http.Error(w, "鉴权失败", http.StatusInternalServerError)
					return
				}
				expectHash, ok, err := st.GetSelfModeKeyHash(r.Context())
				if err != nil {
					http.Error(w, "鉴权失败", http.StatusInternalServerError)
					return
				}
				if !ok {
					http.Error(w, "自用模式尚未设置 Key", http.StatusUnauthorized)
					return
				}
				if subtle.ConstantTimeCompare(rlmcrypto.TokenHash(raw), expectHash) != 1 {
					http.Error(w, "Token 无效", http.StatusUnauthorized)
					return
				}
				tokenID := selfModeVirtualTokenID
				p := auth.Principal{
					ActorType: auth.ActorTypeToken,
					UserID:    selfModeVirtualUserID,
					TokenID:   &tokenID,
					Role:      store.UserRoleRoot,
				}
				next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
				return
			}

			if st == nil {
				http.Error(w, "鉴权失败", http.StatusInternalServerError)
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
