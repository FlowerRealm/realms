package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	openaiapi "realms/internal/api/openai"
	"realms/internal/auth"
	"realms/internal/codexoauth"
	"realms/internal/config"
	rlmcrypto "realms/internal/crypto"
	"realms/internal/quota"
	"realms/internal/store"
	"realms/internal/tickets"
	"realms/internal/upstream"
	"realms/router"
)

func newTestApp(t *testing.T, cfg config.Config) *App {
	t.Helper()

	personalMode := cfg.IsPersonalMode()

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
	st.SetAppSettingsDefaults(cfg.AppSettingsDefaults)
	ticketStorage := tickets.NewStorage(filepath.Join(dir, "tickets"))

	openaiHandler := openaiapi.NewHandler(nil, nil, nil, nil, nil, nil, personalMode, nil, nil, nil, nil, upstream.SSEPumpOptions{}, nil)

	app := &App{
		cfg:           cfg,
		store:         st,
		ticketStorage: ticketStorage,
		openai:        openaiHandler,
	}

	var adminAPIKeyHash []byte
	if raw := strings.TrimSpace(cfg.Security.AdminAPIKey); raw != "" {
		adminAPIKeyHash = rlmcrypto.TokenHash(raw)
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
	engine.Use(sessions.Sessions(SessionCookieNameForPersonalMode(personalMode), sessionStore))

	router.SetRouter(engine, router.Options{
		Store:                           st,
		PersonalMode:                    personalMode,
		AdminAPIKeyHash:                 adminAPIKeyHash,
		AllowOpenRegistration:           cfg.Security.AllowOpenRegistration,
		EmailVerificationEnabledDefault: cfg.EmailVerif.Enable,
		BillingDefault:                  cfg.Billing,
		SMTPDefault:                     cfg.SMTP,
		TicketStorage:                   ticketStorage,
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

func TestRoutes_PersonalMode_DisablesBillingWebhooks(t *testing.T) {
	cfg := config.Config{
		Mode:     config.ModePersonal,
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
		Mode:     config.ModeBusiness,
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

func TestSelfMode_KeyAuth_AllowsAdminUsageWithoutSession(t *testing.T) {
	cfg := config.Config{
		Mode:       config.ModePersonal,
		Security:   config.SecurityConfig{SubscriptionOrderWebhookSecret: "secret"},
		EmailVerif: config.EmailVerifConfig{Enable: false},
	}
	app := newTestApp(t, cfg)

	t.Run("meta shows key not set", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/api/meta", nil)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		data, _ := payload["data"].(map[string]any)
		if v, _ := data["personal_mode_key_set"].(bool); v {
			t.Fatalf("expected personal_mode_key_set=false")
		}
	})

	t.Run("admin usage rejects before bootstrap", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage", nil)
		req.Header.Set("Authorization", "Bearer k_test_123")
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if ok, _ := payload["success"].(bool); ok {
			t.Fatalf("expected success=false, got %v", payload["success"])
		}
	})

	t.Run("bootstrap sets key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/api/personal/bootstrap", strings.NewReader(`{"key":"k_test_123"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if ok, _ := payload["success"].(bool); !ok {
			t.Fatalf("expected success=true, got %v", payload["success"])
		}
	})

	t.Run("bootstrap cannot be called twice", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/api/personal/bootstrap", strings.NewReader(`{"key":"k_other"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if ok, _ := payload["success"].(bool); ok {
			t.Fatalf("expected success=false, got %v", payload["success"])
		}
	})

	t.Run("admin usage accepts after bootstrap", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage", nil)
		req.Header.Set("Authorization", "Bearer k_test_123")
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if ok, _ := payload["success"].(bool); !ok {
			t.Fatalf("expected success=true, got %v", payload["success"])
		}
	})

	t.Run("admin mcp accepts after bootstrap", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/mcp", nil)
		req.Header.Set("Authorization", "Bearer k_test_123")
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if ok, _ := payload["success"].(bool); !ok {
			t.Fatalf("expected success=true, got %v", payload["success"])
		}
	})
}

func TestSelfMode_KeyAuth_RejectsAdminUsageWithoutKey(t *testing.T) {
	cfg := config.Config{
		Mode: config.ModePersonal,
	}
	app := newTestApp(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/personal/bootstrap", strings.NewReader(`{"key":"k_test_123"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage", nil)
	rr = httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if ok, _ := payload["success"].(bool); ok {
		t.Fatalf("expected success=false, got %v", payload["success"])
	}
}

func TestSelfMode_KeyAuth_DataPlaneUsageEventsRequiresKey(t *testing.T) {
	cfg := config.Config{
		Mode: config.ModePersonal,
	}
	app := newTestApp(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/personal/bootstrap", strings.NewReader(`{"key":"k_test_123"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	t.Run("missing key returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/usage/events", nil)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})

	t.Run("admin key is not accepted by data plane", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/usage/events", nil)
		req.Header.Set("Authorization", "Bearer k_test_123")
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})

	t.Run("personal api key allows data plane but not admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/api/personal/keys", strings.NewReader(`{"name":"cli"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer k_test_123")
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if ok, _ := payload["success"].(bool); !ok {
			t.Fatalf("expected success=true, got %v", payload["success"])
		}
		data, _ := payload["data"].(map[string]any)
		pk, _ := data["key"].(string)
		if pk == "" {
			t.Fatalf("expected key, got empty")
		}

		req = httptest.NewRequest(http.MethodGet, "http://example.com/v1/usage/events", nil)
		req.Header.Set("Authorization", "Bearer "+pk)
		rr = httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage", nil)
		req.Header.Set("Authorization", "Bearer "+pk)
		rr = httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		payload = nil
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if ok, _ := payload["success"].(bool); ok {
			t.Fatalf("expected success=false for admin, got %v", payload["success"])
		}
	})
}

func TestBusinessMode_AdminAPIKey_AllowsAdminRoutesWithoutSession(t *testing.T) {
	cfg := config.Config{
		Mode: config.ModeBusiness,
		Security: config.SecurityConfig{
			AdminAPIKey:                    "adm_test_123",
			SubscriptionOrderWebhookSecret: "secret",
		},
	}
	app := newTestApp(t, cfg)

	t.Run("admin usage accepts key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage", nil)
		req.Header.Set("Authorization", "Bearer adm_test_123")
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if ok, _ := payload["success"].(bool); !ok {
			t.Fatalf("expected success=true, got %v", payload["success"])
		}
	})

	t.Run("channel create accepts key", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"type":     store.UpstreamTypeOpenAICompatible,
			"name":     "curl-admin-channel",
			"groups":   "",
			"base_url": "https://api.openai.com/v1",
		})
		req := httptest.NewRequest(http.MethodPost, "http://example.com/api/channel", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", "adm_test_123")
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if ok, _ := payload["success"].(bool); !ok {
			t.Fatalf("expected success=true, got %v body=%s", payload["success"], rr.Body.String())
		}
	})

	t.Run("data plane rejects admin key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/usage/events", nil)
		req.Header.Set("Authorization", "Bearer adm_test_123")
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})
}

func TestBusinessMode_AdminAPIKey_InvalidKeyDoesNotFallbackToSession(t *testing.T) {
	cfg := config.Config{
		Mode: config.ModeBusiness,
		Security: config.SecurityConfig{
			AdminAPIKey:                    "adm_test_456",
			SubscriptionOrderWebhookSecret: "secret",
		},
	}
	app := newTestApp(t, cfg)
	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userID, err := app.store.CreateUser(ctx, "root@example.com", "root", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	loginReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/user/login", strings.NewReader(`{"login":"root@example.com","password":"password123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	app.Handler().ServeHTTP(loginRR, loginReq)
	if loginRR.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRR.Code, loginRR.Body.String())
	}
	var sessionCookie string
	for _, c := range loginRR.Result().Cookies() {
		if c.Name == SessionCookieNameForPersonalMode(false) {
			sessionCookie = c.String()
			break
		}
	}
	if sessionCookie == "" {
		t.Fatalf("expected session cookie")
	}

	t.Run("invalid key rejects even with valid session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage", nil)
		req.Header.Set("Authorization", "Bearer wrong_key")
		req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
		req.Header.Set("Cookie", sessionCookie)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if ok, _ := payload["success"].(bool); ok {
			t.Fatalf("expected success=false, got %v", payload["success"])
		}
	})

	t.Run("session still works when no key header is present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage", nil)
		req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
		req.Header.Set("Cookie", sessionCookie)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if ok, _ := payload["success"].(bool); !ok {
			t.Fatalf("expected success=true, got %v", payload["success"])
		}
	})
}

