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

	ch1, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c1", "default", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel ch1: %v", err)
	}
	ch2, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c2", "default", 0, false, false, false, false)
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
			Channels      []struct {
				ID    int64 `json:"id"`
				Usage struct {
					CommittedUSD string `json:"committed_usd"`
					Tokens       int64  `json:"tokens"`
					CacheRatio   string `json:"cache_ratio"`
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
			if it.Runtime.Available {
				t.Fatalf("expected runtime.available=false when opts.Admin nil")
			}
		}
	}
	if !foundCh1 {
		t.Fatalf("expected ch1 in response")
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

	chID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c1", "default", 0, false, false, false, false)
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
		"test_model": "gpt-4.1-mini",
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
	if detailResp.Data.TestModel == nil || *detailResp.Data.TestModel != "gpt-4.1-mini" {
		t.Fatalf("expected test_model=gpt-4.1-mini, got %#v", detailResp.Data.TestModel)
	}
	if detailResp.Data.Weight != 3 {
		t.Fatalf("expected weight=3, got %d", detailResp.Data.Weight)
	}
	if detailResp.Data.AutoBan {
		t.Fatalf("expected auto_ban=false, got true")
	}
}
