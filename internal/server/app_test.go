package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	openaiapi "realms/internal/api/openai"
	"realms/internal/codexoauth"
	"realms/internal/config"
	"realms/internal/quota"
	"realms/internal/store"
	"realms/internal/upstream"
	"realms/router"
)

func newTestApp(t *testing.T, cfg config.Config) *App {
	t.Helper()

	st := store.New(nil)

	openaiHandler := openaiapi.NewHandler(nil, nil, nil, nil, nil, nil, false, nil, nil, nil, nil, upstream.SSEPumpOptions{})

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
		Store:                           st,
		SelfMode:                        cfg.SelfMode.Enable,
		AllowOpenRegistration:           cfg.Security.AllowOpenRegistration,
		EmailVerificationEnabledDefault: cfg.EmailVerif.Enable,
		BillingDefault:                  cfg.Billing,
		SMTPDefault:                     cfg.SMTP,
		OpenAI:                          openaiHandler,
		FrontendIndexPage:               []byte("<!doctype html><html><body>INDEX</body></html>"),

		Healthz: func(w http.ResponseWriter, r *http.Request) {
			out := map[string]any{"ok": true}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(out)
		},
		RealmsIconSVG: app.handleRealmsIconSVG,
		FaviconICO:    app.handleFaviconICO,

		SubscriptionOrderPaidWebhook:  app.handleSubscriptionOrderPaidWebhook,
		StripeWebhookByPaymentChannel: app.handleStripeWebhookByPaymentChannel,
		EPayNotifyByPaymentChannel:    app.handleEPayNotifyByPaymentChannel,
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
		{method: http.MethodPost, path: "/api/pay/stripe/webhook/1"},
		{method: http.MethodGet, path: "/api/pay/epay/notify/1"},
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
	t.Run("default uses normal provider (hybrid)", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"

		db, err := store.OpenSQLite(path)
		if err != nil {
			t.Fatalf("OpenSQLite: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		if err := store.EnsureSQLiteSchema(db); err != nil {
			t.Fatalf("EnsureSQLiteSchema: %v", err)
		}

		st := store.New(db)
		st.SetDialect(store.DialectSQLite)

		ctx := context.Background()
		userID, err := st.CreateUser(ctx, "alice@example.com", "alice", []byte("pw-hash"), store.UserRoleUser)
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_test_123")
		if err != nil {
			t.Fatalf("CreateUserToken: %v", err)
		}
		if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.RequireFromString("10")); err != nil {
			t.Fatalf("AddUserBalanceUSD: %v", err)
		}

		cfg := config.Config{
			Billing: config.BillingConfig{EnablePayAsYouGo: true},
		}
		st.SetAppSettingsDefaults(cfg.AppSettingsDefaults)

		qp := quotaProvider(st, cfg)
		if _, ok := qp.(*quota.FeatureProvider); !ok {
			t.Fatalf("expected *quota.FeatureProvider, got %T", qp)
		}

		res, err := qp.Reserve(ctx, quota.ReserveInput{
			RequestID: "req_1",
			UserID:    userID,
			TokenID:   tokenID,
		})
		if err != nil {
			t.Fatalf("Reserve: %v", err)
		}

		if got, err := st.GetUserBalanceUSD(ctx, userID); err != nil {
			t.Fatalf("GetUserBalanceUSD: %v", err)
		} else if got.String() != "9.999" {
			t.Fatalf("balance after reserve: got %s want %s", got.String(), "9.999")
		}

		ev, err := st.GetUsageEvent(ctx, res.UsageEventID)
		if err != nil {
			t.Fatalf("GetUsageEvent: %v", err)
		}
		if ev.State != store.UsageStateReserved {
			t.Fatalf("state mismatch: got %q want %q", ev.State, store.UsageStateReserved)
		}
		if ev.ReservedUSD.String() != "0.001" {
			t.Fatalf("reserved_usd mismatch: got %s want %s", ev.ReservedUSD.String(), "0.001")
		}
	})

	t.Run("self_mode uses free provider", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"

		db, err := store.OpenSQLite(path)
		if err != nil {
			t.Fatalf("OpenSQLite: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		if err := store.EnsureSQLiteSchema(db); err != nil {
			t.Fatalf("EnsureSQLiteSchema: %v", err)
		}

		st := store.New(db)
		st.SetDialect(store.DialectSQLite)

		ctx := context.Background()
		userID, err := st.CreateUser(ctx, "alice@example.com", "alice", []byte("pw-hash"), store.UserRoleUser)
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_test_123")
		if err != nil {
			t.Fatalf("CreateUserToken: %v", err)
		}
		if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.RequireFromString("10")); err != nil {
			t.Fatalf("AddUserBalanceUSD: %v", err)
		}

		cfg := config.Config{
			SelfMode: config.SelfModeConfig{Enable: true},
			Billing:  config.BillingConfig{EnablePayAsYouGo: true},
		}
		st.SetAppSettingsDefaults(cfg.AppSettingsDefaults)

		qp := quotaProvider(st, cfg)
		res, err := qp.Reserve(ctx, quota.ReserveInput{
			RequestID: "req_1",
			UserID:    userID,
			TokenID:   tokenID,
		})
		if err != nil {
			t.Fatalf("Reserve: %v", err)
		}

		if got, err := st.GetUserBalanceUSD(ctx, userID); err != nil {
			t.Fatalf("GetUserBalanceUSD: %v", err)
		} else if got.String() != "10" {
			t.Fatalf("balance after reserve: got %s want %s", got.String(), "10")
		}

		ev, err := st.GetUsageEvent(ctx, res.UsageEventID)
		if err != nil {
			t.Fatalf("GetUsageEvent: %v", err)
		}
		if ev.ReservedUSD.String() != "0" {
			t.Fatalf("reserved_usd mismatch: got %s want %s", ev.ReservedUSD.String(), "0")
		}
	})

	t.Run("feature_disable_billing uses free provider", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"

		db, err := store.OpenSQLite(path)
		if err != nil {
			t.Fatalf("OpenSQLite: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		if err := store.EnsureSQLiteSchema(db); err != nil {
			t.Fatalf("EnsureSQLiteSchema: %v", err)
		}

		st := store.New(db)
		st.SetDialect(store.DialectSQLite)

		ctx := context.Background()
		userID, err := st.CreateUser(ctx, "alice@example.com", "alice", []byte("pw-hash"), store.UserRoleUser)
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_test_123")
		if err != nil {
			t.Fatalf("CreateUserToken: %v", err)
		}
		if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.RequireFromString("10")); err != nil {
			t.Fatalf("AddUserBalanceUSD: %v", err)
		}

		cfg := config.Config{
			Billing: config.BillingConfig{EnablePayAsYouGo: true},
			AppSettingsDefaults: config.AppSettingsDefaultsConfig{
				FeatureDisableBilling: true,
			},
		}
		st.SetAppSettingsDefaults(cfg.AppSettingsDefaults)

		qp := quotaProvider(st, cfg)
		res, err := qp.Reserve(ctx, quota.ReserveInput{
			RequestID: "req_1",
			UserID:    userID,
			TokenID:   tokenID,
		})
		if err != nil {
			t.Fatalf("Reserve: %v", err)
		}

		if got, err := st.GetUserBalanceUSD(ctx, userID); err != nil {
			t.Fatalf("GetUserBalanceUSD: %v", err)
		} else if got.String() != "10" {
			t.Fatalf("balance after reserve: got %s want %s", got.String(), "10")
		}

		ev, err := st.GetUsageEvent(ctx, res.UsageEventID)
		if err != nil {
			t.Fatalf("GetUsageEvent: %v", err)
		}
		if ev.ReservedUSD.String() != "0" {
			t.Fatalf("reserved_usd mismatch: got %s want %s", ev.ReservedUSD.String(), "0")
		}
	})
}

