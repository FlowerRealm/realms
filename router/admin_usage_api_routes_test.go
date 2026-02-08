package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/store"
)

func TestAdminUsagePage_EventIncludesFirstTokenLatencyAndTokensPerSecond(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"
	db, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer db.Close()
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}

	st := store.New(db)
	st.SetDialect(store.DialectSQLite)

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userID, err := st.CreateUser(ctx, "root@example.com", "root", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_admin_usage")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	chID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "channel-1", store.DefaultGroupName, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}

	usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_admin_usage_1",
		UserID:           userID,
		TokenID:          tokenID,
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage: %v", err)
	}

	inputTokens := int64(100)
	outputTokens := int64(50)
	chRef := chID
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID:      usageID,
		UpstreamChannelID: &chRef,
		InputTokens:       &inputTokens,
		OutputTokens:      &outputTokens,
		CommittedUSD:      decimal.RequireFromString("1.23"),
	}); err != nil {
		t.Fatalf("CommitUsage: %v", err)
	}

	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID:        usageID,
		Endpoint:            "/v1/responses",
		Method:              "POST",
		StatusCode:          200,
		LatencyMS:           1000,
		FirstTokenLatencyMS: 200,
		UpstreamChannelID:   &chRef,
	}); err != nil {
		t.Fatalf("FinalizeUsageEvent: %v", err)
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	cookieName := "realms_session"
	sessionStore := cookie.NewStore([]byte("test-secret"))
	sessionStore.Options(sessions.Options{
		Path:     "/",
		MaxAge:   2592000,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	engine.Use(sessions.Sessions(cookieName, sessionStore))

	SetRouter(engine, Options{
		Store:             st,
		SelfMode:          false,
		FrontendIndexPage: []byte("<!doctype html><html><body>INDEX</body></html>"),
	})

	loginBody, _ := json.Marshal(map[string]any{
		"login":    "root@example.com",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/user/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rr.Code, rr.Body.String())
	}

	sessionCookie := ""
	for _, c := range rr.Result().Cookies() {
		if c.Name == cookieName {
			sessionCookie = c.String()
			break
		}
	}
	if sessionCookie == "" {
		t.Fatalf("expected session cookie %q", cookieName)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin usage status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Window struct {
				AvgFirstTokenLatency string `json:"avg_first_token_latency"`
				TokensPerSecond      string `json:"tokens_per_second"`
			} `json:"window"`
			Events []struct {
				LatencyMS           string `json:"latency_ms"`
				FirstTokenLatencyMS string `json:"first_token_latency_ms"`
				OutputTokens        string `json:"output_tokens"`
				TokensPerSecond     string `json:"tokens_per_second"`
			} `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal admin usage: %v", err)
	}
	if !got.Success {
		t.Fatalf("admin usage expected success, got message=%q", got.Message)
	}
	if got.Data.Window.AvgFirstTokenLatency != "200.0 ms" {
		t.Fatalf("expected window avg_first_token_latency=200.0 ms, got %q", got.Data.Window.AvgFirstTokenLatency)
	}
	if got.Data.Window.TokensPerSecond != "62.50" {
		t.Fatalf("expected window tokens_per_second=62.50, got %q", got.Data.Window.TokensPerSecond)
	}
	if len(got.Data.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got.Data.Events))
	}
	ev := got.Data.Events[0]
	if ev.LatencyMS != "1000" {
		t.Fatalf("expected event latency_ms=1000, got %q", ev.LatencyMS)
	}
	if ev.FirstTokenLatencyMS != "200" {
		t.Fatalf("expected event first_token_latency_ms=200, got %q", ev.FirstTokenLatencyMS)
	}
	if ev.OutputTokens != "50" {
		t.Fatalf("expected event output_tokens=50, got %q", ev.OutputTokens)
	}
	if ev.TokensPerSecond != "62.50" {
		t.Fatalf("expected event tokens_per_second=62.50, got %q", ev.TokensPerSecond)
	}
}
