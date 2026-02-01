package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func wrapHTTP(h http.Handler) gin.HandlerFunc {
	if h == nil {
		return func(c *gin.Context) {
			c.Status(http.StatusNotFound)
		}
	}

	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func wrapHTTPFunc(f http.HandlerFunc) gin.HandlerFunc {
	return wrapHTTP(f)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
