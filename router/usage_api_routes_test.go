package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
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

	upstreamChannelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch1", "", 0, false, false, false, false)
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

	errClass := "upstream_unavailable"
	errMsg := "上游不可用；最后一次失败: upstream_exhausted 400 The usage limit has been reached"
	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID:       usageEventID,
		Endpoint:           "/v1/chat/completions",
		Method:             "POST",
		StatusCode:         502,
		LatencyMS:          123,
		ErrorClass:         &errClass,
		ErrorMessage:       &errMsg,
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
	if v, ok := ev["error_class"]; !ok || v != "upstream_unavailable" {
		t.Fatalf("expected error_class=upstream_unavailable, got=%v (present=%v)", v, ok)
	}
	if v, ok := ev["error_message"]; !ok || v != "上游不可用" {
		t.Fatalf("expected error_message to be masked as %q, got=%v (present=%v)", "上游不可用", v, ok)
	}
}

func TestUsageEvents_UserResponse_ExposesModelCheck(t *testing.T) {
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
	userID, err := st.CreateUser(ctx, "mismatch@example.com", "mismatch", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "sk-model-check")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	usageEventID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_model_check_1",
		UserID:           userID,
		TokenID:          tokenID,
		Model:            optionalStringForUsageRouteTest("alias"),
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage: %v", err)
	}
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID: usageEventID,
		CommittedUSD: decimal.Zero,
	}); err != nil {
		t.Fatalf("CommitUsage: %v", err)
	}

	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID:          usageEventID,
		Endpoint:              "/v1/responses",
		Method:                "POST",
		StatusCode:            200,
		ForwardedModel:        optionalStringForUsageRouteTest("gpt-5.2"),
		UpstreamResponseModel: optionalStringForUsageRouteTest("gpt-5.2-mini"),
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
		FrontendIndexPage: []byte("<!doctype html><html><body>INDEX</body></html>"),
	})

	loginBody, _ := json.Marshal(map[string]any{
		"login":    "mismatch@example.com",
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

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/events", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("usage events status=%d body=%s", rr.Code, rr.Body.String())
	}

	var listResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Events []struct {
				ID            int64 `json:"id"`
				ModelMismatch bool  `json:"model_mismatch"`
			} `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("json.Unmarshal usage events: %v", err)
	}
	if !listResp.Success {
		t.Fatalf("usage events expected success, got message=%q", listResp.Message)
	}
	if len(listResp.Data.Events) != 1 || !listResp.Data.Events[0].ModelMismatch {
		t.Fatalf("expected model_mismatch=true, got=%+v", listResp.Data.Events)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/events/"+strconv.FormatInt(usageEventID, 10)+"/detail", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", rr.Code, rr.Body.String())
	}

	var detailResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			ModelCheck struct {
				ForwardedModel        string `json:"forwarded_model"`
				UpstreamResponseModel string `json:"upstream_response_model"`
				Mismatch              bool   `json:"mismatch"`
			} `json:"model_check"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("json.Unmarshal detail: %v", err)
	}
	if !detailResp.Success {
		t.Fatalf("detail expected success, got message=%q", detailResp.Message)
	}
	if detailResp.Data.ModelCheck.ForwardedModel != "gpt-5.2" || detailResp.Data.ModelCheck.UpstreamResponseModel != "gpt-5.2-mini" || !detailResp.Data.ModelCheck.Mismatch {
		t.Fatalf("unexpected model_check=%+v", detailResp.Data.ModelCheck)
	}
}

func optionalStringForUsageRouteTest(v string) *string {
	return &v
}

