package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

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

func TestWebSPARoutes_EmbeddedSPAAndFallback(t *testing.T) {
	prevMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	defer gin.SetMode(prevMode)

	embedded := fstest.MapFS{
		"index.html":     {Data: []byte("<!doctype html><html><body>INDEX-EMBED</body></html>")},
		"assets/app.js":  {Data: []byte("console.log('ok')")},
		"assets/app.css": {Data: []byte("body{color:black}")},
		"favicon.ico":    {Data: []byte("ico")},
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	setWebSPARoutes(engine, Options{
		FrontendFS: embedded,
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

	assertIndex("/login", "INDEX-EMBED")
	assertIndex("/admin/models", "INDEX-EMBED")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/assets/app.js", nil)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("path=/assets/app.js status=%d body=%q", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "console.log('ok')") {
		t.Fatalf("expected embedded asset, got %q", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/skills", nil)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("path=/skills status=%d body=%q", rr.Code, rr.Body.String())
	}
}
