package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWebSPARoutes_APINoRouteWithGzip_NoClosedWriterError(t *testing.T) {
	prevMode := gin.Mode()
	gin.SetMode(gin.DebugMode)
	defer gin.SetMode(prevMode)

	prevWriter := gin.DefaultWriter
	logBuf := &bytes.Buffer{}
	gin.DefaultWriter = logBuf
	defer func() {
		gin.DefaultWriter = prevWriter
	}()

	engine := gin.New()
	engine.Use(gin.Recovery())
	setWebSPARoutes(engine, Options{
		FrontendIndexPage: []byte("<!doctype html><html><body>INDEX</body></html>"),
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/not-found", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}

	if strings.Contains(logBuf.String(), "cannot write message to writer during serve error") {
		t.Fatalf("unexpected gin closed-writer debug log: %s", logBuf.String())
	}
}

func TestWebSPARoutes_SPAFallbackLoadsLatestDistIndex(t *testing.T) {
	prevMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	defer gin.SetMode(prevMode)

	distDir := t.TempDir()
	writeIndex := func(tag string) {
		t.Helper()
		p := filepath.Join(distDir, "index.html")
		content := []byte("<!doctype html><html><body>INDEX-" + tag + "</body></html>")
		if err := os.WriteFile(p, content, 0o644); err != nil {
			t.Fatalf("write index: %v", err)
		}
	}

	writeIndex("A")

	engine := gin.New()
	engine.Use(gin.Recovery())
	setWebSPARoutes(engine, Options{
		FrontendDistDir: distDir,
	})

	assertIndex := func(path, marker string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
		rr := httptest.NewRecorder()
		engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("path=%s status=%d body=%q", path, rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), marker) {
			t.Fatalf("path=%s expected %q, got %q", path, marker, rr.Body.String())
		}
	}

	assertIndex("/login", "INDEX-A")
	assertIndex("/admin/models", "INDEX-A")

	writeIndex("B")

	assertIndex("/login", "INDEX-B")
	assertIndex("/admin/models", "INDEX-B")
}
