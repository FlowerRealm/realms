package router

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func requireCSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		want, ok := sessionCSRFToken(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "csrf token 缺失，请重新登录"})
			c.Abort()
			return
		}

		got := strings.TrimSpace(c.GetHeader("X-CSRF-Token"))
		if got == "" {
			got = strings.TrimSpace(c.PostForm("_csrf"))
		}
		if got == "" || got != want {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "csrf token 不正确"})
			c.Abort()
			return
		}
		c.Next()
	}
}

