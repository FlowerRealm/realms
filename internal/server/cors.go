package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type corsConfig struct {
	enabled  bool
	allowAll bool
	allow    map[string]struct{}
}

func parseCORSAllowOrigins(raw string) corsConfig {
	v := strings.TrimSpace(raw)
	if v == "" {
		return corsConfig{}
	}
	if v == "*" {
		return corsConfig{enabled: true, allowAll: true}
	}

	allow := map[string]struct{}{}
	for _, part := range strings.Split(v, ",") {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		allow[origin] = struct{}{}
	}
	if len(allow) == 0 {
		return corsConfig{}
	}
	return corsConfig{enabled: true, allow: allow}
}

// CORSMiddleware 启用最小可用的 CORS 支持（用于浏览器跨域访问）。
//
// 配置语义（rawAllowOrigins）:
// - 为空：禁用 CORS
// - "*": 允许任意 Origin（不启用 credentials）
// - 逗号分隔列表：精确匹配并回显 Origin（建议包含 scheme+host+port）
func CORSMiddleware(rawAllowOrigins string) gin.HandlerFunc {
	cfg := parseCORSAllowOrigins(rawAllowOrigins)
	if !cfg.enabled {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin == "" {
			c.Next()
			return
		}

		allowed := cfg.allowAll
		if !allowed {
			_, allowed = cfg.allow[origin]
		}
		if !allowed {
			c.Next()
			return
		}

		if cfg.allowAll {
			c.Header("Access-Control-Allow-Origin", "*")
		} else {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}

		isPreflight := c.Request.Method == http.MethodOptions &&
			strings.TrimSpace(c.GetHeader("Access-Control-Request-Method")) != ""
		if !isPreflight {
			c.Next()
			return
		}

		reqMethod := strings.TrimSpace(c.GetHeader("Access-Control-Request-Method"))
		reqHeaders := strings.TrimSpace(c.GetHeader("Access-Control-Request-Headers"))

		if reqMethod != "" {
			c.Header("Access-Control-Allow-Methods", reqMethod)
		}
		if reqHeaders != "" {
			c.Header("Access-Control-Allow-Headers", reqHeaders)
		} else {
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		}
		c.Header("Access-Control-Max-Age", "600")
		c.Status(http.StatusNoContent)
		c.Abort()
	}
}
