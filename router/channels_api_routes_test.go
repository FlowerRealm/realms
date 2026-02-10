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

func TestChannels_PageAndReorder_RootFlow(t *testing.T) {
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
	if userID <= 0 {
		t.Fatalf("expected userID > 0")
	}

	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_test_123")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}
	if tokenID <= 0 {
		t.Fatalf("expected tokenID > 0")
	}

	ch1, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel ch1: %v", err)
	}
	ch2, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c2", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel ch2: %v", err)
	}

	usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_test_1",
		UserID:           userID,
		TokenID:          tokenID,
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage: %v", err)
	}
	inTokens := int64(100)
	outTokens := int64(50)
	cachedIn := int64(10)
	cachedOut := int64(5)
	ch1ID := ch1
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID:       usageID,
		UpstreamChannelID:  &ch1ID,
		InputTokens:        &inTokens,
		CachedInputTokens:  &cachedIn,
		OutputTokens:       &outTokens,
		CachedOutputTokens: &cachedOut,
		CommittedUSD:       decimal.RequireFromString("1.23"),
	}); err != nil {
		t.Fatalf("CommitUsage: %v", err)
	}
	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID:        usageID,
		StatusCode:          200,
		LatencyMS:           1000,
		FirstTokenLatencyMS: 200,
		UpstreamChannelID:   &ch1ID,
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

	// page
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/channel/page", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("page status=%d body=%s", rr.Code, rr.Body.String())
	}
	var pageResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			AdminTimeZone string `json:"admin_time_zone"`
			Start         string `json:"start"`
			End           string `json:"end"`
			Overview      struct {
				Requests             int64  `json:"requests"`
				Tokens               int64  `json:"tokens"`
				CommittedUSD         string `json:"committed_usd"`
				CacheRatio           string `json:"cache_ratio"`
				AvgFirstTokenLatency string `json:"avg_first_token_latency"`
				TokensPerSecond      string `json:"tokens_per_second"`
				BindingRuntime       struct {
					Available bool `json:"available"`
				} `json:"binding_runtime"`
			} `json:"overview"`
			Channels []struct {
				ID    int64 `json:"id"`
				Usage struct {
					CommittedUSD          string `json:"committed_usd"`
					Tokens                int64  `json:"tokens"`
					CacheRatio            string `json:"cache_ratio"`
					AvgFirstTokenLatency  string `json:"avg_first_token_latency"`
					OutputTokensPerSecond string `json:"tokens_per_second"`
				} `json:"usage"`
				Runtime struct {
					Available bool `json:"available"`
				} `json:"runtime"`
			} `json:"channels"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &pageResp); err != nil {
		t.Fatalf("json.Unmarshal page: %v", err)
	}
	if !pageResp.Success {
		t.Fatalf("expected success, got message=%q", pageResp.Message)
	}
	if pageResp.Data.AdminTimeZone == "" || pageResp.Data.Start == "" || pageResp.Data.End == "" {
		t.Fatalf("expected tz/start/end, got: %#v", pageResp.Data)
	}
	if pageResp.Data.Overview.Requests != 1 {
		t.Fatalf("expected overview requests=1, got %d", pageResp.Data.Overview.Requests)
	}
	if pageResp.Data.Overview.AvgFirstTokenLatency != "200.0 ms" {
		t.Fatalf("expected overview avg_first_token_latency=200.0 ms, got %q", pageResp.Data.Overview.AvgFirstTokenLatency)
	}
	if pageResp.Data.Overview.TokensPerSecond != "62.50" {
		t.Fatalf("expected overview tokens_per_second=62.50, got %q", pageResp.Data.Overview.TokensPerSecond)
	}
	if pageResp.Data.Overview.BindingRuntime.Available {
		t.Fatalf("expected overview.binding_runtime.available=false when opts.Sched nil")
	}
	if len(pageResp.Data.Channels) < 2 {
		t.Fatalf("expected >=2 channels, got %d", len(pageResp.Data.Channels))
	}
	foundCh1 := false
	for _, it := range pageResp.Data.Channels {
		if it.ID == ch1 {
			foundCh1 = true
			if it.Usage.CommittedUSD != "1.23" {
				t.Fatalf("expected ch1 committed_usd=1.23, got %q", it.Usage.CommittedUSD)
			}
			if it.Usage.Tokens != 150 {
				t.Fatalf("expected ch1 tokens=150, got %d", it.Usage.Tokens)
			}
			if it.Usage.CacheRatio != "10.0%" {
				t.Fatalf("expected ch1 cache_ratio=10.0%%, got %q", it.Usage.CacheRatio)
			}
			if it.Usage.AvgFirstTokenLatency != "200.0 ms" {
				t.Fatalf("expected ch1 avg_first_token_latency=200.0 ms, got %q", it.Usage.AvgFirstTokenLatency)
			}
			if it.Usage.OutputTokensPerSecond != "62.50" {
				t.Fatalf("expected ch1 tokens_per_second=62.50, got %q", it.Usage.OutputTokensPerSecond)
			}
			if it.Runtime.Available {
				t.Fatalf("expected runtime.available=false when opts.Admin nil")
			}
		}
	}
	if !foundCh1 {
		t.Fatalf("expected ch1 in response")
	}

	// channel timeseries
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/channel/"+strconv.FormatInt(ch1, 10)+"/timeseries", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("timeseries status=%d body=%s", rr.Code, rr.Body.String())
	}
	var tsResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			ChannelID   int64  `json:"channel_id"`
			Granularity string `json:"granularity"`
			Points      []struct {
				Bucket               string  `json:"bucket"`
				CommittedUSD         float64 `json:"committed_usd"`
				Tokens               int64   `json:"tokens"`
				CacheRatio           float64 `json:"cache_ratio"`
				AvgFirstTokenLatency float64 `json:"avg_first_token_latency"`
				TokensPerSecond      float64 `json:"tokens_per_second"`
			} `json:"points"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &tsResp); err != nil {
		t.Fatalf("json.Unmarshal timeseries: %v", err)
	}
	if !tsResp.Success {
		t.Fatalf("expected timeseries success, got message=%q", tsResp.Message)
	}
	if tsResp.Data.ChannelID != ch1 {
		t.Fatalf("expected channel_id=%d, got %d", ch1, tsResp.Data.ChannelID)
	}
	if tsResp.Data.Granularity != "hour" {
		t.Fatalf("expected granularity=hour, got %q", tsResp.Data.Granularity)
	}
	if len(tsResp.Data.Points) == 0 {
		t.Fatalf("expected non-empty timeseries points")
	}
	p := tsResp.Data.Points[0]
	if p.Bucket == "" {
		t.Fatalf("expected non-empty point bucket")
	}
	if p.Tokens != 150 {
		t.Fatalf("expected tokens=150, got %d", p.Tokens)
	}
	if p.CommittedUSD < 1.22 || p.CommittedUSD > 1.24 {
		t.Fatalf("expected committed_usd around 1.23, got %.4f", p.CommittedUSD)
	}
	if p.CacheRatio < 9.9 || p.CacheRatio > 10.1 {
		t.Fatalf("expected cache_ratio around 10.0, got %.4f", p.CacheRatio)
	}
	if p.AvgFirstTokenLatency < 199.9 || p.AvgFirstTokenLatency > 200.1 {
		t.Fatalf("expected avg_first_token_latency around 200, got %.4f", p.AvgFirstTokenLatency)
	}
	if p.TokensPerSecond < 62.49 || p.TokensPerSecond > 62.51 {
		t.Fatalf("expected tokens_per_second around 62.50, got %.4f", p.TokensPerSecond)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/channel/"+strconv.FormatInt(ch1, 10)+"/timeseries?granularity=day", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("timeseries(day) status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &tsResp); err != nil {
		t.Fatalf("json.Unmarshal timeseries(day): %v", err)
	}
	if !tsResp.Success {
		t.Fatalf("expected timeseries(day) success, got message=%q", tsResp.Message)
	}
	if tsResp.Data.Granularity != "day" {
		t.Fatalf("expected granularity=day, got %q", tsResp.Data.Granularity)
	}

	// reorder: place ch1 before ch2 (keep other channels in-place)
	ids := make([]int64, 0, len(pageResp.Data.Channels))
	for _, it := range pageResp.Data.Channels {
		ids = append(ids, it.ID)
	}
	desired := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == ch1 {
			continue
		}
		desired = append(desired, id)
	}
	// insert ch1 before ch2
	inserted := false
	out := make([]int64, 0, len(desired)+1)
	for _, id := range desired {
		if !inserted && id == ch2 {
			out = append(out, ch1)
			inserted = true
		}
		out = append(out, id)
	}
	if !inserted {
		out = append(out, ch1)
	}

	reorderBody, _ := json.Marshal(out)
	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/channel/reorder", bytes.NewReader(reorderBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reorder status=%d body=%s", rr.Code, rr.Body.String())
	}
	var okResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &okResp); err != nil {
		t.Fatalf("json.Unmarshal reorder: %v", err)
	}
	if !okResp.Success {
		t.Fatalf("expected reorder success, got message=%q", okResp.Message)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/channel/page", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("page2 status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &pageResp); err != nil {
		t.Fatalf("json.Unmarshal page2: %v", err)
	}
	if !pageResp.Success || len(pageResp.Data.Channels) < 2 {
		t.Fatalf("unexpected page2 resp: %#v", pageResp)
	}
	idx1 := -1
	idx2 := -1
	for i, it := range pageResp.Data.Channels {
		if it.ID == ch1 {
			idx1 = i
		}
		if it.ID == ch2 {
			idx2 = i
		}
	}
	if idx1 < 0 || idx2 < 0 {
		t.Fatalf("expected ch1/ch2 in response, got idx1=%d idx2=%d", idx1, idx2)
	}
	if idx1 >= idx2 {
		t.Fatalf("expected ch1 before ch2 after reorder, got idx1=%d idx2=%d", idx1, idx2)
	}
}