func TestUsageLeaderboard_ReturnsRankedUsersByWindow(t *testing.T) {
	st, db, closeDB := newTestSQLiteStoreWithDB(t)
	defer closeDB()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	aliceID, err := st.CreateUser(ctx, "alice@example.com", "alice", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(alice): %v", err)
	}
	bobID, err := st.CreateUser(ctx, "bob@example.com", "bob", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(bob): %v", err)
	}
	carolID, err := st.CreateUser(ctx, "carol@example.com", "carol", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(carol): %v", err)
	}

	mustToken := func(userID int64, raw string) int64 {
		t.Helper()
		name := "token-" + raw
		tokenID, _, err := st.CreateUserToken(ctx, userID, &name, raw)
		if err != nil {
			t.Fatalf("CreateUserToken(%s): %v", raw, err)
		}
		return tokenID
	}

	aliceTokenID := mustToken(aliceID, "sk-alice")
	bobTokenID := mustToken(bobID, "sk-bob")
	carolTokenID := mustToken(carolID, "sk-carol")

	shanghai, err := loadAdminLocation(defaultAdminTimeZone)
	if err != nil {
		t.Fatalf("loadAdminLocation: %v", err)
	}
	now := time.Date(2026, 3, 9, 14, 30, 0, 0, shanghai).UTC()
	prevNow := usageLeaderboardNow
	usageLeaderboardNow = func() time.Time { return now }
	defer func() {
		usageLeaderboardNow = prevNow
	}()

	insertUsageEventRow(t, db, "req_lb_alice_today", aliceID, aliceTokenID, store.UsageStateCommitted, time.Date(2026, 3, 9, 1, 30, 0, 0, shanghai), 10, 10, "30", "0", 100, 10)
	insertUsageEventRow(t, db, "req_lb_bob_today", bobID, bobTokenID, store.UsageStateCommitted, time.Date(2026, 3, 9, 10, 0, 0, 0, shanghai), 10, 10, "20", "0", 100, 10)
	insertUsageEventRow(t, db, "req_lb_bob_before_today", bobID, bobTokenID, store.UsageStateCommitted, time.Date(2026, 3, 2, 23, 50, 0, 0, shanghai), 10, 10, "5", "0", 100, 10)
	insertUsageEventRow(t, db, "req_lb_carol_week", carolID, carolTokenID, store.UsageStateCommitted, time.Date(2026, 3, 5, 9, 0, 0, 0, shanghai), 10, 10, "90", "0", 100, 10)
	insertUsageEventRow(t, db, "req_lb_alice_month", aliceID, aliceTokenID, store.UsageStateCommitted, time.Date(2026, 2, 12, 8, 0, 0, 0, shanghai), 10, 10, "15", "0", 100, 10)
	insertUsageEventRow(t, db, "req_lb_carol_before_month", carolID, carolTokenID, store.UsageStateCommitted, time.Date(2026, 2, 7, 23, 0, 0, 0, shanghai), 10, 10, "200", "0", 100, 10)

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "alice@example.com", "password123")

	call := func(window string) struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Window string `json:"window"`
			Since  string `json:"since"`
			Until  string `json:"until"`
			Users  []struct {
				Rank          int    `json:"rank"`
				DisplayName   string `json:"display_name"`
				CommittedUSD  string `json:"committed_usd"`
				ReservedUSD   string `json:"reserved_usd"`
				IsCurrentUser bool   `json:"is_current_user"`
			} `json:"users"`
		} `json:"data"`
	} {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/leaderboard?window="+window, nil)
		req.Header.Set("Realms-User", strconv.FormatInt(aliceID, 10))
		req.Header.Set("Cookie", sessionCookie)
		rr := httptest.NewRecorder()
		engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("leaderboard status=%d body=%s", rr.Code, rr.Body.String())
		}
		var got struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Data    struct {
				Window string `json:"window"`
				Since  string `json:"since"`
				Until  string `json:"until"`
				Users  []struct {
					Rank          int    `json:"rank"`
					DisplayName   string `json:"display_name"`
					CommittedUSD  string `json:"committed_usd"`
					ReservedUSD   string `json:"reserved_usd"`
					IsCurrentUser bool   `json:"is_current_user"`
				} `json:"users"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatalf("json.Unmarshal leaderboard: %v", err)
		}
		if !got.Success {
			t.Fatalf("leaderboard expected success, got message=%q", got.Message)
		}
		return got
	}

	got1d := call("1d")
	if got1d.Data.Window != "1d" {
		t.Fatalf("expected window=1d, got %q", got1d.Data.Window)
	}
	if got1d.Data.Since != "2026-03-09T00:00:00+08:00" || got1d.Data.Until != "2026-03-09T14:30:00+08:00" {
		t.Fatalf("unexpected 1d range: since=%q until=%q", got1d.Data.Since, got1d.Data.Until)
	}
	if len(got1d.Data.Users) != 2 {
		t.Fatalf("expected 2 users in 1d leaderboard, got %d", len(got1d.Data.Users))
	}
	if got1d.Data.Users[0].DisplayName != "alice" || !got1d.Data.Users[0].IsCurrentUser || got1d.Data.Users[0].CommittedUSD != "30" {
		t.Fatalf("unexpected first user in 1d leaderboard: %+v", got1d.Data.Users[0])
	}
	if got1d.Data.Users[1].DisplayName != "bob" || got1d.Data.Users[1].CommittedUSD != "20" {
		t.Fatalf("unexpected second user in 1d leaderboard: %+v", got1d.Data.Users[1])
	}

	got7d := call("7d")
	if len(got7d.Data.Users) != 3 {
		t.Fatalf("expected 3 users in 7d leaderboard, got %d", len(got7d.Data.Users))
	}
	if got7d.Data.Since != "2026-03-03T00:00:00+08:00" || got7d.Data.Until != "2026-03-09T14:30:00+08:00" {
		t.Fatalf("unexpected 7d range: since=%q until=%q", got7d.Data.Since, got7d.Data.Until)
	}
	if got7d.Data.Users[0].DisplayName != "carol" || got7d.Data.Users[0].CommittedUSD != "90" {
		t.Fatalf("unexpected first user in 7d leaderboard: %+v", got7d.Data.Users[0])
	}

	got1mo := call("1mo")
	if len(got1mo.Data.Users) != 3 {
		t.Fatalf("expected 3 users in 1mo leaderboard, got %d", len(got1mo.Data.Users))
	}
	if got1mo.Data.Since != "2026-02-08T00:00:00+08:00" || got1mo.Data.Until != "2026-03-09T14:30:00+08:00" {
		t.Fatalf("unexpected 1mo range: since=%q until=%q", got1mo.Data.Since, got1mo.Data.Until)
	}
	if got1mo.Data.Users[0].DisplayName != "carol" || got1mo.Data.Users[1].DisplayName != "alice" {
		t.Fatalf("unexpected order in 1mo leaderboard: %+v", got1mo.Data.Users)
	}
	if got1mo.Data.Users[1].CommittedUSD != "45" {
		t.Fatalf("expected alice month total to aggregate to 45, got %q", got1mo.Data.Users[1].CommittedUSD)
	}
}

func TestUsageLeaderboard_RejectsInvalidWindow(t *testing.T) {
	st, closeDB := newTestSQLiteStore(t)
	defer closeDB()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userID, err := st.CreateUser(ctx, "invalid@example.com", "invalid", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "invalid@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/leaderboard?window=90d", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("leaderboard status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal invalid leaderboard: %v", err)
	}
	if got.Success {
		t.Fatalf("expected invalid window request to fail")
	}
	if got.Message != "window 仅支持 1d/7d/1mo" {
		t.Fatalf("unexpected invalid window message: %q", got.Message)
	}
}

func TestUsageLeaderboardDisplayName_DoesNotExposeEmailWhenUsernameMissing(t *testing.T) {
	got := usageLeaderboardDisplayName("", "masked@example.com", 42)
	if got != "用户42" {
		t.Fatalf("expected anonymized display name, got %q", got)
	}
}

func TestUsageEvents_User_IndexKeyFiltersByTokenName(t *testing.T) {
	st, closeDB := newTestSQLiteStore(t)
	defer closeDB()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userID, err := st.CreateUser(ctx, "u@example.com", "u", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	tokenName := "key-abc"
	tokenID, _, err := st.CreateUserToken(ctx, userID, &tokenName, "sk-test-123")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}
	otherName := "key-xyz"
	otherID, _, err := st.CreateUserToken(ctx, userID, &otherName, "sk-test-456")
	if err != nil {
		t.Fatalf("CreateUserToken(other): %v", err)
	}

	usageID1, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_key_filter_1",
		UserID:           userID,
		TokenID:          tokenID,
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage(1): %v", err)
	}
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID: usageID1,
		CommittedUSD: decimal.Zero,
	}); err != nil {
		t.Fatalf("CommitUsage(1): %v", err)
	}
	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID: usageID1,
		Endpoint:     "/v1/responses",
		Method:       "POST",
		StatusCode:   200,
	}); err != nil {
		t.Fatalf("FinalizeUsageEvent(1): %v", err)
	}

	usageID2, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_key_filter_2",
		UserID:           userID,
		TokenID:          otherID,
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage(2): %v", err)
	}
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID: usageID2,
		CommittedUSD: decimal.Zero,
	}); err != nil {
		t.Fatalf("CommitUsage(2): %v", err)
	}
	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID: usageID2,
		Endpoint:     "/v1/responses",
		Method:       "POST",
		StatusCode:   200,
	}); err != nil {
		t.Fatalf("FinalizeUsageEvent(2): %v", err)
	}

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "u@example.com", "password123")

	list := func(url string) int {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
		req.Header.Set("Cookie", sessionCookie)
		rr := httptest.NewRecorder()
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
		return len(got.Data.Events)
	}

	if n := list("http://example.com/api/usage/events?index=key&q=%25abc%25"); n != 1 {
		t.Fatalf("expected 1 event for q=abc, got %d", n)
	}
	if n := list("http://example.com/api/usage/events?index=key&q=%25does-not-exist%25"); n != 0 {
		t.Fatalf("expected 0 event for missing key, got %d", n)
	}
}

func TestUsageEvents_User_IndexKeyAndModel_AND(t *testing.T) {
	st, closeDB := newTestSQLiteStore(t)
	defer closeDB()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userID, err := st.CreateUser(ctx, "u@example.com", "u", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	keyProd := "prod-key"
	tokenProdID, _, err := st.CreateUserToken(ctx, userID, &keyProd, "tok_prod")
	if err != nil {
		t.Fatalf("CreateUserToken(prod): %v", err)
	}
	keyOther := "other-key"
	tokenOtherID, _, err := st.CreateUserToken(ctx, userID, &keyOther, "tok_other")
	if err != nil {
		t.Fatalf("CreateUserToken(other): %v", err)
	}

	makeEvent := func(reqID string, tokenID int64, model string) {
		usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        reqID,
			UserID:           userID,
			TokenID:          tokenID,
			Model:            &model,
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: time.Now().UTC().Add(1 * time.Hour),
		})
		if err != nil {
			t.Fatalf("ReserveUsage(%s): %v", reqID, err)
		}
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID: usageID,
			CommittedUSD: decimal.Zero,
		}); err != nil {
			t.Fatalf("CommitUsage(%s): %v", reqID, err)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID: usageID,
			Endpoint:     "/v1/responses",
			Method:       "POST",
			StatusCode:   200,
		}); err != nil {
			t.Fatalf("FinalizeUsageEvent(%s): %v", reqID, err)
		}
	}

	makeEvent("req_prod_gpt", tokenProdID, "gpt-5.2")
	makeEvent("req_prod_claude", tokenProdID, "claude-3")
	makeEvent("req_other_gpt", tokenOtherID, "gpt-5.2")

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "u@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/events?index=key,model&q_key=prod&q_model=gpt", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
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
	if rid, _ := got.Data.Events[0]["request_id"].(string); rid != "req_prod_gpt" {
		t.Fatalf("expected request_id=req_prod_gpt, got %v", got.Data.Events[0]["request_id"])
	}
}

func TestUsageTimeSeries_UserResponse_ReturnsPoints(t *testing.T) {
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
	userID, err := st.CreateUser(ctx, "series@example.com", "series", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	tokenName := "t-series"
	tokenID, _, err := st.CreateUserToken(ctx, userID, &tokenName, "sk-series-123")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	usageEventID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_series_1",
		UserID:           userID,
		TokenID:          tokenID,
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage: %v", err)
	}

	inTokens := int64(100)
	outTokens := int64(50)
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID: usageEventID,
		InputTokens:  &inTokens,
		OutputTokens: &outTokens,
		CommittedUSD: decimal.RequireFromString("1.23"),
	}); err != nil {
		t.Fatalf("CommitUsage: %v", err)
	}

	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID:        usageEventID,
		Endpoint:            "/v1/responses",
		Method:              "POST",
		StatusCode:          200,
		LatencyMS:           1000,
		FirstTokenLatencyMS: 200,
	}); err != nil {
		t.Fatalf("FinalizeUsageEvent: %v", err)
	}
	insertUsageEventRow(t, db, "req_series_reserved_noise", userID, tokenID, store.UsageStateReserved, time.Now().UTC(), 900, 600, "0", "9.99", 5000, 1000)

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
		"login":    "series@example.com",
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

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/timeseries?granularity=hour", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("usage timeseries status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Granularity string `json:"granularity"`
			Points      []struct {
				Requests             int64   `json:"requests"`
				Tokens               int64   `json:"tokens"`
				CommittedUSD         float64 `json:"committed_usd"`
				AvgFirstTokenLatency float64 `json:"avg_first_token_latency"`
				TokensPerSecond      float64 `json:"tokens_per_second"`
			} `json:"points"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal usage timeseries: %v", err)
	}
	if !got.Success {
		t.Fatalf("usage timeseries expected success, got message=%q", got.Message)
	}
	if got.Data.Granularity != "hour" {
		t.Fatalf("expected granularity=hour, got %q", got.Data.Granularity)
	}
	if len(got.Data.Points) == 0 {
		t.Fatalf("expected non-empty timeseries points")
	}
	point := got.Data.Points[len(got.Data.Points)-1]
	if point.Requests != 1 {
		t.Fatalf("expected point requests=1, got %d", point.Requests)
	}
	if point.Tokens != 150 {
		t.Fatalf("expected point tokens=150, got %d", point.Tokens)
	}
	if point.CommittedUSD <= 0 {
		t.Fatalf("expected point committed_usd > 0, got %f", point.CommittedUSD)
	}
	if point.AvgFirstTokenLatency != 200 {
		t.Fatalf("expected point avg_first_token_latency=200, got %f", point.AvgFirstTokenLatency)
	}
	if point.TokensPerSecond != 62.5 {
		t.Fatalf("expected point tokens_per_second=62.5, got %f", point.TokensPerSecond)
	}
}

