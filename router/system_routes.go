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
		selfModeKeySet := false
		if opts.SelfMode && opts.Store != nil {
			if _, ok, err := opts.Store.GetSelfModeKeyHash(c.Request.Context()); err == nil && ok {
				selfModeKeySet = true
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"self_mode":         opts.SelfMode,
				"self_mode_key_set": selfModeKeySet,
			},
		})
	})

	type bootstrapReq struct {
		Key string `json:"key"`
	}
	r.POST("/api/self-mode/bootstrap", func(c *gin.Context) {
		if !opts.SelfMode {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "not found"})
			return
		}
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if _, ok, err := opts.Store.GetSelfModeKeyHash(c.Request.Context()); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "自用模式 Key 状态异常"})
			return
		} else if ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "自用模式 Key 已设置"})
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
		inserted, err := opts.Store.InsertAppSettingIfAbsent(c.Request.Context(), store.SettingSelfModeKeyHash, hashHex)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "写入失败"})
			return
		}
		if !inserted {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "自用模式 Key 已设置"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	})

	r.GET("/assets/realms_icon.svg", wrapHTTPFunc(opts.RealmsIconSVG))
	r.HEAD("/assets/realms_icon.svg", wrapHTTPFunc(opts.RealmsIconSVG))

	r.GET("/favicon.ico", wrapHTTPFunc(opts.FaviconICO))
	r.HEAD("/favicon.ico", wrapHTTPFunc(opts.FaviconICO))
}
