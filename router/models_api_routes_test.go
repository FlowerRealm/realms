package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5")); err != nil {
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

func TestAdminUpdateManagedModel_PreservesHighContextPricingWhenOmitted(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ctx := context.Background()

	ownedBy := "openai"
	id, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            "gpt-5.4",
		GroupName:           "default",
		OwnedBy:             &ownedBy,
		InputUSDPer1M:       decimal.RequireFromString("2.5"),
		OutputUSDPer1M:      decimal.RequireFromString("15"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0.25"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0.25"),
		HighContextPricing: &store.ManagedModelHighContextPricing{
			ThresholdInputTokens: 272000,
			AppliesTo:            store.ManagedModelHighContextAppliesToFullRequest,
			ServiceTierPolicy:    store.ManagedModelHighContextServiceTierPolicyForceStandard,
			InputUSDPer1M:        decimal.RequireFromString("5"),
			OutputUSDPer1M:       decimal.RequireFromString("22.5"),
		},
		Status: 1,
	})
	if err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}

	engine, sessionCookie, userID := setupRootSession(t, st)
	body, _ := json.Marshal(map[string]any{
		"id":                       id,
		"public_id":                "gpt-5.4",
		"group_name":               "default",
		"owned_by":                 "openai",
		"input_usd_per_1m":         2.5,
		"output_usd_per_1m":        15,
		"cache_input_usd_per_1m":   0.25,
		"cache_output_usd_per_1m":  0.25,
		"priority_pricing_enabled": false,
		"status":                   1,
	})
	req := httptest.NewRequest(http.MethodPut, "http://example.com/api/models/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Cookie", sessionCookie)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	got, err := st.GetManagedModelByID(ctx, id)
	if err != nil {
		t.Fatalf("GetManagedModelByID: %v", err)
	}
	if got.HighContextPricing == nil {
		t.Fatal("expected high_context_pricing to be preserved")
	}
	if !got.HighContextPricing.InputUSDPer1M.Equal(decimal.RequireFromString("5")) {
		t.Fatalf("high_context input=%s, want 5", got.HighContextPricing.InputUSDPer1M)
	}
}

func TestAdminSelectableManagedModelIDs_OnlyEnabledSorted(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ctx := context.Background()

	for _, model := range []store.ManagedModelCreate{
		{PublicID: "z-last", GroupName: "vip", InputUSDPer1M: decimal.RequireFromString("1"), OutputUSDPer1M: decimal.RequireFromString("2"), CacheInputUSDPer1M: decimal.RequireFromString("0.5"), CacheOutputUSDPer1M: decimal.RequireFromString("0.25"), Status: 1},
		{PublicID: "a-first", GroupName: "vip", InputUSDPer1M: decimal.RequireFromString("1"), OutputUSDPer1M: decimal.RequireFromString("2"), CacheInputUSDPer1M: decimal.RequireFromString("0.5"), CacheOutputUSDPer1M: decimal.RequireFromString("0.25"), Status: 1},
		{PublicID: "m-disabled", GroupName: "vip", InputUSDPer1M: decimal.RequireFromString("1"), OutputUSDPer1M: decimal.RequireFromString("2"), CacheInputUSDPer1M: decimal.RequireFromString("0.5"), CacheOutputUSDPer1M: decimal.RequireFromString("0.25"), Status: 2},
	} {
		if _, err := st.CreateManagedModel(ctx, model); err != nil {
			t.Fatalf("CreateManagedModel(%s): %v", model.PublicID, err)
		}
	}

	engine, sessionCookie, userID := setupRootSession(t, st)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/models/selectable", nil)
	req.Header.Set("Cookie", sessionCookie)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool     `json:"success"`
		Message string   `json:"message"`
		Data    []string `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected success, got message=%q", got.Message)
	}
	want := []string{"a-first", "z-last"}
	if len(got.Data) != len(want) {
		t.Fatalf("len(data)=%d want=%d data=%v", len(got.Data), len(want), got.Data)
	}
	if !sort.StringsAreSorted(got.Data) {
		t.Fatalf("data not sorted: %v", got.Data)
	}
	for i := range want {
		if got.Data[i] != want[i] {
			t.Fatalf("data[%d]=%q want=%q full=%v", i, got.Data[i], want[i], got.Data)
		}
	}
}

func TestAdminCreateChannelModel_RejectsMissingOrDisabledManagedModel(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5")); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch-model", "vip", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            "gpt-disabled",
		GroupName:           "vip",
		InputUSDPer1M:       decimal.RequireFromString("1"),
		OutputUSDPer1M:      decimal.RequireFromString("2"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0.5"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0.25"),
		Status:              2,
	}); err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}

	engine, sessionCookie, userID := setupRootSession(t, st)
	for _, tc := range []struct {
		name      string
		publicID  string
		messageIn string
	}{
		{name: "missing", publicID: "ghost-model", messageIn: "模型不存在"},
		{name: "disabled", publicID: "gpt-disabled", messageIn: "模型已禁用"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"public_id": tc.publicID, "upstream_model": tc.publicID, "status": 1})
			req := httptest.NewRequest(http.MethodPost, "http://example.com/api/channel/"+strconv.FormatInt(channelID, 10)+"/models", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
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
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if got.Success {
				t.Fatalf("expected failure, got success body=%s", rr.Body.String())
			}
			if !strings.Contains(got.Message, tc.messageIn) {
				t.Fatalf("message=%q want contains %q", got.Message, tc.messageIn)
			}
		})
	}
}

func TestAdminUpdateChannelModel_AllowsDisablingMissingManagedModel(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5")); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch-model", "vip", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	bindingID, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     channelID,
		PublicID:      "ghost-model",
		UpstreamModel: "ghost-model",
		Status:        1,
	})
	if err != nil {
		t.Fatalf("CreateChannelModel: %v", err)
	}

	engine, sessionCookie, userID := setupRootSession(t, st)
	body, _ := json.Marshal(map[string]any{
		"id":             bindingID,
		"public_id":      "ghost-model",
		"upstream_model": "ghost-model",
		"status":         2,
	})
	req := httptest.NewRequest(http.MethodPut, "http://example.com/api/channel/"+strconv.FormatInt(channelID, 10)+"/models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
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
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected success, got message=%q", got.Message)
	}

	binding, err := st.GetChannelModelByID(ctx, bindingID)
	if err != nil {
		t.Fatalf("GetChannelModelByID: %v", err)
	}
	if binding.Status != 2 {
		t.Fatalf("binding status=%d want=2", binding.Status)
	}
}