func TestUsageTimeSeries_DayDefaultsToLast30Days(t *testing.T) {
	st, db, closeDB := newTestSQLiteStoreWithDB(t)
	defer closeDB()

	oldNow := usageTimeSeriesNow
	fixedNow := time.Date(2026, 3, 14, 15, 30, 0, 0, time.UTC)
	usageTimeSeriesNow = func() time.Time { return fixedNow }
	defer func() {
		usageTimeSeriesNow = oldNow
	}()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userID, err := st.CreateUser(ctx, "series_30d@example.com", "series30d", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_series_30d")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	insertUsageEventRow(t, db, "req_series_30d_in", userID, tokenID, store.UsageStateCommitted, time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), 20, 10, "1.50", "0", 800, 100)
	insertUsageEventRow(t, db, "req_series_30d_out", userID, tokenID, store.UsageStateCommitted, time.Date(2026, 1, 31, 10, 0, 0, 0, time.UTC), 30, 15, "2.00", "0", 900, 120)

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "series_30d@example.com", "password123")

	type usageTimeSeriesResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Start       string `json:"start"`
			End         string `json:"end"`
			Granularity string `json:"granularity"`
			Points      []struct {
				Bucket string `json:"bucket"`
			} `json:"points"`
		} `json:"data"`
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/timeseries?granularity=day&tz=UTC", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("usage timeseries(day) status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got usageTimeSeriesResp
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal usage timeseries(day): %v", err)
	}
	if !got.Success {
		t.Fatalf("usage timeseries(day) expected success, got message=%q", got.Message)
	}
	if got.Data.Start != "2026-02-13" || got.Data.End != "2026-03-14" {
		t.Fatalf("expected default range 2026-02-13~2026-03-14, got %s~%s", got.Data.Start, got.Data.End)
	}
	if got.Data.Granularity != "day" {
		t.Fatalf("expected granularity=day, got %q", got.Data.Granularity)
	}
	if len(got.Data.Points) != 1 {
		t.Fatalf("expected 1 point inside 30-day default range, got %d", len(got.Data.Points))
	}
	if !strings.HasPrefix(got.Data.Points[0].Bucket, "2026-03-01") {
		t.Fatalf("expected in-range bucket on 2026-03-01, got %q", got.Data.Points[0].Bucket)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/timeseries?granularity=day&tz=UTC&start=2026-01-31&end=2026-01-31", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("usage timeseries(day, explicit) status=%d body=%s", rr.Code, rr.Body.String())
	}

	got = usageTimeSeriesResp{}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal usage timeseries(day, explicit): %v", err)
	}
	if !got.Success {
		t.Fatalf("usage timeseries(day, explicit) expected success, got message=%q", got.Message)
	}
	if got.Data.Start != "2026-01-31" || got.Data.End != "2026-01-31" {
		t.Fatalf("expected explicit range 2026-01-31~2026-01-31, got %s~%s", got.Data.Start, got.Data.End)
	}
	if len(got.Data.Points) != 1 || !strings.HasPrefix(got.Data.Points[0].Bucket, "2026-01-31") {
		t.Fatalf("expected explicit range to keep out-of-window bucket, got %#v", got.Data.Points)
	}
}

