package web

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"realms/internal/auth"
	"realms/internal/crypto"
	"realms/internal/store"
)

func (s *Server) OAuthAuthorizePage(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.UserID == 0 {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	responseType := strings.TrimSpace(q.Get("response_type"))
	clientID := strings.TrimSpace(q.Get("client_id"))
	redirectURI := strings.TrimSpace(q.Get("redirect_uri"))
	scope := strings.TrimSpace(q.Get("scope"))
	state := strings.TrimSpace(q.Get("state"))
	codeChallenge := strings.TrimSpace(q.Get("code_challenge"))
	codeChallengeMethod := strings.TrimSpace(q.Get("code_challenge_method"))

	if responseType != "code" {
		http.Error(w, "不支持的 response_type", http.StatusBadRequest)
		return
	}
	if clientID == "" {
		http.Error(w, "缺少 client_id", http.StatusBadRequest)
		return
	}
	if state == "" {
		http.Error(w, "缺少 state", http.StatusBadRequest)
		return
	}
	var err error
	redirectURI, err = store.NormalizeOAuthRedirectURI(redirectURI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	scope, err = store.NormalizeOAuthScope(scope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	app, ok, err := s.store.GetOAuthAppByClientID(r.Context(), clientID)
	if err != nil {
		http.Error(w, "查询应用失败", http.StatusInternalServerError)
		return
	}
	if !ok || app.Status != store.OAuthAppStatusEnabled {
		http.Error(w, "应用不存在或已停用", http.StatusBadRequest)
		return
	}
	allowed, err := s.store.OAuthAppHasRedirectURI(r.Context(), app.ID, redirectURI)
	if err != nil {
		http.Error(w, "redirect_uri 校验失败", http.StatusBadRequest)
		return
	}
	if !allowed {
		http.Error(w, "redirect_uri 未登记", http.StatusBadRequest)
		return
	}

	grant, hasGrant, err := s.store.GetOAuthUserGrant(r.Context(), p.UserID, app.ID)
	if err != nil {
		http.Error(w, "查询授权记录失败", http.StatusInternalServerError)
		return
	}
	if hasGrant && strings.TrimSpace(grant.Scope) == scope {
		s.issueOAuthAuthCodeAndRedirect(w, r, app.ID, p.UserID, redirectURI, scope, state, nullableNonEmpty(codeChallenge), nullableNonEmpty(codeChallengeMethod))
		return
	}

	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	s.Render(w, "page_oauth_consent", s.withFeatures(r.Context(), TemplateData{
		Title:                    "应用授权 - Realms",
		User:                     userViewFromUser(u),
		CSRFToken:                csrfToken(p),
		OAuthAppName:             app.Name,
		OAuthClientID:            app.ClientID,
		OAuthRedirectURI:         redirectURI,
		OAuthScope:               scope,
		OAuthState:               state,
		OAuthCodeChallenge:       codeChallenge,
		OAuthCodeChallengeMethod: codeChallengeMethod,
	}))
}

func (s *Server) OAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.UserID == 0 {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	clientID := strings.TrimSpace(r.FormValue("client_id"))
	redirectURI := strings.TrimSpace(r.FormValue("redirect_uri"))
	scope := strings.TrimSpace(r.FormValue("scope"))
	state := strings.TrimSpace(r.FormValue("state"))
	decision := strings.TrimSpace(r.FormValue("decision"))
	remember := strings.TrimSpace(r.FormValue("remember")) != ""
	codeChallenge := strings.TrimSpace(r.FormValue("code_challenge"))
	codeChallengeMethod := strings.TrimSpace(r.FormValue("code_challenge_method"))

	if clientID == "" || state == "" {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	var err error
	redirectURI, err = store.NormalizeOAuthRedirectURI(redirectURI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	scope, err = store.NormalizeOAuthScope(scope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	app, ok, err := s.store.GetOAuthAppByClientID(r.Context(), clientID)
	if err != nil {
		http.Error(w, "查询应用失败", http.StatusInternalServerError)
		return
	}
	if !ok || app.Status != store.OAuthAppStatusEnabled {
		http.Error(w, "应用不存在或已停用", http.StatusBadRequest)
		return
	}
	allowed, err := s.store.OAuthAppHasRedirectURI(r.Context(), app.ID, redirectURI)
	if err != nil {
		http.Error(w, "redirect_uri 校验失败", http.StatusBadRequest)
		return
	}
	if !allowed {
		http.Error(w, "redirect_uri 未登记", http.StatusBadRequest)
		return
	}

	switch decision {
	case "approve":
		if remember {
			_ = s.store.UpsertOAuthUserGrant(r.Context(), p.UserID, app.ID, scope)
		}
		s.issueOAuthAuthCodeAndRedirect(w, r, app.ID, p.UserID, redirectURI, scope, state, nullableNonEmpty(codeChallenge), nullableNonEmpty(codeChallengeMethod))
		return
	case "deny":
		u, _ := url.Parse(redirectURI)
		q := u.Query()
		q.Set("error", "access_denied")
		q.Set("state", state)
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
		return
	default:
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
}

func (s *Server) issueOAuthAuthCodeAndRedirect(w http.ResponseWriter, r *http.Request, appID int64, userID int64, redirectURI string, scope string, state string, codeChallenge *string, codeChallengeMethod *string) {
	if r == nil || r.URL == nil {
		http.Error(w, "请求无效", http.StatusBadRequest)
		return
	}
	code, err := auth.NewRandomToken("oc_", 32)
	if err != nil {
		http.Error(w, "生成授权码失败", http.StatusInternalServerError)
		return
	}
	expiresAt := time.Now().Add(10 * time.Minute)
	if _, err := s.store.InsertOAuthAuthCode(r.Context(), crypto.TokenHash(code), appID, userID, redirectURI, scope, codeChallenge, codeChallengeMethod, expiresAt); err != nil {
		http.Error(w, "写入授权码失败", http.StatusInternalServerError)
		return
	}

	u, _ := url.Parse(redirectURI)
	q := u.Query()
	q.Set("code", code)
	q.Set("state", state)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope,omitempty"`
}

type oauthErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

func (s *Server) OAuthToken(w http.ResponseWriter, r *http.Request) {
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

	app, ok, err := s.store.GetOAuthAppByClientID(r.Context(), clientID)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "查询应用失败")
		return
	}
	if !ok || app.Status != store.OAuthAppStatusEnabled {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client", "应用不存在或已停用")
		return
	}
	allowed, err := s.store.OAuthAppHasRedirectURI(r.Context(), app.ID, redirectURI)
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

	ac, ok, err := s.store.ConsumeOAuthAuthCode(r.Context(), code, app.ID, redirectURI)
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
	tokenID, _, err := s.store.CreateUserToken(r.Context(), ac.UserID, &name, rawToken)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "创建 token 失败")
		return
	}
	if err := s.store.CreateOAuthAppToken(r.Context(), app.ID, ac.UserID, tokenID, ac.Scope); err != nil {
		_ = s.store.DeleteUserToken(r.Context(), ac.UserID, tokenID)
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

func nullableNonEmpty(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}
