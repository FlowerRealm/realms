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

func TestUsageEventDetail_UserResponse_IncludesPricingBreakdown(t *testing.T) {
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
	userID, err := st.CreateUser(ctx, "detail@example.com", "detailuser", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}
	if _, err := st.CreateChannelGroup(ctx, "staff", nil, 1, decimal.RequireFromString("2"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(staff): %v", err)
	}
	if err := st.ReplaceUserGroups(ctx, userID, []string{"vip", "staff"}); err != nil {
		t.Fatalf("ReplaceUserGroups: %v", err)
	}

	modelID := "m_detail_1"
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            modelID,
		GroupName:           store.DefaultGroupName,
		InputUSDPer1M:       decimal.RequireFromString("2"),
		OutputUSDPer1M:      decimal.RequireFromString("4"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0.5"),
		CacheOutputUSDPer1M: decimal.RequireFromString("1"),
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}

	tokenName := "t-detail"
	tokenID, _, err := st.CreateUserToken(ctx, userID, &tokenName, "sk-detail-123")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	usageEventID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_detail_1",
		UserID:           userID,
		SubscriptionID:   nil,
		TokenID:          tokenID,
		Model:            &modelID,
		ReservedUSD:      decimal.RequireFromString("9.3"),
		ReserveExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage: %v", err)
	}

	inTokens := int64(1_000_000)
	outTokens := int64(500_000)
	cachedInTokens := int64(400_000)
	cachedOutTokens := int64(100_000)
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID:       usageEventID,
		InputTokens:        &inTokens,
		CachedInputTokens:  &cachedInTokens,
		OutputTokens:       &outTokens,
		CachedOutputTokens: &cachedOutTokens,
		CommittedUSD:       decimal.RequireFromString("9.3"),
	}); err != nil {
		t.Fatalf("CommitUsage: %v", err)
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
		"login":    "detail@example.com",
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
	for _, cookieItem := range rr.Result().Cookies() {
		if cookieItem.Name == cookieName {
			sessionCookie = cookieItem.String()
			break
		}
	}
	if sessionCookie == "" {
		t.Fatalf("expected session cookie %q", cookieName)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/events/"+strconv.FormatInt(usageEventID, 10)+"/detail", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			EventID          int64 `json:"event_id"`
			Available        bool  `json:"available"`
			PricingBreakdown struct {
				CostSource           string `json:"cost_source"`
				CostSourceUSD        string `json:"cost_source_usd"`
				ModelFound           bool   `json:"model_found"`
				InputTokensTotal     int64  `json:"input_tokens_total"`
				InputTokensCached    int64  `json:"input_tokens_cached"`
				InputTokensBillable  int64  `json:"input_tokens_billable"`
				OutputTokensTotal    int64  `json:"output_tokens_total"`
				OutputTokensCached   int64  `json:"output_tokens_cached"`
				OutputTokensBillable int64  `json:"output_tokens_billable"`
				InputUSDPer1M        string `json:"input_usd_per_1m"`
				OutputUSDPer1M       string `json:"output_usd_per_1m"`
				CacheInputUSDPer1M   string `json:"cache_input_usd_per_1m"`
				CacheOutputUSDPer1M  string `json:"cache_output_usd_per_1m"`
				InputCostUSD         string `json:"input_cost_usd"`
				OutputCostUSD        string `json:"output_cost_usd"`
				CacheInputCostUSD    string `json:"cache_input_cost_usd"`
				CacheOutputCostUSD   string `json:"cache_output_cost_usd"`
				BaseCostUSD          string `json:"base_cost_usd"`
				UserMultiplier       string `json:"user_multiplier"`
				EffectiveMultiplier  string `json:"effective_multiplier"`
				FinalCostUSD         string `json:"final_cost_usd"`
			} `json:"pricing_breakdown"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal detail: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected success, got message=%q", got.Message)
	}
	if got.Data.EventID != usageEventID {
		t.Fatalf("event_id mismatch: got=%d want=%d", got.Data.EventID, usageEventID)
	}
	if got.Data.PricingBreakdown.CostSource != "committed" {
		t.Fatalf("cost_source mismatch: got=%q want=%q", got.Data.PricingBreakdown.CostSource, "committed")
	}
	if got.Data.PricingBreakdown.CostSourceUSD != "9.3" {
		t.Fatalf("cost_source_usd mismatch: got=%q want=%q", got.Data.PricingBreakdown.CostSourceUSD, "9.3")
	}
	if !got.Data.PricingBreakdown.ModelFound {
		t.Fatalf("expected model_found=true")
	}
	if got.Data.PricingBreakdown.InputTokensTotal != 1_000_000 {
		t.Fatalf("input_tokens_total mismatch: got=%d want=%d", got.Data.PricingBreakdown.InputTokensTotal, 1_000_000)
	}
	if got.Data.PricingBreakdown.InputTokensCached != 400_000 {
		t.Fatalf("input_tokens_cached mismatch: got=%d want=%d", got.Data.PricingBreakdown.InputTokensCached, 400_000)
	}
	if got.Data.PricingBreakdown.InputTokensBillable != 600_000 {
		t.Fatalf("input_tokens_billable mismatch: got=%d want=%d", got.Data.PricingBreakdown.InputTokensBillable, 600_000)
	}
	if got.Data.PricingBreakdown.OutputTokensTotal != 500_000 {
		t.Fatalf("output_tokens_total mismatch: got=%d want=%d", got.Data.PricingBreakdown.OutputTokensTotal, 500_000)
	}
	if got.Data.PricingBreakdown.OutputTokensCached != 100_000 {
		t.Fatalf("output_tokens_cached mismatch: got=%d want=%d", got.Data.PricingBreakdown.OutputTokensCached, 100_000)
	}
	if got.Data.PricingBreakdown.OutputTokensBillable != 400_000 {
		t.Fatalf("output_tokens_billable mismatch: got=%d want=%d", got.Data.PricingBreakdown.OutputTokensBillable, 400_000)
	}
	if got.Data.PricingBreakdown.InputUSDPer1M != "2" {
		t.Fatalf("input_usd_per_1m mismatch: got=%q want=%q", got.Data.PricingBreakdown.InputUSDPer1M, "2")
	}
	if got.Data.PricingBreakdown.OutputUSDPer1M != "4" {
		t.Fatalf("output_usd_per_1m mismatch: got=%q want=%q", got.Data.PricingBreakdown.OutputUSDPer1M, "4")
	}
	if got.Data.PricingBreakdown.CacheInputUSDPer1M != "0.5" {
		t.Fatalf("cache_input_usd_per_1m mismatch: got=%q want=%q", got.Data.PricingBreakdown.CacheInputUSDPer1M, "0.5")
	}
	if got.Data.PricingBreakdown.CacheOutputUSDPer1M != "1" {
		t.Fatalf("cache_output_usd_per_1m mismatch: got=%q want=%q", got.Data.PricingBreakdown.CacheOutputUSDPer1M, "1")
	}
	if got.Data.PricingBreakdown.InputCostUSD != "1.2" {
		t.Fatalf("input_cost_usd mismatch: got=%q want=%q", got.Data.PricingBreakdown.InputCostUSD, "1.2")
	}
	if got.Data.PricingBreakdown.OutputCostUSD != "1.6" {
		t.Fatalf("output_cost_usd mismatch: got=%q want=%q", got.Data.PricingBreakdown.OutputCostUSD, "1.6")
	}
	if got.Data.PricingBreakdown.CacheInputCostUSD != "0.2" {
		t.Fatalf("cache_input_cost_usd mismatch: got=%q want=%q", got.Data.PricingBreakdown.CacheInputCostUSD, "0.2")
	}
	if got.Data.PricingBreakdown.CacheOutputCostUSD != "0.1" {
		t.Fatalf("cache_output_cost_usd mismatch: got=%q want=%q", got.Data.PricingBreakdown.CacheOutputCostUSD, "0.1")
	}
	if got.Data.PricingBreakdown.BaseCostUSD != "3.1" {
		t.Fatalf("base_cost_usd mismatch: got=%q want=%q", got.Data.PricingBreakdown.BaseCostUSD, "3.1")
	}
	if got.Data.PricingBreakdown.UserMultiplier != "3" {
		t.Fatalf("user_multiplier mismatch: got=%q want=%q", got.Data.PricingBreakdown.UserMultiplier, "3")
	}
	if got.Data.PricingBreakdown.EffectiveMultiplier != "3" {
		t.Fatalf("effective_multiplier mismatch: got=%q want=%q", got.Data.PricingBreakdown.EffectiveMultiplier, "3")
	}
	if got.Data.PricingBreakdown.FinalCostUSD != "9.3" {
		t.Fatalf("final_cost_usd mismatch: got=%q want=%q", got.Data.PricingBreakdown.FinalCostUSD, "9.3")
	}
}
