package router

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"realms/internal/auth"
	"realms/internal/crypto"
	"realms/internal/store"
)

type oauthAuthorizePrepareResponse struct {
	AppName             string `json:"app_name"`
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	Scope               string `json:"scope"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge,omitempty"`
	CodeChallengeMethod string `json:"code_challenge_method,omitempty"`
	RedirectTo          string `json:"redirect_to,omitempty"`
}

func setOAuthAPIRoutes(r gin.IRoutes, opts Options) {
	authn := requireUserSession(opts)

	r.GET("/oauth/authorize", authn, oauthAuthorizePrepareHandler(opts))
	r.POST("/oauth/authorize", authn, oauthAuthorizeDecisionHandler(opts))
}

func oauthAuthorizePrepareHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		userID, ok := userIDFromContext(c)
		if !ok || userID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		q := c.Request.URL.Query()
		responseType := strings.TrimSpace(q.Get("response_type"))
		clientID := strings.TrimSpace(q.Get("client_id"))
		redirectURI := strings.TrimSpace(q.Get("redirect_uri"))
		scope := strings.TrimSpace(q.Get("scope"))
		state := strings.TrimSpace(q.Get("state"))
		codeChallenge := strings.TrimSpace(q.Get("code_challenge"))
		codeChallengeMethod := strings.TrimSpace(q.Get("code_challenge_method"))

		if responseType != "code" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "不支持的 response_type"})
			return
		}
		if clientID == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "缺少 client_id"})
			return
		}
		if state == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "缺少 state"})
			return
		}

		var err error
		redirectURI, err = store.NormalizeOAuthRedirectURI(redirectURI)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		scope, err = store.NormalizeOAuthScope(scope)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}

		app, ok, err := opts.Store.GetOAuthAppByClientID(c.Request.Context(), clientID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询应用失败"})
			return
		}
		if !ok || app.Status != store.OAuthAppStatusEnabled {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "应用不存在或已停用"})
			return
		}

		allowed, err := opts.Store.OAuthAppHasRedirectURI(c.Request.Context(), app.ID, redirectURI)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "redirect_uri 校验失败"})
			return
		}
		if !allowed {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "redirect_uri 未登记"})
			return
		}

		grant, hasGrant, err := opts.Store.GetOAuthUserGrant(c.Request.Context(), userID, app.ID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询授权记录失败"})
			return
		}

		resp := oauthAuthorizePrepareResponse{
			AppName:             strings.TrimSpace(app.Name),
			ClientID:            app.ClientID,
			RedirectURI:         redirectURI,
			Scope:               scope,
			State:               state,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
		}

		if hasGrant && strings.TrimSpace(grant.Scope) == scope {
			redirectTo, err := issueOAuthAuthCodeRedirect(c.Request.Context(), opts.Store, app.ID, userID, redirectURI, scope, state, nullableNonEmpty(codeChallenge), nullableNonEmpty(codeChallengeMethod))
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
				return
			}
			resp.RedirectTo = redirectTo
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": resp})
	}
}

func oauthAuthorizeDecisionHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		ClientID            string `json:"client_id"`
		RedirectURI         string `json:"redirect_uri"`
		Scope               string `json:"scope"`
		State               string `json:"state"`
		Decision            string `json:"decision"`
		Remember            bool   `json:"remember"`
		CodeChallenge       string `json:"code_challenge"`
		CodeChallengeMethod string `json:"code_challenge_method"`
	}

	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok || userID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		clientID := strings.TrimSpace(req.ClientID)
		redirectURI := strings.TrimSpace(req.RedirectURI)
		scope := strings.TrimSpace(req.Scope)
		state := strings.TrimSpace(req.State)
		decision := strings.TrimSpace(req.Decision)
		codeChallenge := strings.TrimSpace(req.CodeChallenge)
		codeChallengeMethod := strings.TrimSpace(req.CodeChallengeMethod)

		if clientID == "" || state == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}
		var err error
		redirectURI, err = store.NormalizeOAuthRedirectURI(redirectURI)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		scope, err = store.NormalizeOAuthScope(scope)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}

		app, ok, err := opts.Store.GetOAuthAppByClientID(c.Request.Context(), clientID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询应用失败"})
			return
		}
		if !ok || app.Status != store.OAuthAppStatusEnabled {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "应用不存在或已停用"})
			return
		}
		allowed, err := opts.Store.OAuthAppHasRedirectURI(c.Request.Context(), app.ID, redirectURI)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "redirect_uri 校验失败"})
			return
		}
		if !allowed {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "redirect_uri 未登记"})
			return
		}

		switch decision {
		case "approve":
			if req.Remember {
				_ = opts.Store.UpsertOAuthUserGrant(c.Request.Context(), userID, app.ID, scope)
			}
			redirectTo, err := issueOAuthAuthCodeRedirect(c.Request.Context(), opts.Store, app.ID, userID, redirectURI, scope, state, nullableNonEmpty(codeChallenge), nullableNonEmpty(codeChallengeMethod))
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"redirect_to": redirectTo}})
			return
		case "deny":
			u, _ := url.Parse(redirectURI)
			q := u.Query()
			q.Set("error", "access_denied")
			q.Set("state", state)
			u.RawQuery = q.Encode()
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"redirect_to": u.String()}})
			return
		default:
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}
	}
}

func issueOAuthAuthCodeRedirect(ctx context.Context, st *store.Store, appID int64, userID int64, redirectURI string, scope string, state string, codeChallenge *string, codeChallengeMethod *string) (string, error) {
	code, err := auth.NewRandomToken("oc_", 32)
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().Add(10 * time.Minute)
	if _, err := st.InsertOAuthAuthCode(ctx, crypto.TokenHash(code), appID, userID, redirectURI, scope, codeChallenge, codeChallengeMethod, expiresAt); err != nil {
		return "", err
	}

	u, _ := url.Parse(redirectURI)
	q := u.Query()
	q.Set("code", code)
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func nullableNonEmpty(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}
