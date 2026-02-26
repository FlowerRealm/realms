package router

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"realms/internal/auth"
)

type personalAPIKeyView struct {
	ID         int64      `json:"id"`
	Name       *string    `json:"name,omitempty"`
	KeyHint    *string    `json:"key_hint,omitempty"`
	Status     int        `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func setPersonalAPIKeyAPIRoutes(r gin.IRoutes, opts Options) {
	authn := requireRoot(opts)

	r.GET("/personal/keys", authn, listPersonalAPIKeysHandler(opts))
	r.GET("/personal/keys/", authn, listPersonalAPIKeysHandler(opts))

	r.POST("/personal/keys", authn, createPersonalAPIKeyHandler(opts))
	r.POST("/personal/keys/", authn, createPersonalAPIKeyHandler(opts))

	r.POST("/personal/keys/:key_id/revoke", authn, revokePersonalAPIKeyHandler(opts))
}

func listPersonalAPIKeysHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		keys, err := opts.Store.ListPersonalAPIKeys(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 Key 列表失败"})
			return
		}
		out := make([]personalAPIKeyView, 0, len(keys))
		for _, k := range keys {
			out = append(out, personalAPIKeyView{
				ID:         k.ID,
				Name:       k.Name,
				KeyHint:    k.KeyHint,
				Status:     k.Status,
				CreatedAt:  k.CreatedAt,
				RevokedAt:  k.RevokedAt,
				LastUsedAt: k.LastUsedAt,
			})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func createPersonalAPIKeyHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Name *string `json:"name,omitempty"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		var req reqBody
		_ = c.ShouldBindJSON(&req)
		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if name == "" {
				req.Name = nil
			} else {
				req.Name = &name
			}
		}

		raw, err := auth.NewRandomToken("pk_", 32)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成 Key 失败"})
			return
		}
		id, hint, err := opts.Store.CreatePersonalAPIKey(c.Request.Context(), req.Name, raw)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建 Key 失败"})
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"key_id":   id,
				"key":      raw,
				"key_hint": hint,
				"name":     req.Name,
			},
		})
	}
}

func revokePersonalAPIKeyHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		keyID, err := strconv.ParseInt(strings.TrimSpace(c.Param("key_id")), 10, 64)
		if err != nil || keyID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "key_id 不合法"})
			return
		}
		if err := opts.Store.RevokePersonalAPIKey(c.Request.Context(), keyID); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "Key 不存在或已撤销"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "撤销失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}
