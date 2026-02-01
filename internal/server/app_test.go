package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	openaiapi "realms/internal/api/openai"
	"realms/internal/config"
	"realms/internal/quota"
	"realms/internal/store"
	"realms/internal/upstream"
	"realms/router"
)

func newTestApp(t *testing.T, cfg config.Config) *App {
	t.Helper()

	st := store.New(nil)

	openaiHandler := openaiapi.NewHandler(nil, nil, nil, nil, nil, nil, false, nil, nil, nil, upstream.SSEPumpOptions{})

	app := &App{
		cfg:    cfg,
		store:  st,
		openai: openaiHandler,
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	sessionStore := cookie.NewStore([]byte("test-secret"))
	sessionStore.Options(sessions.Options{
		Path:     "/",
		MaxAge:   2592000,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	engine.Use(sessions.Sessions(SessionCookieNameForSelfMode(cfg.SelfMode.Enable), sessionStore))

	router.SetRouter(engine, router.Options{
		Store:             st,
		SelfMode:          cfg.SelfMode.Enable,
		AllowOpenRegistration:         cfg.Security.AllowOpenRegistration,
		EmailVerificationEnabledDefault: cfg.EmailVerif.Enable,
		BillingDefault:     cfg.Billing,
		PaymentDefault:     cfg.Payment,
		SMTPDefault:        cfg.SMTP,
		OpenAI:            openaiHandler,
		FrontendIndexPage: []byte("<!doctype html><html><body>INDEX</body></html>"),

		Healthz: func(w http.ResponseWriter, r *http.Request) {
			out := map[string]any{"ok": true}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(out)
		},
		Version: func(w http.ResponseWriter, r *http.Request) {
			out := map[string]any{"ok": true}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(out)
		},
		RealmsIconSVG: app.handleRealmsIconSVG,
		FaviconICO:    app.handleFaviconICO,

		SubscriptionOrderPaidWebhook: app.handleSubscriptionOrderPaidWebhook,
	})
	app.engine = engine
	return app
}

func TestRoutes_SelfMode_DisablesBillingWebhooks(t *testing.T) {
	cfg := config.Config{
		SelfMode: config.SelfModeConfig{Enable: true},
		Security: config.SecurityConfig{SubscriptionOrderWebhookSecret: "secret"},
	}
	app := newTestApp(t, cfg)

	cases := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/webhooks/subscription-orders/1/paid"},
		{method: http.MethodPost, path: "/api/pay/stripe/webhook"},
		{method: http.MethodGet, path: "/api/pay/epay/notify"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, "http://example.com"+tc.path, nil)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s %s expected status %d, got %d", tc.method, tc.path, http.StatusNotFound, rr.Code)
		}
	}
}

func TestRoutes_DefaultMode_KeepsSubscriptionOrderWebhook(t *testing.T) {
	cfg := config.Config{
		SelfMode: config.SelfModeConfig{Enable: false},
		Security: config.SecurityConfig{SubscriptionOrderWebhookSecret: "secret"},
	}
	app := newTestApp(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/webhooks/subscription-orders/1/paid", nil)
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
	if got := rr.Header().Get("WWW-Authenticate"); got == "" {
		t.Fatalf("expected WWW-Authenticate header")
	}
}

func TestRoutes_SPAFallback(t *testing.T) {
	cfg := config.Config{
		SelfMode: config.SelfModeConfig{Enable: false},
		Security: config.SecurityConfig{SubscriptionOrderWebhookSecret: "secret"},
	}
	app := newTestApp(t, cfg)

	t.Run("GET /login serves index", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/login", nil)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("expected Content-Type text/html, got %q", ct)
		}
		if body := rr.Body.String(); !strings.Contains(body, "INDEX") {
			t.Fatalf("expected index html body, got %q", body)
		}
	})

	t.Run("GET /api/unknown returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/api/unknown", nil)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
		}
	})
}

func TestRoutes_NoChatFeature(t *testing.T) {
	cfg := config.Config{
		SelfMode: config.SelfModeConfig{Enable: false},
		Security: config.SecurityConfig{SubscriptionOrderWebhookSecret: "secret"},
	}
	app := newTestApp(t, cfg)

	t.Run("GET /chat serves index (SPA handles 404)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/chat", nil)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("POST /api/chat/token stays 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/api/chat/token", nil)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
		}
	})
}

func TestRoutes_Assets_IconAndFavicon(t *testing.T) {
	cfg := config.Config{}
	app := newTestApp(t, cfg)

	t.Run("GET /assets/realms_icon.svg", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/assets/realms_icon.svg", nil)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		ct := rr.Header().Get("Content-Type")
		if !strings.HasPrefix(ct, "image/svg+xml") {
			t.Fatalf("expected Content-Type image/svg+xml, got %q", ct)
		}
		if body := rr.Body.String(); !strings.Contains(body, "<svg") {
			t.Fatalf("expected svg body, got %q", body)
		}
	})

	t.Run("GET /favicon.ico", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/favicon.ico", nil)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusPermanentRedirect {
			t.Fatalf("expected status %d, got %d", http.StatusPermanentRedirect, rr.Code)
		}
		if got := rr.Header().Get("Location"); got != "/assets/realms_icon.svg" {
			t.Fatalf("expected Location %q, got %q", "/assets/realms_icon.svg", got)
		}
	})
}

func TestQuotaProviderForConfig(t *testing.T) {
	st := store.New(nil)

	qp := quotaProvider(st)
	if _, ok := qp.(*quota.FreeProvider); !ok {
		t.Fatalf("expected *quota.FreeProvider, got %T", qp)
	}
}
