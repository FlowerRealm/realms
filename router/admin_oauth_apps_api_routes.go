package router

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"realms/internal/auth"
	"realms/internal/store"
)

type adminOAuthAppView struct {
	ID           int64    `json:"id"`
	ClientID     string   `json:"client_id"`
	Name         string   `json:"name"`
	Status       int      `json:"status"`
	StatusLabel  string   `json:"status_label"`
	HasSecret    bool     `json:"has_secret"`
	RedirectURIs []string `json:"redirect_uris"`
}

func setAdminOAuthAppAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/oauth-apps", adminListOAuthAppsHandler(opts))
	r.POST("/oauth-apps", adminCreateOAuthAppHandler(opts))
	r.GET("/oauth-apps/:app_id", adminGetOAuthAppHandler(opts))
	r.PUT("/oauth-apps/:app_id", adminUpdateOAuthAppHandler(opts))
	r.POST("/oauth-apps/:app_id/rotate-secret", adminRotateOAuthAppSecretHandler(opts))
}

func adminListOAuthAppsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		apps, err := opts.Store.ListOAuthApps(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 OAuth Apps 失败"})
			return
		}
		out := make([]adminOAuthAppView, 0, len(apps))
		for _, a := range apps {
			uris, _ := opts.Store.ListOAuthAppRedirectURIs(c.Request.Context(), a.ID)
			statusLabel := "停用"
			if a.Status == store.OAuthAppStatusEnabled {
				statusLabel = "启用"
			}
			out = append(out, adminOAuthAppView{
				ID:           a.ID,
				ClientID:     a.ClientID,
				Name:         a.Name,
				Status:       a.Status,
				StatusLabel:  statusLabel,
				HasSecret:    len(a.ClientSecretHash) > 0,
				RedirectURIs: uris,
			})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func adminCreateOAuthAppHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Name         string   `json:"name"`
		Status       int      `json:"status"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		name := strings.TrimSpace(req.Name)
		if name == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "应用名称不能为空"})
			return
		}
		status := req.Status
		if status != store.OAuthAppStatusEnabled && status != store.OAuthAppStatusDisabled {
			status = store.OAuthAppStatusEnabled
		}
		redirectURIs, err := normalizeRedirectURIs(req.RedirectURIs)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}

		clientID, err := auth.NewRandomToken("oa_", 16)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成 client_id 失败"})
			return
		}
		clientSecret, err := auth.NewRandomToken("oas_", 32)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成 client_secret 失败"})
			return
		}
		secretHash, err := auth.HashPassword(clientSecret)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成 client_secret 失败"})
			return
		}

		appID, err := opts.Store.CreateOAuthApp(c.Request.Context(), clientID, name, secretHash, status)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败"})
			return
		}
		if err := opts.Store.ReplaceOAuthAppRedirectURIs(c.Request.Context(), appID, redirectURIs); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "已创建",
			"data": gin.H{
				"id":            appID,
				"client_id":     clientID,
				"client_secret": clientSecret,
			},
		})
	}
}

func adminGetOAuthAppHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		appID, err := strconv.ParseInt(strings.TrimSpace(c.Param("app_id")), 10, 64)
		if err != nil || appID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "app_id 不合法"})
			return
		}

		a, err := opts.Store.GetOAuthAppByID(c.Request.Context(), appID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}
		uris, _ := opts.Store.ListOAuthAppRedirectURIs(c.Request.Context(), a.ID)
		statusLabel := "停用"
		if a.Status == store.OAuthAppStatusEnabled {
			statusLabel = "启用"
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": adminOAuthAppView{
			ID:           a.ID,
			ClientID:     a.ClientID,
			Name:         a.Name,
			Status:       a.Status,
			StatusLabel:  statusLabel,
			HasSecret:    len(a.ClientSecretHash) > 0,
			RedirectURIs: uris,
		}})
	}
}

func adminUpdateOAuthAppHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Name         string   `json:"name"`
		Status       int      `json:"status"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		appID, err := strconv.ParseInt(strings.TrimSpace(c.Param("app_id")), 10, 64)
		if err != nil || appID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "app_id 不合法"})
			return
		}
		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "应用名称不能为空"})
			return
		}
		status := req.Status
		if status != store.OAuthAppStatusEnabled && status != store.OAuthAppStatusDisabled {
			status = store.OAuthAppStatusEnabled
		}
		redirectURIs, err := normalizeRedirectURIs(req.RedirectURIs)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		if err := opts.Store.UpdateOAuthApp(c.Request.Context(), appID, name, status); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}
		if err := opts.Store.ReplaceOAuthAppRedirectURIs(c.Request.Context(), appID, redirectURIs); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func adminRotateOAuthAppSecretHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		appID, err := strconv.ParseInt(strings.TrimSpace(c.Param("app_id")), 10, 64)
		if err != nil || appID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "app_id 不合法"})
			return
		}
		if _, err := opts.Store.GetOAuthAppByID(c.Request.Context(), appID); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		clientSecret, err := auth.NewRandomToken("oas_", 32)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成 client_secret 失败"})
			return
		}
		secretHash, err := auth.HashPassword(clientSecret)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成 client_secret 失败"})
			return
		}
		if err := opts.Store.UpdateOAuthAppSecretHash(c.Request.Context(), appID, secretHash); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "轮换失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "已轮换",
			"data": gin.H{
				"client_secret": clientSecret,
			},
		})
	}
}

func normalizeRedirectURIs(raw []string) ([]string, error) {
	var out []string
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		norm, err := store.NormalizeOAuthRedirectURI(s)
		if err != nil {
			return nil, err
		}
		out = append(out, norm)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("redirect_uri 不能为空")
	}
	return out, nil
}

