package router

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"realms/internal/auth"
	"realms/internal/store"
)

type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope,omitempty"`
}

type oauthErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

func oauthTokenHandler(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if opts.Store == nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "store 未初始化")
			return
		}
		if err := r.ParseForm(); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "表单解析失败")
			return
		}
		grantType := strings.TrimSpace(r.FormValue("grant_type"))
		if grantType != "authorization_code" {
			writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "仅支持 authorization_code")
			return
		}

		code := strings.TrimSpace(r.FormValue("code"))
		redirectURI := strings.TrimSpace(r.FormValue("redirect_uri"))
		clientID := strings.TrimSpace(r.FormValue("client_id"))
		clientSecret := strings.TrimSpace(r.FormValue("client_secret"))
		codeVerifier := strings.TrimSpace(r.FormValue("code_verifier"))

		if basicID, basicSecret, ok := parseBasicAuth(r); ok {
			if clientID != "" && clientID != basicID {
				writeOAuthError(w, http.StatusBadRequest, "invalid_request", "client_id 不一致")
				return
			}
			if clientID == "" {
				clientID = basicID
			}
			if clientSecret == "" {
				clientSecret = basicSecret
			}
		}

		if code == "" || clientID == "" || redirectURI == "" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "缺少必要参数")
			return
		}
		var err error
		redirectURI, err = store.NormalizeOAuthRedirectURI(redirectURI)
		if err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		app, ok, err := opts.Store.GetOAuthAppByClientID(r.Context(), clientID)
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "查询应用失败")
			return
		}
		if !ok || app.Status != store.OAuthAppStatusEnabled {
			writeOAuthError(w, http.StatusBadRequest, "invalid_client", "应用不存在或已停用")
			return
		}
		allowed, err := opts.Store.OAuthAppHasRedirectURI(r.Context(), app.ID, redirectURI)
		if err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "redirect_uri 校验失败")
			return
		}
		if !allowed {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "redirect_uri 未登记")
			return
		}

		if len(app.ClientSecretHash) > 0 {
			if clientSecret == "" {
				writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "缺少 client_secret")
				return
			}
			if !auth.CheckPassword(app.ClientSecretHash, clientSecret) {
				writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "client_secret 无效")
				return
			}
		}

		ac, ok, err := opts.Store.ConsumeOAuthAuthCode(r.Context(), code, app.ID, redirectURI)
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "授权码校验失败")
			return
		}
		if !ok {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "授权码无效或已过期")
			return
		}

		if len(app.ClientSecretHash) == 0 {
			if ac.CodeChallenge == nil {
				writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "该应用需要 PKCE")
				return
			}
			if codeVerifier == "" {
				writeOAuthError(w, http.StatusBadRequest, "invalid_request", "缺少 code_verifier")
				return
			}
		}
		if ac.CodeChallenge != nil {
			method := "S256"
			if ac.CodeChallengeMethod != nil && strings.TrimSpace(*ac.CodeChallengeMethod) != "" {
				method = strings.TrimSpace(*ac.CodeChallengeMethod)
			}
			if method != "S256" {
				writeOAuthError(w, http.StatusBadRequest, "invalid_request", "不支持的 code_challenge_method")
				return
			}
			if codeVerifier == "" {
				writeOAuthError(w, http.StatusBadRequest, "invalid_request", "缺少 code_verifier")
				return
			}
			if !verifyPKCES256(*ac.CodeChallenge, codeVerifier) {
				writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "code_verifier 无效")
				return
			}
		}

		rawToken, err := auth.NewRandomToken("rlm_", 32)
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "生成 token 失败")
			return
		}
		name := "oauth:" + app.ClientID
		tokenID, _, err := opts.Store.CreateUserToken(r.Context(), ac.UserID, &name, rawToken)
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "创建 token 失败")
			return
		}
		if err := opts.Store.CreateOAuthAppToken(r.Context(), app.ID, ac.UserID, tokenID, ac.Scope); err != nil {
			_ = opts.Store.DeleteUserToken(r.Context(), ac.UserID, tokenID)
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "写入授权记录失败")
			return
		}

		resp := oauthTokenResponse{
			AccessToken: rawToken,
			TokenType:   "bearer",
			Scope:       ac.Scope,
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func verifyPKCES256(codeChallenge string, codeVerifier string) bool {
	sum := sha256.Sum256([]byte(codeVerifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	return want == strings.TrimSpace(codeChallenge)
}

func parseBasicAuth(r *http.Request) (string, string, bool) {
	if r == nil {
		return "", "", false
	}
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(h, "Basic ") {
		return "", "", false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(h, "Basic "))
	if raw == "" {
		return "", "", false
	}
	dec, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(string(dec), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	id := strings.TrimSpace(parts[0])
	secret := strings.TrimSpace(parts[1])
	if id == "" || secret == "" {
		return "", "", false
	}
	return id, secret, true
}

func writeOAuthError(w http.ResponseWriter, status int, code string, desc string) {
	if status == 0 {
		status = http.StatusBadRequest
	}
	if strings.TrimSpace(code) == "" {
		code = "invalid_request"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(oauthErrorResponse{Error: code, ErrorDescription: strings.TrimSpace(desc)})
}

