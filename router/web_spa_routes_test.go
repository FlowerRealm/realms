package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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