func TestChannels_SettingsAndCredentials_RootFlow(t *testing.T) {
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
	if userID <= 0 {
		t.Fatalf("expected userID > 0")
	}

	chID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}

	ep, err := st.SetUpstreamEndpointBaseURL(ctx, chID, "https://api.openai.com")
	if err != nil {
		t.Fatalf("SetUpstreamEndpointBaseURL: %v", err)
	}
	cred1, _, err := st.CreateOpenAICompatibleCredential(ctx, ep.ID, nil, "sk-test-abc123")
	if err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential: %v", err)
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

	// credentials list
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/channel/"+strconv.FormatInt(chID, 10)+"/credentials", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("credentials list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var listResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    []struct {
			ID        int64  `json:"id"`
			MaskedKey string `json:"masked_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("json.Unmarshal credentials list: %v", err)
	}
	if !listResp.Success {
		t.Fatalf("expected success, got message=%q", listResp.Message)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].ID != cred1 {
		t.Fatalf("unexpected credentials list: %#v", listResp.Data)
	}
	if listResp.Data[0].MaskedKey != "...c123" {
		t.Fatalf("expected masked_key=...c123, got %q", listResp.Data[0].MaskedKey)
	}

	// credentials create
	createCredBody, _ := json.Marshal(map[string]any{
		"name":    "team-a",
		"api_key": "sk-test-xyz987",
	})
	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/channel/"+strconv.FormatInt(chID, 10)+"/credentials", bytes.NewReader(createCredBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("credentials create status=%d body=%s", rr.Code, rr.Body.String())
	}
	var createResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("json.Unmarshal credentials create: %v", err)
	}
	if !createResp.Success || createResp.Data.ID <= 0 {
		t.Fatalf("unexpected credentials create resp: %#v", createResp)
	}

	// credentials delete (delete the first one)
	req = httptest.NewRequest(http.MethodDelete, "http://example.com/api/channel/"+strconv.FormatInt(chID, 10)+"/credentials/"+strconv.FormatInt(cred1, 10), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("credentials delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	var delResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &delResp); err != nil {
		t.Fatalf("json.Unmarshal credentials delete: %v", err)
	}
	if !delResp.Success {
		t.Fatalf("expected delete success, got message=%q", delResp.Message)
	}

	// meta update
	metaBody, _ := json.Marshal(map[string]any{
		"test_model": "gpt-5.2",
		"weight":     3,
		"auto_ban":   false,
	})
	req = httptest.NewRequest(http.MethodPut, "http://example.com/api/channel/"+strconv.FormatInt(chID, 10)+"/meta", bytes.NewReader(metaBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("meta update status=%d body=%s", rr.Code, rr.Body.String())
	}
	var metaResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &metaResp); err != nil {
		t.Fatalf("json.Unmarshal meta update: %v", err)
	}
	if !metaResp.Success {
		t.Fatalf("expected meta update success, got message=%q", metaResp.Message)
	}

	// channel detail includes new meta fields
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/channel/"+strconv.FormatInt(chID, 10), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get channel status=%d body=%s", rr.Code, rr.Body.String())
	}
	var detailResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			ID        int64   `json:"id"`
			Status    int     `json:"status"`
			TestModel *string `json:"test_model"`
			Weight    int     `json:"weight"`
			AutoBan   bool    `json:"auto_ban"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("json.Unmarshal get channel: %v", err)
	}
	if !detailResp.Success || detailResp.Data.ID != chID {
		t.Fatalf("unexpected get channel resp: %#v", detailResp)
	}
	if detailResp.Data.TestModel == nil || *detailResp.Data.TestModel != "gpt-5.2" {
		t.Fatalf("expected test_model=gpt-5.2, got %#v", detailResp.Data.TestModel)
	}
	if detailResp.Data.Weight != 3 {
		t.Fatalf("expected weight=3, got %d", detailResp.Data.Weight)
	}
	if detailResp.Data.AutoBan {
		t.Fatalf("expected auto_ban=false, got true")
	}

	// manual disable channel
	updateBody, _ := json.Marshal(map[string]any{
		"id":     chID,
		"status": 0,
	})
	req = httptest.NewRequest(http.MethodPut, "http://example.com/api/channel", bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("update channel status=%d body=%s", rr.Code, rr.Body.String())
	}
	var updateResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("json.Unmarshal update channel: %v", err)
	}
	if !updateResp.Success {
		t.Fatalf("expected update channel success, got message=%q", updateResp.Message)
	}

	// verify disabled status persisted
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/channel/"+strconv.FormatInt(chID, 10), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get channel after disable status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("json.Unmarshal get channel after disable: %v", err)
	}
	if !detailResp.Success || detailResp.Data.Status != 0 {
		t.Fatalf("expected channel status=0 after disable, got %#v", detailResp)
	}
}