func TestBusinessMode_AdminAPIKey_SystemAuditPaths(t *testing.T) {
	cfg := config.Config{
		Mode: config.ModeBusiness,
		Security: config.SecurityConfig{
			AdminAPIKey:                    "adm_audit_123",
			SubscriptionOrderWebhookSecret: "secret",
		},
	}
	app := newTestApp(t, cfg)
	ctx := context.Background()

	userID, err := app.store.CreateUser(ctx, "user@example.com", "user1", []byte("pw"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	planID, err := app.store.CreateSubscriptionPlan(ctx, store.SubscriptionPlanCreate{
		Code:            "plan_sys",
		Name:            "System Plan",
		PriceMultiplier: store.DefaultGroupPriceMultiplier,
		PriceCNY:        decimal.RequireFromString("10"),
		DurationDays:    30,
		Status:          1,
	})
	if err != nil {
		t.Fatalf("CreateSubscriptionPlan: %v", err)
	}
	order, _, err := app.store.CreateSubscriptionOrderByPlanID(ctx, userID, planID, time.Now())
	if err != nil {
		t.Fatalf("CreateSubscriptionOrderByPlanID: %v", err)
	}
	ticketID, _, err := app.store.CreateTicketWithMessageAndAttachments(ctx, userID, "Need help", "first message", nil)
	if err != nil {
		t.Fatalf("CreateTicketWithMessageAndAttachments: %v", err)
	}

	t.Run("approve order stores approved_by=0", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/api/admin/orders/"+strconv.FormatInt(order.ID, 10)+"/approve", nil)
		req.Header.Set("Authorization", "Bearer adm_audit_123")
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if ok, _ := payload["success"].(bool); !ok {
			t.Fatalf("expected success=true, got %v", payload["success"])
		}
		gotOrder, err := app.store.GetSubscriptionOrderByID(ctx, order.ID)
		if err != nil {
			t.Fatalf("GetSubscriptionOrderByID: %v", err)
		}
		if gotOrder.ApprovedBy == nil || *gotOrder.ApprovedBy != 0 {
			t.Fatalf("ApprovedBy = %#v, want pointer to 0", gotOrder.ApprovedBy)
		}
	})

	t.Run("ticket reply uses system actor", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if err := writer.WriteField("body", "system reply"); err != nil {
			t.Fatalf("WriteField: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("writer.Close: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "http://example.com/api/admin/tickets/"+strconv.FormatInt(ticketID, 10)+"/reply", &body)
		req.Header.Set("Authorization", "Bearer adm_audit_123")
		req.Header.Set("Content-Type", writer.FormDataContentType())
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if ok, _ := payload["success"].(bool); !ok {
			t.Fatalf("expected success=true, got %v", payload["success"])
		}
		msgs, err := app.store.ListTicketMessagesWithActors(ctx, ticketID)
		if err != nil {
			t.Fatalf("ListTicketMessagesWithActors: %v", err)
		}
		last := msgs[len(msgs)-1]
		if last.ActorType != store.TicketActorTypeSystem {
			t.Fatalf("ActorType = %q, want %q", last.ActorType, store.TicketActorTypeSystem)
		}
		if last.ActorUserID == nil || *last.ActorUserID != 0 {
			t.Fatalf("ActorUserID = %#v, want pointer to 0", last.ActorUserID)
		}
	})
}

func TestRoutes_SPAFallback(t *testing.T) {
	cfg := config.Config{
		Mode:     config.ModeBusiness,
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

	t.Run("GET /api/admin/mcp returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/mcp", nil)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
		}
	})

	t.Run("GET /admin/mcp returns 404", func(t *testing.T) {
		for _, p := range []string{
			"/admin/mcp",
			"/admin/mcp/x",
		} {
			req := httptest.NewRequest(http.MethodGet, "http://example.com"+p, nil)
			rr := httptest.NewRecorder()
			app.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusNotFound {
				t.Fatalf("GET %s expected status %d, got %d", p, http.StatusNotFound, rr.Code)
			}
		}
	})
}

