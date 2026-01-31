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

type userTokenView struct {
	ID         int64      `json:"id"`
	Name       *string    `json:"name,omitempty"`
	TokenHint  *string    `json:"token_hint,omitempty"`
	Status     int        `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func setTokenAPIRoutes(r gin.IRoutes, opts Options) {
	authn := requireUserSession(opts)

	r.GET("/token", authn, listUserTokensHandler(opts))
	r.GET("/token/", authn, listUserTokensHandler(opts))

	r.POST("/token", authn, createUserTokenHandler(opts))
	r.POST("/token/", authn, createUserTokenHandler(opts))

	r.POST("/token/:token_id/rotate", authn, rotateUserTokenHandler(opts))
	r.POST("/token/:token_id/revoke", authn, revokeUserTokenHandler(opts))
	r.DELETE("/token/:token_id", authn, deleteUserTokenHandler(opts))
}

func listUserTokensHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		tokens, err := opts.Store.ListUserTokens(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 Token 列表失败"})
			return
		}
		out := make([]userTokenView, 0, len(tokens))
		for _, t := range tokens {
			out = append(out, userTokenView{
				ID:         t.ID,
				Name:       t.Name,
				TokenHint:  t.TokenHint,
				Status:     t.Status,
				CreatedAt:  t.CreatedAt,
				RevokedAt:  t.RevokedAt,
				LastUsedAt: t.LastUsedAt,
			})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func createUserTokenHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Name *string `json:"name,omitempty"`
	}
	return func(c *gin.Context) {
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
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

		raw, err := auth.NewRandomToken("sk-", 32)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成令牌失败"})
			return
		}
		tokenID, hint, err := opts.Store.CreateUserToken(c.Request.Context(), userID, req.Name, raw)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建令牌失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"token_id":   tokenID,
				"token":      raw,
				"token_hint": hint,
			},
		})
	}
}

func rotateUserTokenHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		tokenID, err := strconv.ParseInt(strings.TrimSpace(c.Param("token_id")), 10, 64)
		if err != nil || tokenID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "token_id 不合法"})
			return
		}

		raw, err := auth.NewRandomToken("sk-", 32)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成令牌失败"})
			return
		}
		if err := opts.Store.RotateUserToken(c.Request.Context(), userID, tokenID, raw); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "令牌不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "重新生成失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"token_id": tokenID,
				"token":    raw,
			},
		})
	}
}

func revokeUserTokenHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		tokenID, err := strconv.ParseInt(strings.TrimSpace(c.Param("token_id")), 10, 64)
		if err != nil || tokenID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "token_id 不合法"})
			return
		}
		if err := opts.Store.RevokeUserToken(c.Request.Context(), userID, tokenID); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "撤销失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}

func deleteUserTokenHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		tokenID, err := strconv.ParseInt(strings.TrimSpace(c.Param("token_id")), 10, 64)
		if err != nil || tokenID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "token_id 不合法"})
			return
		}
		if err := opts.Store.DeleteUserToken(c.Request.Context(), userID, tokenID); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "令牌不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}
