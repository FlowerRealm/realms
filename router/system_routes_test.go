package router

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDebugRoutes_DisabledByDefault(t *testing.T) {
	t.Setenv("REALMS_DEBUG_ROUTES", "")
	t.Setenv("REALMS_DEBUG_ROUTES_ALLOW_CIDRS", "")
	t.Setenv("REALMS_DEBUG_ROUTES_TOKEN", "")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	setSystemRoutes(r, Options{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/debug/vars", nil)
	r.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDebugRoutes_GuardBlocksNonLocalWithoutAllowlistOrToken(t *testing.T) {
	t.Setenv("REALMS_DEBUG_ROUTES", "1")
	t.Setenv("REALMS_DEBUG_ROUTES_ALLOW_CIDRS", "")
	t.Setenv("REALMS_DEBUG_ROUTES_TOKEN", "")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	setSystemRoutes(r, Options{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/debug/vars", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestDebugRoutes_GuardAllowsCIDR(t *testing.T) {
	t.Setenv("REALMS_DEBUG_ROUTES", "1")
	t.Setenv("REALMS_DEBUG_ROUTES_ALLOW_CIDRS", "8.8.8.0/24")
	t.Setenv("REALMS_DEBUG_ROUTES_TOKEN", "")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	setSystemRoutes(r, Options{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/debug/vars", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDebugRoutes_GuardAllowsToken(t *testing.T) {
	t.Setenv("REALMS_DEBUG_ROUTES", "1")
	t.Setenv("REALMS_DEBUG_ROUTES_ALLOW_CIDRS", "")
	t.Setenv("REALMS_DEBUG_ROUTES_TOKEN", "secret")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	setSystemRoutes(r, Options{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/debug/vars", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	req.Header.Set("X-Realms-Debug-Token", "secret")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/debug/vars", nil)
	req2.RemoteAddr = "8.8.8.8:1234"
	req2.Header.Set("X-Realms-Debug-Token", "wrong")
	r.ServeHTTP(w2, req2)
	if w2.Code != 403 {
		t.Fatalf("expected 403, got %d", w2.Code)
	}
}

func TestDebugRoutes_GuardUsesXFFOnlyFromTrustedProxy(t *testing.T) {
	t.Setenv("REALMS_DEBUG_ROUTES", "1")
	t.Setenv("REALMS_DEBUG_ROUTES_ALLOW_CIDRS", "1.2.3.0/24")
	t.Setenv("REALMS_DEBUG_ROUTES_TOKEN", "")
	t.Setenv("REALMS_TRUST_PROXY_HEADERS", "true")
	t.Setenv("REALMS_TRUSTED_PROXY_CIDRS", "10.0.0.0/8")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	setSystemRoutes(r, Options{})

	// Untrusted proxy: RemoteAddr is used, so XFF shouldn't help.
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/debug/vars", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	// Trusted proxy: XFF is used.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/debug/vars", nil)
	req2.RemoteAddr = "10.1.2.3:1234"
	req2.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
}
