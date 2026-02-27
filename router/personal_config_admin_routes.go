package router

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func setAdminPersonalConfigAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/personal-config", adminPersonalConfigGetHandler(opts))
	r.PUT("/personal-config", adminPersonalConfigPutHandler(opts))
	r.POST("/personal-config/reload", adminPersonalConfigReloadHandler(opts))
}

func adminPersonalConfigGetHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.PersonalConfig == nil || !opts.PersonalConfig.Enabled() {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "personal config 未启用"})
			return
		}
		raw, sha, err := opts.PersonalConfig.ReadRaw(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取配置文件失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"enabled":    true,
				"path":       opts.PersonalConfig.Path(),
				"sha256":     sha,
				"bundle_json": string(raw),
			},
		})
	}
}

type personalConfigPutReq struct {
	BundleJSON string `json:"bundle_json"`
}

func adminPersonalConfigPutHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.PersonalConfig == nil || !opts.PersonalConfig.Enabled() {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "personal config 未启用"})
			return
		}
		var req personalConfigPutReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}
		raw := strings.TrimSpace(req.BundleJSON)
		if raw == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "bundle_json 不能为空"})
			return
		}
		sha, err := opts.PersonalConfig.ReplaceFileAndApply(c.Request.Context(), []byte(raw))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "写入配置文件失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"sha256": sha,
				"path":   opts.PersonalConfig.Path(),
			},
		})
	}
}

func adminPersonalConfigReloadHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.PersonalConfig == nil || !opts.PersonalConfig.Enabled() {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "personal config 未启用"})
			return
		}
		if err := opts.PersonalConfig.ApplyExternalChange(c.Request.Context()); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "重载失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}

