package router

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func wrapHTTP(h http.Handler) gin.HandlerFunc {
	if h == nil {
		return func(c *gin.Context) {
			c.Status(http.StatusNotFound)
		}
	}

	return func(c *gin.Context) {
		// 兼容 net/http ServeMux 的 r.PathValue(...)：把 gin 的 path params 写回 *http.Request。
		//
		// 注意：gin 的 catch-all（*path）值以 "/" 开头；ServeMux 的 "{path...}" 不带前导 "/"
		// 因此这里统一去掉前导 "/"，避免内部 handler 取值不一致。
		for _, p := range c.Params {
			v := p.Value
			if strings.HasPrefix(v, "/") {
				v = strings.TrimPrefix(v, "/")
			}
			c.Request.SetPathValue(p.Key, v)
		}

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