func TestUsageEvents_TokenFilter_WorksAndChecksOwnership(t *testing.T) {
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
	user1ID, err := st.CreateUser(ctx, "u1@example.com", "u1", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(u1): %v", err)
	}
	user2ID, err := st.CreateUser(ctx, "u2@example.com", "u2", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(u2): %v", err)
	}

	t1Name := "t1"
	t1ID, _, err := st.CreateUserToken(ctx, user1ID, &t1Name, "sk-test-u1-t1")
	if err != nil {
		t.Fatalf("CreateUserToken(t1): %v", err)
	}
	t2Name := "t2"
	t2ID, _, err := st.CreateUserToken(ctx, user1ID, &t2Name, "sk-test-u1-t2")
	if err != nil {
		t.Fatalf("CreateUserToken(t2): %v", err)
	}
	otherName := "other"
	otherTokenID, _, err := st.CreateUserToken(ctx, user2ID, &otherName, "sk-test-u2-t1")
	if err != nil {
		t.Fatalf("CreateUserToken(other): %v", err)
	}

	now := time.Now().UTC()
	newUsageEvent := func(reqID string, tokenID int64) int64 {
		t.Helper()

		usageEventID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        reqID,
			UserID:           user1ID,
			SubscriptionID:   nil,
			TokenID:          tokenID,
			Model:            nil,
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: now.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("ReserveUsage(%s): %v", reqID, err)
		}
		inTokens := int64(1)
		outTokens := int64(2)
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID: usageEventID,
			InputTokens:  &inTokens,
			OutputTokens: &outTokens,
			CommittedUSD: decimal.RequireFromString("1.00"),
		}); err != nil {
			t.Fatalf("CommitUsage(%s): %v", reqID, err)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID:  usageEventID,
			Endpoint:      "/v1/responses",
			Method:        "POST",
			StatusCode:    200,
			LatencyMS:     10,
			IsStream:      false,
			RequestBytes:  123,
			ResponseBytes: 456,
		}); err != nil {
			t.Fatalf("FinalizeUsageEvent(%s): %v", reqID, err)
		}
		return usageEventID
	}

	newUsageEvent("req_u1_t1", t1ID)
	newUsageEvent("req_u1_t2", t2ID)

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

	// login (user1)
	loginBody, _ := json.Marshal(map[string]any{
		"login":    "u1@example.com",
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

	// list usage events filtered by token_id=t1
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/events?token_id="+strconv.FormatInt(t1ID, 10), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(user1ID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("usage events status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool `json:"success"`
		Data    struct {
			Events []struct {
				TokenID int64 `json:"token_id"`
			} `json:"events"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal usage events: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected success, got message=%q", got.Message)
	}
	if len(got.Data.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got.Data.Events))
	}
	if got.Data.Events[0].TokenID != t1ID {
		t.Fatalf("expected token_id=%d, got=%d", t1ID, got.Data.Events[0].TokenID)
	}

	// token_id not owned by user1 should return not found
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/events?token_id="+strconv.FormatInt(otherTokenID, 10), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(user1ID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("usage events (not owned) status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal usage events (not owned): %v", err)
	}
	if got.Success {
		t.Fatalf("expected failure for not owned token_id")
	}
	if got.Message != "not found" {
		t.Fatalf("expected message=%q, got=%q", "not found", got.Message)
	}
}

func TestUsageWindows_TokenFilter_WorksAndChecksOwnership(t *testing.T) {
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
	user1ID, err := st.CreateUser(ctx, "u1w@example.com", "u1w", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(u1): %v", err)
	}
	user2ID, err := st.CreateUser(ctx, "u2w@example.com", "u2w", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(u2): %v", err)
	}

	t1Name := "t1"
	t1ID, _, err := st.CreateUserToken(ctx, user1ID, &t1Name, "sk-test-u1w-t1")
	if err != nil {
		t.Fatalf("CreateUserToken(t1): %v", err)
	}
	t2Name := "t2"
	_, _, err = st.CreateUserToken(ctx, user1ID, &t2Name, "sk-test-u1w-t2")
	if err != nil {
		t.Fatalf("CreateUserToken(t2): %v", err)
	}
	otherName := "other"
	otherTokenID, _, err := st.CreateUserToken(ctx, user2ID, &otherName, "sk-test-u2w-t1")
	if err != nil {
		t.Fatalf("CreateUserToken(other): %v", err)
	}

	now := time.Now().UTC()
	newUsageEvent := func(reqID string, tokenID int64, committedUSD string, inTok, outTok int64) {
		t.Helper()

		usageEventID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        reqID,
			UserID:           user1ID,
			SubscriptionID:   nil,
			TokenID:          tokenID,
			Model:            nil,
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: now.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("ReserveUsage(%s): %v", reqID, err)
		}
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID: usageEventID,
			InputTokens:  &inTok,
			OutputTokens: &outTok,
			CommittedUSD: decimal.RequireFromString(committedUSD),
		}); err != nil {
			t.Fatalf("CommitUsage(%s): %v", reqID, err)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID:  usageEventID,
			Endpoint:      "/v1/responses",
			Method:        "POST",
			StatusCode:    200,
			LatencyMS:     10,
			IsStream:      false,
			RequestBytes:  123,
			ResponseBytes: 456,
		}); err != nil {
			t.Fatalf("FinalizeUsageEvent(%s): %v", reqID, err)
		}
	}

	newUsageEvent("req_u1w_t1", t1ID, "1.23", 10, 5)
	insertUsageEventRow(t, db, "req_u1w_t1_reserved_noise", user1ID, t1ID, store.UsageStateReserved, time.Now().UTC(), 900, 600, "0", "9.99", 5000, 1000)

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

	// login (user1)
	loginBody, _ := json.Marshal(map[string]any{
		"login":    "u1w@example.com",
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

	// /api/usage/windows filtered by token_id=t1
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/windows?token_id="+strconv.FormatInt(t1ID, 10), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(user1ID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("usage windows status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Windows []struct {
				Requests     int64           `json:"requests"`
				Tokens       int64           `json:"tokens"`
				UsedUSD      decimal.Decimal `json:"used_usd"`
				CommittedUSD decimal.Decimal `json:"committed_usd"`
				ReservedUSD  decimal.Decimal `json:"reserved_usd"`
			} `json:"windows"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal usage windows: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected success, got message=%q", got.Message)
	}
	if len(got.Data.Windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(got.Data.Windows))
	}
	w := got.Data.Windows[0]
	if w.Requests != 1 {
		t.Fatalf("requests mismatch: got=%d want=%d", w.Requests, 1)
	}
	if w.Tokens != 15 {
		t.Fatalf("tokens mismatch: got=%d want=%d", w.Tokens, 15)
	}
	if gotUSD := w.CommittedUSD.StringFixed(2); gotUSD != "1.23" {
		t.Fatalf("committed_usd mismatch: got=%s want=%s", gotUSD, "1.23")
	}
	if gotUSD := w.ReservedUSD.StringFixed(2); gotUSD != "9.99" {
		t.Fatalf("reserved_usd mismatch: got=%s want=%s", gotUSD, "9.99")
	}
	if gotUSD := w.UsedUSD.StringFixed(2); gotUSD != "11.22" {
		t.Fatalf("used_usd mismatch: got=%s want=%s", gotUSD, "11.22")
	}

	// token_id not owned by user1 should return not found
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/usage/windows?token_id="+strconv.FormatInt(otherTokenID, 10), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(user1ID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("usage windows (not owned) status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal usage windows (not owned): %v", err)
	}
	if got.Success {
		t.Fatalf("expected failure for not owned token_id")
	}
	if got.Message != "not found" {
		t.Fatalf("expected message=%q, got=%q", "not found", got.Message)
	}
}

