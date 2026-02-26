package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSMiddleware_Disabled_DoesNotInterceptOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORSMiddleware(""))
	r.OPTIONS("/x", func(c *gin.Context) { c.Status(418) })
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodOptions, "http://example.com/x", nil)
	req.Header.Set("Origin", "https://a.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != 418 {
		t.Fatalf("expected status %d, got %d", 418, rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no Access-Control-Allow-Origin, got %q", got)
	}
}

func TestCORSMiddleware_AllowAll_SetsHeadersAndInterceptsPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORSMiddleware("*"))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	t.Run("simple get", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/x", nil)
		req.Header.Set("Origin", "https://a.example")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("expected status %d, got %d", 200, rr.Code)
		}
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Fatalf("expected Access-Control-Allow-Origin '*', got %q", got)
		}
	})

	t.Run("preflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "http://example.com/x", nil)
		req.Header.Set("Origin", "https://a.example")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "X-Test")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
		}
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Fatalf("expected Access-Control-Allow-Origin '*', got %q", got)
		}
		if got := rr.Header().Get("Access-Control-Allow-Methods"); got != "POST" {
			t.Fatalf("expected Access-Control-Allow-Methods 'POST', got %q", got)
		}
		if got := rr.Header().Get("Access-Control-Allow-Headers"); got != "X-Test" {
			t.Fatalf("expected Access-Control-Allow-Headers 'X-Test', got %q", got)
		}
	})
}

func TestCORSMiddleware_Whitelist_ReflectsOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORSMiddleware("https://a.example, https://b.example"))
	r.OPTIONS("/x", func(c *gin.Context) { c.Status(418) })
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	t.Run("allowed origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/x", nil)
		req.Header.Set("Origin", "https://a.example")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("expected status %d, got %d", 200, rr.Code)
		}
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://a.example" {
			t.Fatalf("expected Access-Control-Allow-Origin to reflect origin, got %q", got)
		}
		if got := rr.Header().Get("Vary"); got != "Origin" {
			t.Fatalf("expected Vary 'Origin', got %q", got)
		}
	})

	t.Run("disallowed preflight not intercepted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "http://example.com/x", nil)
		req.Header.Set("Origin", "https://c.example")
		req.Header.Set("Access-Control-Request-Method", "POST")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != 418 {
			t.Fatalf("expected status %d, got %d", 418, rr.Code)
		}
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("expected no Access-Control-Allow-Origin, got %q", got)
		}
	})
}
