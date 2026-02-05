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

func TestUsageEvents_UserResponse_HidesUpstreamChannel(t *testing.T) {
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
	userID, err := st.CreateUser(ctx, "u@example.com", "u", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	upstreamChannelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch1", store.DefaultGroupName, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	upstreamEndpointID, err := st.CreateUpstreamEndpoint(ctx, upstreamChannelID, "http://example.com/v1", 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	upstreamCredID, _, err := st.CreateOpenAICompatibleCredential(ctx, upstreamEndpointID, nil, "sk-upstream-123")
	if err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential: %v", err)
	}

	tokenName := "t1"
	tokenID, _, err := st.CreateUserToken(ctx, userID, &tokenName, "sk-test-123")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	usageEventID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_test_1",
		UserID:           userID,
		SubscriptionID:   nil,
		TokenID:          tokenID,
		Model:            nil,
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage: %v", err)
	}

	inTokens := int64(1)
	outTokens := int64(2)
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID:      usageEventID,
		UpstreamChannelID: &upstreamChannelID,
		InputTokens:       &inTokens,
		OutputTokens:      &outTokens,
		CommittedUSD:      decimal.Zero,
	}); err != nil {
		t.Fatalf("CommitUsage: %v", err)
	}

	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID:       usageEventID,
		Endpoint:           "/v1/chat/completions",
		Method:             "POST",
		StatusCode:         200,
		LatencyMS:          123,
		UpstreamChannelID:  &upstreamChannelID,
		UpstreamEndpointID: &upstreamEndpointID,
		UpstreamCredID:     &upstreamCredID,
		IsStream:           false,
		RequestBytes:       100,
		ResponseBytes:      200,
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

	// login
	loginBody, _ := json.Marshal(map[string]any{
		"login":    "u@example.com",
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

	// list usage events
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/events", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("usage events status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Events []map[string]any `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal usage events: %v", err)
	}
	if !got.Success {
		t.Fatalf("usage events expected success, got message=%q", got.Message)
	}
	if len(got.Data.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got.Data.Events))
	}

	ev := got.Data.Events[0]
	if _, ok := ev["upstream_channel_id"]; ok {
		t.Fatalf("expected upstream_channel_id to be hidden for user usage events")
	}
	if _, ok := ev["upstream_channel_name"]; ok {
		t.Fatalf("expected upstream_channel_name to be hidden for user usage events")
	}
	if _, ok := ev["upstream_endpoint_id"]; ok {
		t.Fatalf("expected upstream_endpoint_id to be hidden for user usage events")
	}
	if _, ok := ev["upstream_credential_id"]; ok {
		t.Fatalf("expected upstream_credential_id to be hidden for user usage events")
	}
}