func TestChannels_CodexOAuthAccounts_RootFlow(t *testing.T) {
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
	if userID <= 0 {
		t.Fatalf("expected userID > 0")
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeCodexOAuth, "codex-main", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	ep, err := st.SetUpstreamEndpointBaseURL(ctx, channelID, "https://chatgpt.com/backend-api/codex")
	if err != nil {
		t.Fatalf("SetUpstreamEndpointBaseURL: %v", err)
	}
	existingEmail := "codex-user@example.com"
	existingAccountID, err := st.CreateCodexOAuthAccount(ctx, ep.ID, "acc_existing", &existingEmail, "at_old", "rt_old", nil, nil)
	if err != nil {
		t.Fatalf("CreateCodexOAuthAccount: %v", err)
	}

	var (
		startEndpointID   int64
		startActorUserID  int64
		completeState     string
		completeCode      string
		refreshEndpointID int64
		refreshAccountID  int64
	)

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
		StartCodexOAuth: func(_ context.Context, endpointID int64, actorUserID int64) (string, error) {
			startEndpointID = endpointID
			startActorUserID = actorUserID
			return "https://chatgpt.com/oauth/authorize?state=s1", nil
		},
		CompleteCodexOAuth: func(_ context.Context, endpointID int64, actorUserID int64, state string, code string) error {
			if endpointID != ep.ID {
				t.Fatalf("unexpected endpointID in complete: got=%d want=%d", endpointID, ep.ID)
			}
			if actorUserID != userID {
				t.Fatalf("unexpected actorUserID in complete: got=%d want=%d", actorUserID, userID)
			}
			completeState = state
			completeCode = code
			return nil
		},
		RefreshCodexQuotasByEndpointID: func(_ context.Context, endpointID int64) error {
			refreshEndpointID = endpointID
			return nil
		},
		RefreshCodexQuotaByAccountID: func(_ context.Context, accID int64) error {
			refreshAccountID = accID
			return nil
		},
	})

	// login
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

	// list accounts
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/channel/"+strconv.FormatInt(channelID, 10)+"/codex-accounts", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list codex accounts status=%d body=%s", rr.Code, rr.Body.String())
	}
	var listResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    []struct {
			ID        int64  `json:"id"`
			AccountID string `json:"account_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("json.Unmarshal list codex accounts: %v", err)
	}
	if !listResp.Success {
		t.Fatalf("expected success, got message=%q", listResp.Message)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].ID != existingAccountID || listResp.Data[0].AccountID != "acc_existing" {
		t.Fatalf("unexpected codex account list: %#v", listResp.Data)
	}

	// start oauth
	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/channel/"+strconv.FormatInt(channelID, 10)+"/codex-oauth/start", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("start codex oauth status=%d body=%s", rr.Code, rr.Body.String())
	}
	var startResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			AuthURL string `json:"auth_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &startResp); err != nil {
		t.Fatalf("json.Unmarshal start codex oauth: %v", err)
	}
	if !startResp.Success {
		t.Fatalf("expected start success, got message=%q", startResp.Message)
	}
	if startResp.Data.AuthURL == "" {
		t.Fatalf("expected auth_url not empty")
	}
	if startEndpointID != ep.ID || startActorUserID != userID {
		t.Fatalf("unexpected start args: endpoint=%d actor=%d", startEndpointID, startActorUserID)
	}

	// complete oauth callback
	completeBody, _ := json.Marshal(map[string]any{
		"callback_url": "http://localhost:1455/auth/callback?code=abc123&state=state789",
	})
	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/channel/"+strconv.FormatInt(channelID, 10)+"/codex-oauth/complete", bytes.NewReader(completeBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("complete codex oauth status=%d body=%s", rr.Code, rr.Body.String())
	}
	var completeResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &completeResp); err != nil {
		t.Fatalf("json.Unmarshal complete codex oauth: %v", err)
	}
	if !completeResp.Success {
		t.Fatalf("expected complete success, got message=%q", completeResp.Message)
	}
	if completeCode != "abc123" || completeState != "state789" {
		t.Fatalf("unexpected complete args: state=%q code=%q", completeState, completeCode)
	}

	// manual create account
	createBody, _ := json.Marshal(map[string]any{
		"account_id":    "acc_manual",
		"email":         "manual@example.com",
		"access_token":  "at_manual",
		"refresh_token": "rt_manual",
	})
	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/channel/"+strconv.FormatInt(channelID, 10)+"/codex-accounts", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create codex account status=%d body=%s", rr.Code, rr.Body.String())
	}
	var createResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("json.Unmarshal create codex account: %v", err)
	}
	if !createResp.Success || createResp.Data.ID <= 0 {
		t.Fatalf("unexpected create codex account resp: %#v", createResp)
	}

	// refresh all by endpoint
	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/channel/"+strconv.FormatInt(channelID, 10)+"/codex-accounts/refresh", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("refresh codex accounts status=%d body=%s", rr.Code, rr.Body.String())
	}
	if refreshEndpointID != ep.ID {
		t.Fatalf("unexpected refresh endpoint id: got=%d want=%d", refreshEndpointID, ep.ID)
	}

	// refresh one account
	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/channel/"+strconv.FormatInt(channelID, 10)+"/codex-accounts/"+strconv.FormatInt(existingAccountID, 10)+"/refresh", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("refresh one codex account status=%d body=%s", rr.Code, rr.Body.String())
	}
	if refreshAccountID != existingAccountID {
		t.Fatalf("unexpected refresh account id: got=%d want=%d", refreshAccountID, existingAccountID)
	}

	// delete existing account
	req = httptest.NewRequest(http.MethodDelete, "http://example.com/api/channel/"+strconv.FormatInt(channelID, 10)+"/codex-accounts/"+strconv.FormatInt(existingAccountID, 10), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete codex account status=%d body=%s", rr.Code, rr.Body.String())
	}
	var delResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &delResp); err != nil {
		t.Fatalf("json.Unmarshal delete codex account: %v", err)
	}
	if !delResp.Success {
		t.Fatalf("expected delete success, got message=%q", delResp.Message)
	}

	// list should still include manual account only
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/channel/"+strconv.FormatInt(channelID, 10)+"/codex-accounts", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list codex accounts after delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("json.Unmarshal list codex accounts after delete: %v", err)
	}
	if !listResp.Success || len(listResp.Data) != 1 || listResp.Data[0].AccountID != "acc_manual" {
		t.Fatalf("expected remaining manual account, got %#v", listResp)
	}
}