func TestCodexOAuthRedirectURI(t *testing.T) {
	t.Run("default_uses_codex_cli_redirect", func(t *testing.T) {
		got := codexOAuthRedirectURI(":8080")
		if got != codexoauth.DefaultRedirectURI {
			t.Fatalf("codexOAuthRedirectURI = %q, want %q", got, codexoauth.DefaultRedirectURI)
		}
	})

	t.Run("prefer_realms_env_override", func(t *testing.T) {
		t.Setenv("REALMS_CODEX_OAUTH_REDIRECT_URI", "https://example.com/auth/callback")
		got := codexOAuthRedirectURI(":8080")
		if got != "https://example.com/auth/callback" {
			t.Fatalf("codexOAuthRedirectURI = %q, want %q", got, "https://example.com/auth/callback")
		}
	})

	t.Run("fallback_legacy_env_override", func(t *testing.T) {
		t.Setenv("REALMS_CODEX_OAUTH_REDIRECT_URI", "")
		t.Setenv("CODEX_OAUTH_REDIRECT_URI", "http://localhost:8080/auth/callback")
		got := codexOAuthRedirectURI(":8080")
		if got != "http://localhost:8080/auth/callback" {
			t.Fatalf("codexOAuthRedirectURI = %q, want %q", got, "http://localhost:8080/auth/callback")
		}
	})
}