func TestRoutes_PersonalMode_SPAAllowedPaths(t *testing.T) {
	cfg := config.Config{
		Mode: config.ModePersonal,
	}
	app := newTestApp(t, cfg)

	t.Run("allowed paths serve index", func(t *testing.T) {
		paths := []string{
			"/",
			"/login",
			"/mcp",
			"/admin",
			"/admin?tab=channels",
			"/admin?tab=usage",
			"/admin?tab=settings",
			"/admin/channels",
			"/admin/usage",
			"/admin/settings",
			"/admin/api-keys",
		}

		for _, p := range paths {
			req := httptest.NewRequest(http.MethodGet, "http://example.com"+p, nil)
			rr := httptest.NewRecorder()
			app.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("GET %s expected status %d, got %d", p, http.StatusOK, rr.Code)
			}
			if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Fatalf("GET %s expected Content-Type text/html, got %q", p, ct)
			}
			if body := rr.Body.String(); !strings.Contains(body, "INDEX") {
				t.Fatalf("GET %s expected index html body, got %q", p, body)
			}
		}
	})

	t.Run("disallowed paths return 404", func(t *testing.T) {
		for _, p := range []string{
			"/usage",
			"/admin/mcp",
			"/admin/mcp/x",
		} {
			req := httptest.NewRequest(http.MethodGet, "http://example.com"+p, nil)
			rr := httptest.NewRecorder()
			app.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusNotFound {
				t.Fatalf("GET %s expected status %d, got %d", p, http.StatusNotFound, rr.Code)
			}
		}
	})
}

