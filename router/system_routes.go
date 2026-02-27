package router

import (
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	rlmcrypto "realms/internal/crypto"
	"realms/internal/store"
)

func setSystemRoutes(r *gin.Engine, opts Options) {
	r.GET("/healthz", wrapHTTPFunc(opts.Healthz))

	r.GET("/api/meta", func(c *gin.Context) {
		personalModeKeySet := false
		if opts.PersonalMode && opts.Store != nil {
			if _, ok, err := opts.Store.GetPersonalModeKeyHash(c.Request.Context()); err == nil && ok {
				personalModeKeySet = true
			}
		}
		personalConfigEnabled := false
		personalConfigPath := ""
		personalConfigSHA := ""
		personalConfigErr := ""
		if opts.PersonalConfig != nil && opts.PersonalConfig.Enabled() {
			personalConfigEnabled = true
			personalConfigPath = opts.PersonalConfig.Path()
			personalConfigSHA = opts.PersonalConfig.CurrentSHA256()
			personalConfigErr = opts.PersonalConfig.LastError()
		}
		mode := "business"
		if opts.PersonalMode {
			mode = "personal"
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"mode":                  mode,
				"personal_mode_key_set": personalModeKeySet,
				"personal_config_enabled":    personalConfigEnabled,
				"personal_config_path":       personalConfigPath,
				"personal_config_sha256":     personalConfigSHA,
				"personal_config_last_error": personalConfigErr,
			},
		})
	})

	type bootstrapReq struct {
		Key string `json:"key"`
	}
	bootstrap := func(c *gin.Context) {
		if !opts.PersonalMode {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "not found"})
			return
		}
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if _, ok, err := opts.Store.GetPersonalModeKeyHash(c.Request.Context()); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "personal 模式 Key 状态异常"})
			return
		} else if ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "personal 模式 Key 已设置"})
			return
		}

		var req bootstrapReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}
		key := strings.TrimSpace(req.Key)
		if key == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "Key 不能为空"})
			return
		}
		hashHex := hex.EncodeToString(rlmcrypto.TokenHash(key))
		inserted, err := opts.Store.InsertAppSettingIfAbsent(c.Request.Context(), store.SettingPersonalModeKeyHash, hashHex)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "写入失败"})
			return
		}
		if !inserted {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "personal 模式 Key 已设置"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}

	// 新命名：personal 模式初始化 Key。
	r.POST("/api/personal/bootstrap", bootstrap)

	r.GET("/assets/realms_icon.svg", wrapHTTPFunc(opts.RealmsIconSVG))
	r.HEAD("/assets/realms_icon.svg", wrapHTTPFunc(opts.RealmsIconSVG))

	r.GET("/favicon.ico", wrapHTTPFunc(opts.FaviconICO))
	r.HEAD("/favicon.ico", wrapHTTPFunc(opts.FaviconICO))
}