func TestV1Usage_TokenAuth_IsSingleKeyAndCannotAccessOtherTokenEvents(t *testing.T) {
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
	userID, err := st.CreateUser(ctx, "u@example.com", "u", []byte("pw-hash"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	t1Name := "t1"
	rawT1 := "sk-test-t1"
	t1ID, _, err := st.CreateUserToken(ctx, userID, &t1Name, rawT1)
	if err != nil {
		t.Fatalf("CreateUserToken(t1): %v", err)
	}
	t2Name := "t2"
	rawT2 := "sk-test-t2"
	t2ID, _, err := st.CreateUserToken(ctx, userID, &t2Name, rawT2)
	if err != nil {
		t.Fatalf("CreateUserToken(t2): %v", err)
	}

	now := time.Now().UTC()
	newUsageEvent := func(reqID string, tokenID int64) int64 {
		t.Helper()

		usageEventID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        reqID,
			UserID:           userID,
			SubscriptionID:   nil,
			TokenID:          tokenID,
			Model:            nil,
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: now.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("ReserveUsage(%s): %v", reqID, err)
		}
		inTokens := int64(1)
		outTokens := int64(2)
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID: usageEventID,
			InputTokens:  &inTokens,
			OutputTokens: &outTokens,
			CommittedUSD: decimal.Zero,
		}); err != nil {
			t.Fatalf("CommitUsage(%s): %v", reqID, err)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID:  usageEventID,
			Endpoint:      "/v1/responses",
			Method:        "POST",
			StatusCode:    200,
			LatencyMS:     10,
			IsStream:      false,
			RequestBytes:  123,
			ResponseBytes: 456,
		}); err != nil {
			t.Fatalf("FinalizeUsageEvent(%s): %v", reqID, err)
		}
		return usageEventID
	}

	newUsageEvent("req_t1", t1ID)
	ev2ID := newUsageEvent("req_t2", t2ID)

	engine := gin.New()
	engine.Use(gin.Recovery())
	SetRouter(engine, Options{
		Store:             st,
		FrontendIndexPage: []byte("<!doctype html><html><body>INDEX</body></html>"),
	})

	// list /v1/usage/events with token1 should only see token1 events
	req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/usage/events", nil)
	req.Header.Set("Authorization", "Bearer "+rawT1)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("v1 usage events status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Events []struct {
				TokenID int64 `json:"token_id"`
			} `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal v1 usage events: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected success, got message=%q", got.Message)
	}
	if len(got.Data.Events) != 1 || got.Data.Events[0].TokenID != t1ID {
		t.Fatalf("expected exactly one event for token1")
	}

	// token1 cannot access token2's event detail
	req = httptest.NewRequest(http.MethodGet, "http://example.com/v1/usage/events/"+strconv.FormatInt(ev2ID, 10)+"/detail", nil)
	req.Header.Set("Authorization", "Bearer "+rawT1)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("v1 usage event detail status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal v1 usage event detail: %v", err)
	}
	if got.Success {
		t.Fatalf("expected not found for other token event detail")
	}
	if got.Message != "not found" {
		t.Fatalf("expected message=%q, got=%q", "not found", got.Message)
	}

	// /v1/usage rejects token_id query param
	req = httptest.NewRequest(http.MethodGet, "http://example.com/v1/usage/events?token_id=1", nil)
	req.Header.Set("Authorization", "Bearer "+rawT2)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("v1 usage events (token_id param) status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal v1 usage events (token_id param): %v", err)
	}
	if got.Success {
		t.Fatalf("expected failure when token_id param present")
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
	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5")); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}
	if _, err := st.CreateChannelGroup(ctx, "staff", nil, 1, decimal.RequireFromString("2")); err != nil {
		t.Fatalf("CreateChannelGroup(staff): %v", err)
	}

	modelID := "m_detail_1"
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            modelID,
		GroupName:           "staff",
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
	groupName := "staff"
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID:             usageEventID,
		InputTokens:              &inTokens,
		CachedInputTokens:        &cachedInTokens,
		OutputTokens:             &outTokens,
		CachedOutputTokens:       &cachedOutTokens,
		CommittedUSD:             decimal.RequireFromString("9.3"),
		PriceMultiplier:          decimal.RequireFromString("3"),
		PriceMultiplierGroup:     decimal.RequireFromString("2"),
		PriceMultiplierPayment:   decimal.RequireFromString("1.5"),
		PriceMultiplierGroupName: &groupName,
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
				PaymentMultiplier    string `json:"payment_multiplier"`
				GroupName            string `json:"group_name"`
				GroupMultiplier      string `json:"group_multiplier"`
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
	if got.Data.PricingBreakdown.PaymentMultiplier != "1.5" {
		t.Fatalf("payment_multiplier mismatch: got=%q want=%q", got.Data.PricingBreakdown.PaymentMultiplier, "1.5")
	}
	if got.Data.PricingBreakdown.GroupName != "staff" {
		t.Fatalf("group_name mismatch: got=%q want=%q", got.Data.PricingBreakdown.GroupName, "staff")
	}
	if got.Data.PricingBreakdown.GroupMultiplier != "2" {
		t.Fatalf("group_multiplier mismatch: got=%q want=%q", got.Data.PricingBreakdown.GroupMultiplier, "2")
	}
	if got.Data.PricingBreakdown.EffectiveMultiplier != "3" {
		t.Fatalf("effective_multiplier mismatch: got=%q want=%q", got.Data.PricingBreakdown.EffectiveMultiplier, "3")
	}
	if got.Data.PricingBreakdown.FinalCostUSD != "9.3" {
		t.Fatalf("final_cost_usd mismatch: got=%q want=%q", got.Data.PricingBreakdown.FinalCostUSD, "9.3")
	}
}

func TestUsageEventDetail_UserResponse_IncludesNestedGroupPathPricingBreakdown(t *testing.T) {
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
	userID, err := st.CreateUser(ctx, "detail-nested@example.com", "detailnested", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	modelID := "m_detail_nested"
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            modelID,
		GroupName:           "parent",
		InputUSDPer1M:       decimal.RequireFromString("1"),
		OutputUSDPer1M:      decimal.Zero,
		CacheInputUSDPer1M:  decimal.Zero,
		CacheOutputUSDPer1M: decimal.Zero,
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}

	tokenName := "t-detail-nested"
	tokenID, _, err := st.CreateUserToken(ctx, userID, &tokenName, "sk-detail-nested-123")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	usageEventID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_detail_nested_1",
		UserID:           userID,
		SubscriptionID:   nil,
		TokenID:          tokenID,
		Model:            &modelID,
		ReservedUSD:      decimal.RequireFromString("1.8"),
		ReserveExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage: %v", err)
	}

	inTokens := int64(1_000_000)
	groupPath := "parent/child"
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID:             usageEventID,
		InputTokens:              &inTokens,
		CommittedUSD:             decimal.RequireFromString("1.8"),
		PriceMultiplier:          decimal.RequireFromString("1.8"),
		PriceMultiplierGroup:     decimal.RequireFromString("1.8"),
		PriceMultiplierPayment:   decimal.RequireFromString("1"),
		PriceMultiplierGroupName: &groupPath,
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
		FrontendIndexPage: []byte("<!doctype html><html><body>INDEX</body></html>"),
	})

	loginBody, _ := json.Marshal(map[string]any{
		"login":    "detail-nested@example.com",
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
			PricingBreakdown struct {
				GroupName           string `json:"group_name"`
				GroupMultiplier     string `json:"group_multiplier"`
				PaymentMultiplier   string `json:"payment_multiplier"`
				EffectiveMultiplier string `json:"effective_multiplier"`
				FinalCostUSD        string `json:"final_cost_usd"`
			} `json:"pricing_breakdown"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal detail: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected success, got message=%q", got.Message)
	}
	if got.Data.PricingBreakdown.GroupName != "parent/child" {
		t.Fatalf("group_name mismatch: got=%q want=%q", got.Data.PricingBreakdown.GroupName, "parent/child")
	}
	if got.Data.PricingBreakdown.GroupMultiplier != "1.8" {
		t.Fatalf("group_multiplier mismatch: got=%q want=%q", got.Data.PricingBreakdown.GroupMultiplier, "1.8")
	}
	if got.Data.PricingBreakdown.PaymentMultiplier != "1" {
		t.Fatalf("payment_multiplier mismatch: got=%q want=%q", got.Data.PricingBreakdown.PaymentMultiplier, "1")
	}
	if got.Data.PricingBreakdown.EffectiveMultiplier != "1.8" {
		t.Fatalf("effective_multiplier mismatch: got=%q want=%q", got.Data.PricingBreakdown.EffectiveMultiplier, "1.8")
	}
	if got.Data.PricingBreakdown.FinalCostUSD != "1.8" {
		t.Fatalf("final_cost_usd mismatch: got=%q want=%q", got.Data.PricingBreakdown.FinalCostUSD, "1.8")
	}
}

