// Package middleware 提供数据面 Token 鉴权：Authorization: Bearer <token> 或 x-api-key。
package middleware

import (
	"database/sql"
	"net/http"
	"strings"

	"realms/internal/auth"
	rlmcrypto "realms/internal/crypto"
	"realms/internal/store"
)

const (
	personalModeVirtualUserID  int64 = 1
	personalModeVirtualTokenID int64 = 1

	// personal 模式下的“数据面 API Key”（personal_api_keys）使用独立 token_id 区间，
	// 避免与管理 Key（virtual token id=1）混淆。
	personalModeAPIKeyTokenIDOffset int64 = 1_000_000
)

func TokenAuth(st *store.Store, personalMode bool) Middleware {
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

			if personalMode {
				if st == nil {
					http.Error(w, "鉴权失败", http.StatusInternalServerError)
					return
				}
				gotHash := rlmcrypto.TokenHash(raw)
				id, err := st.GetPersonalAPIKeyIDByHash(r.Context(), gotHash)
				if err != nil {
					if err == sql.ErrNoRows {
						http.Error(w, "Token 无效", http.StatusUnauthorized)
						return
					}
					http.Error(w, "鉴权失败", http.StatusInternalServerError)
					return
				}
				tokenID := personalModeAPIKeyTokenIDOffset + id
				p := auth.Principal{
					ActorType: auth.ActorTypeToken,
					UserID:    personalModeVirtualUserID,
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
