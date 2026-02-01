package router

import (
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

func setWebSPARoutes(r *gin.Engine, opts Options) {
	frontendBaseURL := strings.TrimRight(strings.TrimSpace(opts.FrontendBaseURL), "/")
	if frontendBaseURL != "" {
		r.NoRoute(func(c *gin.Context) {
			c.Redirect(http.StatusMovedPermanently, frontendBaseURL+c.Request.RequestURI)
		})
		return
	}

	// Align new-api: apply gzip only for static/SPAs (registered after API routes).
	r.Use(gzip.Gzip(gzip.DefaultCompression))

	if opts.FrontendFS != nil {
		if sub, err := fs.Sub(opts.FrontendFS, "web/dist"); err == nil {
			r.Use(static.Serve("/", &embedFileSystem{FileSystem: http.FS(sub)}))
		}
	} else if distDir := strings.TrimSpace(opts.FrontendDistDir); distDir != "" {
		r.Use(static.Serve("/", &hideRootFileSystem{ServeFileSystem: static.LocalFile(distDir, false)}))
	}

	indexPage := opts.FrontendIndexPage
	if len(indexPage) == 0 {
		indexPage = defaultIndexPage()
	}

	r.NoRoute(func(c *gin.Context) {
		if isAPIPrefix(c.Request.URL.Path) {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
	})
}

// embedFileSystem is a minimal static.ServeFileSystem wrapper for embed.FS sub folders.
// Credit: gin-contrib/static issue #19 pattern.
type embedFileSystem struct {
	http.FileSystem
}

func (e *embedFileSystem) Exists(prefix string, p string) bool {
	_, err := e.Open(p)
	return err == nil
}

func (e *embedFileSystem) Open(name string) (http.File, error) {
	if name == "/" {
		return nil, os.ErrNotExist
	}
	return e.FileSystem.Open(name)
}

type hideRootFileSystem struct {
	static.ServeFileSystem
}

func (h *hideRootFileSystem) Exists(prefix string, p string) bool {
	if strings.TrimSpace(p) == "" || p == "/" {
		return false
	}
	return h.ServeFileSystem.Exists(prefix, p)
}

func (h *hideRootFileSystem) Open(name string) (http.File, error) {
	if name == "/" {
		return nil, os.ErrNotExist
	}
	return h.ServeFileSystem.Open(name)
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
      <p style="margin: 0 0 12px;">前端构建产物未发现（默认路径：<code>web/dist</code>）。</p>
      <p style="margin: 0;">请先构建前端，再重新启动后端。</p>
    </div>
  </body>
</html>`)
}