func TestParseDateRangeInLocation_UsesLocalDayBoundary(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	nowUTC := time.Date(2026, 2, 8, 1, 30, 0, 0, time.UTC) // CST: 2026-02-08 09:30

	sinceUTC, untilUTC, sinceLocal, untilLocal, ok := parseDateRangeInLocation(nowUTC, "2026-02-08", "2026-02-08", loc)
	if !ok {
		t.Fatalf("expected local date range to be accepted")
	}
	if got, want := sinceLocal, time.Date(2026, 2, 8, 0, 0, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("sinceLocal=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if got, want := untilLocal, time.Date(2026, 2, 8, 9, 30, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("untilLocal=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if got, want := sinceUTC, time.Date(2026, 2, 7, 16, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("sinceUTC=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if got, want := untilUTC, nowUTC; !got.Equal(want) {
		t.Fatalf("untilUTC=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestUsageEventDetail_PricingBreakdown_ExposesGroupPathMultiplier(t *testing.T) {
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
	userID, err := st.CreateUser(ctx, "group-path@example.com", "grouppath", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "sk-group-path")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            "nested-model",
		GroupName:           "parent",
		InputUSDPer1M:       decimal.RequireFromString("1"),
		OutputUSDPer1M:      decimal.Zero,
		CacheInputUSDPer1M:  decimal.Zero,
		CacheOutputUSDPer1M: decimal.Zero,
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}

	usageEventID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_group_path_detail_1",
		UserID:           userID,
		TokenID:          tokenID,
		Model:            optionalStringForUsageRouteTest("nested-model"),
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage: %v", err)
	}
	inTokens := int64(1_000_000)
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID:             usageEventID,
		InputTokens:              &inTokens,
		CommittedUSD:             decimal.RequireFromString("1.8"),
		PriceMultiplier:          decimal.RequireFromString("1.8"),
		PriceMultiplierGroup:     decimal.RequireFromString("1.8"),
		PriceMultiplierPayment:   decimal.RequireFromString("1"),
		PriceMultiplierGroupName: optionalStringForUsageRouteTest("parent/child"),
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
		FrontendIndexPage: []byte("<!doctype html><html><body>INDEX</body></html>"),
	})

	loginBody, _ := json.Marshal(map[string]any{
		"login":    "group-path@example.com",
		"password": "password123",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/user/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	loginResp := httptest.NewRecorder()
	engine.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginResp.Code, loginResp.Body.String())
	}
	sessionCookie := ""
	for _, cookieItem := range loginResp.Result().Cookies() {
		if cookieItem.Name == cookieName {
			sessionCookie = cookieItem.String()
			break
		}
	}
	if sessionCookie == "" {
		t.Fatalf("expected session cookie %q", cookieName)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/usage/events/"+strconv.FormatInt(usageEventID, 10)+"/detail", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			PricingBreakdown struct {
				GroupName           string `json:"group_name"`
				GroupMultiplier     string `json:"group_multiplier"`
				EffectiveMultiplier string `json:"effective_multiplier"`
			} `json:"pricing_breakdown"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal detail: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected success, got message=%q", got.Message)
	}
	if got.Data.PricingBreakdown.GroupName != "parent/child" {
		t.Fatalf("group_name mismatch: got=%q want=%q", got.Data.PricingBreakdown.GroupName, "parent/child")
	}
	if got.Data.PricingBreakdown.GroupMultiplier != "1.8" {
		t.Fatalf("group_multiplier mismatch: got=%q want=%q", got.Data.PricingBreakdown.GroupMultiplier, "1.8")
	}
	if got.Data.PricingBreakdown.EffectiveMultiplier != "1.8" {
		t.Fatalf("effective_multiplier mismatch: got=%q want=%q", got.Data.PricingBreakdown.EffectiveMultiplier, "1.8")
	}
}