func TestRoutes_NoChatFeature(t *testing.T) {
	cfg := config.Config{
		Mode:     config.ModeBusiness,
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

	t.Run("personal mode uses free provider", func(t *testing.T) {
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
			Mode:    config.ModePersonal,
			Billing: config.BillingConfig{EnablePayAsYouGo: true},
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

func TestNewConcurrencyManager_PingFailureReturnsError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	mgr, err := newConcurrencyManager(config.Config{
		Redis: config.RedisConfig{Addr: addr},
		Gateway: config.GatewayConfig{
			UserMaxConcurrency:  1,
			WaitTimeoutMS:       30000,
			RetryBaseDelayMS:    300,
			RetryMaxDelayMS:     3000,
			WaitQueueExtraSlots: 20,
		},
	})
	if err == nil {
		t.Fatalf("expected ping redis error")
	}
	if mgr != nil {
		t.Fatalf("expected nil manager on ping failure")
	}
	if !strings.Contains(err.Error(), "ping redis") {
		t.Fatalf("expected wrapped ping redis error, got %v", err)
	}
}

func TestNewConcurrencyManager_PingSuccessReturnsManager(t *testing.T) {
	mr := miniredis.RunT(t)

	mgr, err := newConcurrencyManager(config.Config{
		Redis: config.RedisConfig{Addr: mr.Addr(), KeyPrefix: "server-test"},
		Gateway: config.GatewayConfig{
			UserMaxConcurrency:  1,
			WaitTimeoutMS:       1500,
			RetryBaseDelayMS:    120,
			RetryMaxDelayMS:     980,
			WaitQueueExtraSlots: 13,
		},
	})
	if err != nil {
		t.Fatalf("newConcurrencyManager: %v", err)
	}
	if mgr == nil || !mgr.Enabled() {
		t.Fatalf("expected enabled manager")
	}
	defer func() { _ = mgr.Close() }()
}

func TestNewConcurrencyManager_SkipsRedisWhenConcurrencyDisabled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	mgr, err := newConcurrencyManager(config.Config{
		Redis: config.RedisConfig{Addr: addr},
		Gateway: config.GatewayConfig{
			UserMaxConcurrency:       0,
			CredentialMaxConcurrency: 0,
			WaitTimeoutMS:            30000,
			RetryBaseDelayMS:         300,
			RetryMaxDelayMS:          3000,
			WaitQueueExtraSlots:      20,
		},
	})
	if err != nil {
		t.Fatalf("newConcurrencyManager should skip optional redis when concurrency disabled: %v", err)
	}
	if mgr != nil {
		t.Fatalf("expected nil manager when concurrency is disabled")
	}
}
