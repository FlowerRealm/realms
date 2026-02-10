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

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/store"
)

func TestUserModelsDetail_UsesMainGroupSubgroupsAndBasePricing(t *testing.T) {
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
	userID, err := st.CreateUser(ctx, "pricing@example.com", "pricing", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}
	if err := st.CreateMainGroup(ctx, "ug1", nil, 1); err != nil {
		t.Fatalf("CreateMainGroup: %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, "ug1", []string{"vip"}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, "ug1"); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch-model", "vip", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            "gpt-5.2",
		GroupName:           "vip",
		OwnedBy:             nil,
		InputUSDPer1M:       decimal.RequireFromString("1"),
		OutputUSDPer1M:      decimal.RequireFromString("2"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0.5"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0.25"),
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     channelID,
		PublicID:      "gpt-5.2",
		UpstreamModel: "gpt-5.2",
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel: %v", err)
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
		"login":    "pricing@example.com",
		"password": "password123",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/user/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	loginRR := httptest.NewRecorder()
	engine.ServeHTTP(loginRR, loginReq)
	if loginRR.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRR.Code, loginRR.Body.String())
	}
	sessionCookie := ""
	for _, cookieItem := range loginRR.Result().Cookies() {
		if cookieItem.Name == cookieName {
			sessionCookie = cookieItem.String()
			break
		}
	}
	if sessionCookie == "" {
		t.Fatalf("expected session cookie %q", cookieName)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/user/models/detail", nil)
	req.Header.Set("Cookie", sessionCookie)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    []struct {
			PublicID            string `json:"public_id"`
			InputUSDPer1M       string `json:"input_usd_per_1m"`
			OutputUSDPer1M      string `json:"output_usd_per_1m"`
			CacheInputUSDPer1M  string `json:"cache_input_usd_per_1m"`
			CacheOutputUSDPer1M string `json:"cache_output_usd_per_1m"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected success, got message=%q", got.Message)
	}
	if len(got.Data) != 1 {
		t.Fatalf("expected 1 model, got %d", len(got.Data))
	}

	model := got.Data[0]
	if model.PublicID != "gpt-5.2" {
		t.Fatalf("public_id mismatch: got=%q want=%q", model.PublicID, "gpt-5.2")
	}
	if model.InputUSDPer1M != "1" {
		t.Fatalf("input_usd_per_1m mismatch: got=%q want=%q", model.InputUSDPer1M, "1")
	}
	if model.OutputUSDPer1M != "2" {
		t.Fatalf("output_usd_per_1m mismatch: got=%q want=%q", model.OutputUSDPer1M, "2")
	}
	if model.CacheInputUSDPer1M != "0.5" {
		t.Fatalf("cache_input_usd_per_1m mismatch: got=%q want=%q", model.CacheInputUSDPer1M, "0.5")
	}
	if model.CacheOutputUSDPer1M != "0.25" {
		t.Fatalf("cache_output_usd_per_1m mismatch: got=%q want=%q", model.CacheOutputUSDPer1M, "0.25")
	}
}
