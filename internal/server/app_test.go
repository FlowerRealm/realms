package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"realms/internal/admin"
	openaiapi "realms/internal/api/openai"
	"realms/internal/config"
	"realms/internal/limits"
	"realms/internal/quota"
	"realms/internal/store"
	"realms/internal/upstream"
	"realms/internal/web"
)

func newTestApp(t *testing.T, cfg config.Config) *App {
	t.Helper()

	st := store.New(nil)

	webServer, err := web.NewServer(
		st,
		nil,
		nil,
		cfg.SelfMode.Enable,
		cfg.Security.AllowOpenRegistration,
		cfg.Security.DisableSecureCookies,
		cfg.Billing,
		cfg.Payment,
		cfg.SMTP,
		cfg.EmailVerif.Enable,
		cfg.Server.PublicBaseURL,
		cfg.Security.TrustProxyHeaders,
		cfg.Security.TrustedProxyCIDRs,
		cfg.Tickets,
		nil,
	)
	if err != nil {
		t.Fatalf("web.NewServer failed: %v", err)
	}

	adminServer, err := admin.NewServer(
		st,
		nil,
		nil,
		cfg.SelfMode.Enable,
		cfg.EmailVerif.Enable,
		cfg.SMTP,
		cfg.Billing,
		cfg.Payment,
		cfg.Server.PublicBaseURL,
		cfg.AppSettingsDefaults.AdminTimeZone,
		cfg.Security.TrustProxyHeaders,
		cfg.Security.TrustedProxyCIDRs,
		cfg.Tickets,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("admin.NewServer failed: %v", err)
	}

	openaiHandler := openaiapi.NewHandler(nil, nil, nil, nil, nil, nil, nil, nil, false, nil, nil, nil, cfg.Limits.DefaultMaxOutputTokens, upstream.SSEPumpOptions{})

	app := &App{
		cfg:         cfg,
		store:       st,
		web:         webServer,
		admin:       adminServer,
		openai:      openaiHandler,
		tokenLimits: limits.NewTokenLimits(1, 1),
		mux:         http.NewServeMux(),
	}
	app.routes()
	return app
}

func TestRoutes_SelfMode_DisablesBillingAndTickets(t *testing.T) {
	cfg := config.Config{
		SelfMode: config.SelfModeConfig{Enable: true},
		Security: config.SecurityConfig{SubscriptionOrderWebhookSecret: "secret"},
		Limits: config.LimitsConfig{
			MaxBodyBytes:       1 << 20,
			MaxRequestDuration: 2 * time.Second,
		},
	}
	app := newTestApp(t, cfg)

	cases := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/subscription"},
		{method: http.MethodGet, path: "/topup"},
		{method: http.MethodGet, path: "/pay/subscription/1"},
		{method: http.MethodPost, path: "/pay/subscription/1/start"},
		{method: http.MethodGet, path: "/tickets"},
		{method: http.MethodGet, path: "/tickets/1"},

		{method: http.MethodGet, path: "/admin/subscriptions"},
		{method: http.MethodGet, path: "/admin/orders"},
		{method: http.MethodGet, path: "/admin/payment-channels"},
		{method: http.MethodGet, path: "/admin/settings/payment-channels"},
		{method: http.MethodGet, path: "/admin/settings/payment-channels/1"},
		{method: http.MethodGet, path: "/admin/tickets"},

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
		Limits: config.LimitsConfig{
			MaxBodyBytes:       1 << 20,
			MaxRequestDuration: 2 * time.Second,
		},
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

func TestRoutes_DefaultMode_EnablesBillingAndTickets(t *testing.T) {
	cfg := config.Config{
		SelfMode: config.SelfModeConfig{Enable: false},
		Security: config.SecurityConfig{SubscriptionOrderWebhookSecret: "secret"},
		Limits: config.LimitsConfig{
			MaxBodyBytes:       1 << 20,
			MaxRequestDuration: 2 * time.Second,
		},
	}
	app := newTestApp(t, cfg)

	cases := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/subscription"},
		{method: http.MethodGet, path: "/topup"},
		{method: http.MethodGet, path: "/pay/subscription/1"},
		{method: http.MethodPost, path: "/pay/subscription/1/start"},
		{method: http.MethodGet, path: "/tickets"},
		{method: http.MethodGet, path: "/tickets/1"},

		{method: http.MethodGet, path: "/admin/subscriptions"},
		{method: http.MethodGet, path: "/admin/orders"},
		{method: http.MethodGet, path: "/admin/payment-channels"},
		{method: http.MethodGet, path: "/admin/settings/payment-channels"},
		{method: http.MethodGet, path: "/admin/settings/payment-channels/1"},
		{method: http.MethodGet, path: "/admin/tickets"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, "http://example.com"+tc.path, nil)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code == http.StatusNotFound {
			t.Fatalf("%s %s expected status != %d, got %d", tc.method, tc.path, http.StatusNotFound, rr.Code)
		}
	}
}

func TestRoutes_NoChatFeature(t *testing.T) {
	cfg := config.Config{
		SelfMode: config.SelfModeConfig{Enable: false},
		Security: config.SecurityConfig{SubscriptionOrderWebhookSecret: "secret"},
		Limits: config.LimitsConfig{
			MaxBodyBytes:       1 << 20,
			MaxRequestDuration: 2 * time.Second,
		},
	}
	app := newTestApp(t, cfg)

	cases := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/chat"},
		{method: http.MethodPost, path: "/api/chat/token"},
		{method: http.MethodPost, path: "/v1/chat/completions"},
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

func TestRoutes_Assets_IconAndFavicon(t *testing.T) {
	cfg := config.Config{
		Limits: config.LimitsConfig{
			MaxBodyBytes:       1 << 20,
			MaxRequestDuration: 2 * time.Second,
		},
	}
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

	cfg := config.Config{
		SelfMode: config.SelfModeConfig{Enable: true},
		Limits: config.LimitsConfig{
			MaxRequestDuration: 1 * time.Second,
		},
	}
	qp := quotaProviderForConfig(st, cfg)
	if _, ok := qp.(*quota.FeatureProvider); !ok {
		t.Fatalf("expected *quota.FeatureProvider, got %T", qp)
	}

	cfg.SelfMode.Enable = false
	qp = quotaProviderForConfig(st, cfg)
	if _, ok := qp.(*quota.FeatureProvider); !ok {
		t.Fatalf("expected *quota.FeatureProvider, got %T", qp)
	}
}
