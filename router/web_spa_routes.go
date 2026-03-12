package router

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func serveEmbeddedSPA(frontendFS fs.FS) gin.HandlerFunc {
	fileServer := http.FileServer(http.FS(frontendFS))
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		p := strings.TrimSpace(c.Request.URL.Path)
		if p == "" || p == "/" || strings.HasSuffix(p, "/") {
			c.Next()
			return
		}
		if _, err := fs.Stat(frontendFS, strings.TrimPrefix(p, "/")); err != nil {
			c.Next()
			return
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}

func setWebSPARoutes(r *gin.Engine, opts Options) {
	// Align new-api: apply gzip only for static/SPAs (registered after API routes).
	r.Use(gzip.Gzip(gzip.DefaultCompression))

	if opts.FrontendFS != nil {
		r.Use(serveEmbeddedSPA(opts.FrontendFS))
	}

	fallbackIndexPage := defaultIndexPage()

	r.NoRoute(func(c *gin.Context) {
		p := strings.TrimSpace(c.Request.URL.Path)
		if p == "/mcp" || strings.HasPrefix(p, "/mcp/") || p == "/skills" || strings.HasPrefix(p, "/skills/") || p == "/admin/mcp" || strings.HasPrefix(p, "/admin/mcp/") {
			c.Data(http.StatusNotFound, "text/plain; charset=utf-8", []byte("Not Found"))
			return
		}
		if isAPIPrefix(p) {
			c.Data(http.StatusNotFound, "text/plain; charset=utf-8", []byte("Not Found"))
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", resolveSPAIndexPage(opts, fallbackIndexPage))
	})
}

func resolveSPAIndexPage(opts Options, fallback []byte) []byte {
	if len(opts.FrontendIndexPage) > 0 {
		return opts.FrontendIndexPage
	}
	if opts.FrontendFS != nil {
		if b, err := fs.ReadFile(opts.FrontendFS, "index.html"); err == nil && len(b) > 0 {
			return b
		}
	}
	return fallback
}

func isAPIPrefix(p string) bool {
	p = strings.TrimSpace(p)
	switch {
	case strings.HasPrefix(p, "/v1"):
		return true
	case strings.HasPrefix(p, "/v1beta"):
		return true
	case strings.HasPrefix(p, "/api"):
		return true
	case strings.HasPrefix(p, "/oauth/authorize"):
		return false
	case strings.HasPrefix(p, "/oauth"):
		return true
	case strings.HasPrefix(p, "/assets"):
		return true
	case p == "/healthz":
		return true
	default:
		return false
	}
}

func defaultIndexPage() []byte {
	return []byte(`<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Realms</title>
  </head>
  <body>
    <div style="font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif; max-width: 720px; margin: 40px auto; padding: 0 16px;">
      <h1 style="margin: 0 0 12px;">Realms</h1>
      <p style="margin: 0 0 12px;">前端构建产物未发现。</p>
      <p style="margin: 0;">请先构建并嵌入前端产物，再重新启动后端。</p>
    </div>
  </body>
</html>`)
}
