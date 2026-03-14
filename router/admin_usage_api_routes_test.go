package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/store"
)

func TestAdminUsagePage_EventIncludesFirstTokenLatencyAndTokensPerSecond(t *testing.T) {
	st, closeDB := newTestSQLiteStore(t)
	defer closeDB()

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

	chID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "channel-1", "", 0, false, false, false, false)
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

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "root@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
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

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage/timeseries?granularity=hour", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin usage timeseries status=%d body=%s", rr.Code, rr.Body.String())
	}

	var ts struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Granularity string `json:"granularity"`
			Points      []struct {
				Requests             int64   `json:"requests"`
				Tokens               int64   `json:"tokens"`
				CommittedUSD         float64 `json:"committed_usd"`
				CacheRatio           float64 `json:"cache_ratio"`
				AvgFirstTokenLatency float64 `json:"avg_first_token_latency"`
				TokensPerSecond      float64 `json:"tokens_per_second"`
			} `json:"points"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &ts); err != nil {
		t.Fatalf("json.Unmarshal admin usage timeseries: %v", err)
	}
	if !ts.Success {
		t.Fatalf("admin usage timeseries expected success, got message=%q", ts.Message)
	}
	if ts.Data.Granularity != "hour" {
		t.Fatalf("expected granularity=hour, got %q", ts.Data.Granularity)
	}
	if len(ts.Data.Points) == 0 {
		t.Fatalf("expected non-empty timeseries points")
	}
	point := ts.Data.Points[len(ts.Data.Points)-1]
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

func TestAdminUsagePage_ExposesModelCheck(t *testing.T) {
	st, closeDB := newTestSQLiteStore(t)
	defer closeDB()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userID, err := st.CreateUser(ctx, "root-model@example.com", "rootmodel", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_admin_model_check")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_admin_model_check_1",
		UserID:           userID,
		TokenID:          tokenID,
		Model:            adminUsageOptionalString("alias"),
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage: %v", err)
	}
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID: usageID,
		CommittedUSD: decimal.Zero,
	}); err != nil {
		t.Fatalf("CommitUsage: %v", err)
	}

	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID:          usageID,
		Endpoint:              "/v1/responses",
		Method:                "POST",
		StatusCode:            200,
		ForwardedModel:        adminUsageOptionalString("gpt-5.2"),
		UpstreamResponseModel: adminUsageOptionalString("gpt-5.2-mini"),
	}); err != nil {
		t.Fatalf("FinalizeUsageEvent: %v", err)
	}

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "root-model@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin usage status=%d body=%s", rr.Code, rr.Body.String())
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
		t.Fatalf("json.Unmarshal admin usage: %v", err)
	}
	if !listResp.Success {
		t.Fatalf("admin usage expected success, got message=%q", listResp.Message)
	}
	if len(listResp.Data.Events) != 1 || !listResp.Data.Events[0].ModelMismatch {
		t.Fatalf("expected model_mismatch=true, got=%+v", listResp.Data.Events)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage/events/"+strconv.FormatInt(usageID, 10)+"/detail", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin detail status=%d body=%s", rr.Code, rr.Body.String())
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
		t.Fatalf("json.Unmarshal admin detail: %v", err)
	}
	if !detailResp.Success {
		t.Fatalf("admin detail expected success, got message=%q", detailResp.Message)
	}
	if detailResp.Data.ModelCheck.ForwardedModel != "gpt-5.2" || detailResp.Data.ModelCheck.UpstreamResponseModel != "gpt-5.2-mini" || !detailResp.Data.ModelCheck.Mismatch {
		t.Fatalf("unexpected model_check=%+v", detailResp.Data.ModelCheck)
	}
}

func adminUsageOptionalString(v string) *string {
	return &v
}

func TestAdminUsagePage_WindowAndTimeseries_IgnoreNonCommittedRows(t *testing.T) {
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
	userID, err := st.CreateUser(ctx, "root_usage_noise@example.com", "rootusagenoise", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_usage_noise")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	now := time.Now().UTC()
	insertUsageEventRow(t, db, "req_admin_usage_committed", userID, tokenID, store.UsageStateCommitted, now, 100, 50, "1.23", "0", 1000, 200)
	insertUsageEventRow(t, db, "req_admin_usage_reserved_noise", userID, tokenID, store.UsageStateReserved, now, 900, 600, "0", "9.99", 5000, 1000)

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "root_usage_noise@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin usage status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Window struct {
				Requests             int64  `json:"requests"`
				Tokens               int64  `json:"tokens"`
				InputTokens          int64  `json:"input_tokens"`
				OutputTokens         int64  `json:"output_tokens"`
				AvgFirstTokenLatency string `json:"avg_first_token_latency"`
				TokensPerSecond      string `json:"tokens_per_second"`
				CommittedUSD         string `json:"committed_usd"`
				ReservedUSD          string `json:"reserved_usd"`
				TotalUSD             string `json:"total_usd"`
			} `json:"window"`
			Events []struct {
				RequestID string `json:"request_id"`
			} `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal admin usage: %v", err)
	}
	if !got.Success {
		t.Fatalf("admin usage expected success, got message=%q", got.Message)
	}
	if got.Data.Window.Requests != 1 {
		t.Fatalf("expected window requests=1, got %d", got.Data.Window.Requests)
	}
	if got.Data.Window.Tokens != 150 {
		t.Fatalf("expected window tokens=150, got %d", got.Data.Window.Tokens)
	}
	if got.Data.Window.InputTokens != 100 || got.Data.Window.OutputTokens != 50 {
		t.Fatalf("expected input/output=100/50, got %d/%d", got.Data.Window.InputTokens, got.Data.Window.OutputTokens)
	}
	if got.Data.Window.AvgFirstTokenLatency != "200.0 ms" {
		t.Fatalf("expected avg_first_token_latency=200.0 ms, got %q", got.Data.Window.AvgFirstTokenLatency)
	}
	if got.Data.Window.TokensPerSecond != "62.50" {
		t.Fatalf("expected tokens_per_second=62.50, got %q", got.Data.Window.TokensPerSecond)
	}
	if got.Data.Window.CommittedUSD != "1.23" {
		t.Fatalf("expected committed_usd=1.23, got %q", got.Data.Window.CommittedUSD)
	}
	if got.Data.Window.ReservedUSD != "9.99" {
		t.Fatalf("expected reserved_usd=9.99, got %q", got.Data.Window.ReservedUSD)
	}
	if got.Data.Window.TotalUSD != "11.22" {
		t.Fatalf("expected total_usd=11.22, got %q", got.Data.Window.TotalUSD)
	}
	if len(got.Data.Events) == 0 {
		t.Fatalf("expected non-empty events list")
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage/timeseries?granularity=hour", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin usage timeseries status=%d body=%s", rr.Code, rr.Body.String())
	}

	var ts struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Points []struct {
				Requests             int64   `json:"requests"`
				Tokens               int64   `json:"tokens"`
				CommittedUSD         float64 `json:"committed_usd"`
				AvgFirstTokenLatency float64 `json:"avg_first_token_latency"`
				TokensPerSecond      float64 `json:"tokens_per_second"`
			} `json:"points"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &ts); err != nil {
		t.Fatalf("json.Unmarshal admin usage timeseries: %v", err)
	}
	if !ts.Success {
		t.Fatalf("admin usage timeseries expected success, got message=%q", ts.Message)
	}
	if len(ts.Data.Points) == 0 {
		t.Fatalf("expected non-empty timeseries points")
	}
	point := ts.Data.Points[len(ts.Data.Points)-1]
	if point.Requests != 1 {
		t.Fatalf("expected point requests=1, got %d", point.Requests)
	}
	if point.Tokens != 150 {
		t.Fatalf("expected point tokens=150, got %d", point.Tokens)
	}
	if point.CommittedUSD != 1.23 {
		t.Fatalf("expected point committed_usd=1.23, got %f", point.CommittedUSD)
	}
	if point.AvgFirstTokenLatency != 200 {
		t.Fatalf("expected point avg_first_token_latency=200, got %f", point.AvgFirstTokenLatency)
	}
	if point.TokensPerSecond != 62.5 {
		t.Fatalf("expected point tokens_per_second=62.5, got %f", point.TokensPerSecond)
	}
}

func TestAdminUsageTimeSeries_DayDefaultsToLast30Days(t *testing.T) {
	st, db, closeDB := newTestSQLiteStoreWithDB(t)
	defer closeDB()

	oldNow := adminUsageTimeSeriesNow
	fixedNow := time.Date(2026, 3, 14, 4, 30, 0, 0, time.UTC)
	adminUsageTimeSeriesNow = func() time.Time { return fixedNow }
	defer func() {
		adminUsageTimeSeriesNow = oldNow
	}()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userID, err := st.CreateUser(ctx, "root_30d@example.com", "root30d", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_admin_30d")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	insertUsageEventRow(t, db, "req_admin_30d_in", userID, tokenID, store.UsageStateCommitted, time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC), 40, 20, "3.25", "0", 700, 100)
	insertUsageEventRow(t, db, "req_admin_30d_out", userID, tokenID, store.UsageStateCommitted, time.Date(2026, 1, 31, 9, 0, 0, 0, time.UTC), 10, 5, "0.75", "0", 600, 90)

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "root_30d@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage/timeseries?granularity=day", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin usage timeseries(day) status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
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
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal admin usage timeseries(day): %v", err)
	}
	if !got.Success {
		t.Fatalf("admin usage timeseries(day) expected success, got message=%q", got.Message)
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
	if !strings.HasPrefix(got.Data.Points[0].Bucket, "2026-03-05") {
		t.Fatalf("expected in-range bucket on 2026-03-05, got %q", got.Data.Points[0].Bucket)
	}
}

func TestAdminUsagePage_IndexFilters_UserChannelAndModel(t *testing.T) {
	st, closeDB := newTestSQLiteStore(t)
	defer closeDB()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	rootID, err := st.CreateUser(ctx, "root@example.com", "root", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser(root): %v", err)
	}
	aliceID, err := st.CreateUser(ctx, "alice@example.com", "alice", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(alice): %v", err)
	}

	rootKeyName := "root-key"
	rootTokenID, _, err := st.CreateUserToken(ctx, rootID, &rootKeyName, "tok_root")
	if err != nil {
		t.Fatalf("CreateUserToken(root): %v", err)
	}
	aliceKeyName := "alice-key"
	aliceTokenID, _, err := st.CreateUserToken(ctx, aliceID, &aliceKeyName, "tok_alice")
	if err != nil {
		t.Fatalf("CreateUserToken(alice): %v", err)
	}

	ch1, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "channel-1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(ch1): %v", err)
	}
	ch2, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "channel-2", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(ch2): %v", err)
	}

	createEvent := func(userID int64, tokenID int64, requestID string, chID int64, model string) {
		usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        requestID,
			UserID:           userID,
			TokenID:          tokenID,
			Model:            &model,
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("ReserveUsage(%s): %v", requestID, err)
		}
		chRef := chID
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID:      usageID,
			UpstreamChannelID: &chRef,
			CommittedUSD:      decimal.Zero,
		}); err != nil {
			t.Fatalf("CommitUsage(%s): %v", requestID, err)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID:      usageID,
			Endpoint:          "/v1/responses",
			Method:            "POST",
			StatusCode:        200,
			UpstreamChannelID: &chRef,
		}); err != nil {
			t.Fatalf("FinalizeUsageEvent(%s): %v", requestID, err)
		}
	}

	createEvent(rootID, rootTokenID, "req_filter_root_gpt", ch1, "gpt-5.2")
	createEvent(aliceID, aliceTokenID, "req_filter_alice_gpt", ch2, "gpt-5.2")
	createEvent(aliceID, aliceTokenID, "req_filter_alice_claude", ch2, "claude-3")

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "root@example.com", "password123")

	list := func(url string) []map[string]any {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
		req.Header.Set("Cookie", sessionCookie)
		rr := httptest.NewRecorder()
		engine.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("admin usage status=%d body=%s", rr.Code, rr.Body.String())
		}
		var got struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Data    struct {
				Events []map[string]any `json:"events"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatalf("json.Unmarshal admin usage: %v", err)
		}
		if !got.Success {
			t.Fatalf("admin usage expected success, got message=%q", got.Message)
		}
		return got.Data.Events
	}

	if events := list("http://example.com/api/admin/usage?index=user&q=alice"); len(events) != 2 {
		t.Fatalf("expected 2 events for user=alice, got %d", len(events))
	}
	if events := list("http://example.com/api/admin/usage?index=channel&q=channel-2"); len(events) != 2 {
		t.Fatalf("expected 2 events for channel-2, got %d", len(events))
	}
	if events := list("http://example.com/api/admin/usage?index=channel&q=999999"); len(events) != 0 {
		t.Fatalf("expected 0 event for missing channel id, got %d", len(events))
	}

	baseline := list("http://example.com/api/admin/usage")
	if len(baseline) != 3 {
		t.Fatalf("expected baseline 3 events, got %d", len(baseline))
	}
	// /admin/usage does not index key; index=key/q_key should be ignored for compatibility.
	if events := list("http://example.com/api/admin/usage?index=key&q=root-key"); len(events) != len(baseline) {
		t.Fatalf("expected index=key to be ignored (len=%d), got %d", len(baseline), len(events))
	}
	if events := list("http://example.com/api/admin/usage?q_key=alice-key"); len(events) != len(baseline) {
		t.Fatalf("expected q_key to be ignored (len=%d), got %d", len(baseline), len(events))
	}

	events := list("http://example.com/api/admin/usage?index=user,channel,model&q_user=alice&q_channel=channel-2&q_model=gpt")
	if len(events) != 1 {
		t.Fatalf("expected 1 event for AND filters, got %d", len(events))
	}
	if rid, _ := events[0]["request_id"].(string); rid != "req_filter_alice_gpt" {
		t.Fatalf("expected request_id=req_filter_alice_gpt, got %v", events[0]["request_id"])
	}
}

func TestAdminUsagePage_UpstreamUnavailable_ShowsDetailedErrorMessage(t *testing.T) {
	st, closeDB := newTestSQLiteStore(t)
	defer closeDB()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userID, err := st.CreateUser(ctx, "root2@example.com", "root2", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_admin_usage_2")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_admin_usage_2",
		UserID:           userID,
		TokenID:          tokenID,
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage: %v", err)
	}

	inTokens := int64(1)
	outTokens := int64(2)
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID: usageID,
		InputTokens:  &inTokens,
		OutputTokens: &outTokens,
		CommittedUSD: decimal.Zero,
	}); err != nil {
		t.Fatalf("CommitUsage: %v", err)
	}

	errClass := "upstream_unavailable"
	errMsg := "上游不可用；最后一次失败: upstream_status 429 Too Many Requests"
	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID:  usageID,
		Endpoint:      "/v1/responses",
		Method:        "POST",
		StatusCode:    502,
		LatencyMS:     321,
		ErrorClass:    &errClass,
		ErrorMessage:  &errMsg,
		RequestBytes:  10,
		ResponseBytes: 20,
	}); err != nil {
		t.Fatalf("FinalizeUsageEvent: %v", err)
	}

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "root2@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin usage status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Events []struct {
				ErrorClass   string `json:"error_class"`
				ErrorMessage string `json:"error_message"`
			} `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal admin usage: %v", err)
	}
	if !got.Success {
		t.Fatalf("admin usage expected success, got message=%q", got.Message)
	}
	if len(got.Data.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got.Data.Events))
	}
	ev := got.Data.Events[0]
	if ev.ErrorClass != "upstream_unavailable" {
		t.Fatalf("expected error_class=upstream_unavailable, got %q", ev.ErrorClass)
	}
	if ev.ErrorMessage != errMsg {
		t.Fatalf("expected error_message=%q, got %q", errMsg, ev.ErrorMessage)
	}
}

func TestAdminUsagePage_UserIDFilter_Works(t *testing.T) {
	st, closeDB := newTestSQLiteStore(t)
	defer closeDB()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	rootID, err := st.CreateUser(ctx, "root_userid_filter@example.com", "rootuseridfilter", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser(root): %v", err)
	}
	aliceID, err := st.CreateUser(ctx, "alice_userid_filter@example.com", "aliceuseridfilter", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(alice): %v", err)
	}

	rootTokenID, _, err := st.CreateUserToken(ctx, rootID, nil, "tok_root_userid_filter")
	if err != nil {
		t.Fatalf("CreateUserToken(root): %v", err)
	}
	aliceTokenID, _, err := st.CreateUserToken(ctx, aliceID, nil, "tok_alice_userid_filter")
	if err != nil {
		t.Fatalf("CreateUserToken(alice): %v", err)
	}
	ch1, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "channel-userid-filter-1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(ch1): %v", err)
	}

	createEvent := func(userID int64, tokenID int64, requestID string) {
		model := "gpt-5.2"
		usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        requestID,
			UserID:           userID,
			TokenID:          tokenID,
			Model:            &model,
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("ReserveUsage(%s): %v", requestID, err)
		}
		chRef := ch1
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID:      usageID,
			UpstreamChannelID: &chRef,
			CommittedUSD:      decimal.Zero,
		}); err != nil {
			t.Fatalf("CommitUsage(%s): %v", requestID, err)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID:      usageID,
			Endpoint:          "/v1/responses",
			Method:            "POST",
			StatusCode:        200,
			UpstreamChannelID: &chRef,
		}); err != nil {
			t.Fatalf("FinalizeUsageEvent(%s): %v", requestID, err)
		}
	}

	createEvent(rootID, rootTokenID, "req_userid_filter_root")
	createEvent(aliceID, aliceTokenID, "req_userid_filter_alice_1")
	createEvent(aliceID, aliceTokenID, "req_userid_filter_alice_2")

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "root_userid_filter@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage?user_id="+strconv.FormatInt(aliceID, 10), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin usage status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Events []map[string]any `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal admin usage: %v", err)
	}
	if !got.Success {
		t.Fatalf("admin usage expected success, got message=%q", got.Message)
	}
	if len(got.Data.Events) != 2 {
		t.Fatalf("expected 2 events for user_id=%d, got %d", aliceID, len(got.Data.Events))
	}
	for _, e := range got.Data.Events {
		idAny, ok := e["user_id"]
		if !ok {
			t.Fatalf("missing user_id in event: %v", e)
		}
		idNum, ok := idAny.(float64)
		if !ok {
			t.Fatalf("unexpected user_id type %T value=%v", idAny, idAny)
		}
		if int64(idNum) != aliceID {
			t.Fatalf("expected user_id=%d, got %v", aliceID, idAny)
		}
	}
}

func TestAdminUsageUsersSuggest_StripsAtPrefix(t *testing.T) {
	st, closeDB := newTestSQLiteStore(t)
	defer closeDB()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	rootID, err := st.CreateUser(ctx, "root_suggest@example.com", "rootsuggest", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser(root): %v", err)
	}
	aliceID, err := st.CreateUser(ctx, "alice_suggest@example.com", "alice", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(alice): %v", err)
	}

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "root_suggest@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage/users/suggest?q=@ali", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin suggest status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal admin suggest: %v", err)
	}
	if !got.Success {
		t.Fatalf("admin suggest expected success, got message=%q", got.Message)
	}
	found := false
	for _, v := range got.Data {
		if v.ID == aliceID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected suggest to include alice id=%d, got=%v", aliceID, got.Data)
	}
}

func TestAdminUsagePage_UpstreamChannelIDFilter_Works(t *testing.T) {
	st, closeDB := newTestSQLiteStore(t)
	defer closeDB()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	rootID, err := st.CreateUser(ctx, "root_chid_filter@example.com", "rootchidfilter", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser(root): %v", err)
	}
	aliceID, err := st.CreateUser(ctx, "alice_chid_filter@example.com", "alicechidfilter", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(alice): %v", err)
	}

	rootTokenID, _, err := st.CreateUserToken(ctx, rootID, nil, "tok_root_chid_filter")
	if err != nil {
		t.Fatalf("CreateUserToken(root): %v", err)
	}
	aliceTokenID, _, err := st.CreateUserToken(ctx, aliceID, nil, "tok_alice_chid_filter")
	if err != nil {
		t.Fatalf("CreateUserToken(alice): %v", err)
	}

	ch1, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "chid-channel-1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(ch1): %v", err)
	}
	ch2, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "chid-channel-2", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(ch2): %v", err)
	}

	createEvent := func(userID int64, tokenID int64, requestID string, chID int64) {
		model := "gpt-5.2"
		usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        requestID,
			UserID:           userID,
			TokenID:          tokenID,
			Model:            &model,
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("ReserveUsage(%s): %v", requestID, err)
		}
		chRef := chID
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID:      usageID,
			UpstreamChannelID: &chRef,
			CommittedUSD:      decimal.Zero,
		}); err != nil {
			t.Fatalf("CommitUsage(%s): %v", requestID, err)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID:      usageID,
			Endpoint:          "/v1/responses",
			Method:            "POST",
			StatusCode:        200,
			UpstreamChannelID: &chRef,
		}); err != nil {
			t.Fatalf("FinalizeUsageEvent(%s): %v", requestID, err)
		}
	}

	createEvent(rootID, rootTokenID, "req_chid_filter_root", ch1)
	createEvent(aliceID, aliceTokenID, "req_chid_filter_alice_1", ch2)
	createEvent(aliceID, aliceTokenID, "req_chid_filter_alice_2", ch2)

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "root_chid_filter@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage?upstream_channel_id="+strconv.FormatInt(ch2, 10), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin usage status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Events []map[string]any `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal admin usage: %v", err)
	}
	if !got.Success {
		t.Fatalf("admin usage expected success, got message=%q", got.Message)
	}
	if len(got.Data.Events) != 2 {
		t.Fatalf("expected 2 events for upstream_channel_id=%d, got %d", ch2, len(got.Data.Events))
	}
}

func TestAdminUsagePage_ModelExactFilter_Works(t *testing.T) {
	st, closeDB := newTestSQLiteStore(t)
	defer closeDB()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	rootID, err := st.CreateUser(ctx, "root_model_filter@example.com", "rootmodelfilter", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser(root): %v", err)
	}

	tokenID, _, err := st.CreateUserToken(ctx, rootID, nil, "tok_root_model_filter")
	if err != nil {
		t.Fatalf("CreateUserToken(root): %v", err)
	}
	ch1, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "model-channel-1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(ch1): %v", err)
	}

	createEvent := func(requestID string, model string) {
		usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        requestID,
			UserID:           rootID,
			TokenID:          tokenID,
			Model:            &model,
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("ReserveUsage(%s): %v", requestID, err)
		}
		chRef := ch1
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID:      usageID,
			UpstreamChannelID: &chRef,
			CommittedUSD:      decimal.Zero,
		}); err != nil {
			t.Fatalf("CommitUsage(%s): %v", requestID, err)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID:      usageID,
			Endpoint:          "/v1/responses",
			Method:            "POST",
			StatusCode:        200,
			UpstreamChannelID: &chRef,
		}); err != nil {
			t.Fatalf("FinalizeUsageEvent(%s): %v", requestID, err)
		}
	}

	createEvent("req_model_exact_1", "gpt-5.2")
	createEvent("req_model_exact_2", "gpt-4.1")

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "root_model_filter@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage?model=gpt-5.2", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin usage status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Events []map[string]any `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal admin usage: %v", err)
	}
	if !got.Success {
		t.Fatalf("admin usage expected success, got message=%q", got.Message)
	}
	if len(got.Data.Events) != 1 {
		t.Fatalf("expected 1 event for model=gpt-5.2, got %d", len(got.Data.Events))
	}
	if m, _ := got.Data.Events[0]["model"].(string); m != "gpt-5.2" {
		t.Fatalf("expected model=gpt-5.2, got %v", got.Data.Events[0]["model"])
	}
}

func TestAdminUsageSuggest_Channels_RangeScoped(t *testing.T) {
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
	rootID, err := st.CreateUser(ctx, "root_suggest_ch@example.com", "rootsuggestch", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser(root): %v", err)
	}
	tokenID, _, err := st.CreateUserToken(ctx, rootID, nil, "tok_suggest_ch")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}
	chIn, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c-in", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(chIn): %v", err)
	}
	chOut, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c-out", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(chOut): %v", err)
	}

	createEvent := func(requestID string, chID int64) int64 {
		model := "gpt-5.2"
		usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        requestID,
			UserID:           rootID,
			TokenID:          tokenID,
			Model:            &model,
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("ReserveUsage(%s): %v", requestID, err)
		}
		chRef := chID
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID:      usageID,
			UpstreamChannelID: &chRef,
			CommittedUSD:      decimal.Zero,
		}); err != nil {
			t.Fatalf("CommitUsage(%s): %v", requestID, err)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID:      usageID,
			Endpoint:          "/v1/responses",
			Method:            "POST",
			StatusCode:        200,
			UpstreamChannelID: &chRef,
		}); err != nil {
			t.Fatalf("FinalizeUsageEvent(%s): %v", requestID, err)
		}
		return usageID
	}

	_ = createEvent("req_suggest_ch_in", chIn)
	outID := createEvent("req_suggest_ch_out", chOut)

	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	oldTimeStr := oldTime.Format("2006-01-02 15:04:05")
	if _, err := db.ExecContext(ctx, `UPDATE usage_events SET time=? WHERE id=?`, oldTimeStr, outID); err != nil {
		t.Fatalf("update usage_events.time: %v", err)
	}

	loc, _ := time.LoadLocation("Asia/Shanghai")
	day := time.Now().In(loc).Format("2006-01-02")

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "root_suggest_ch@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage/channels/suggest?q=c-&start="+day+"&end="+day, nil)
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin usage channels suggest status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal channels suggest: %v", err)
	}
	if !got.Success {
		t.Fatalf("channels suggest expected success, got message=%q", got.Message)
	}
	if len(got.Data) != 1 {
		t.Fatalf("expected 1 channel suggest, got %d", len(got.Data))
	}
	if got.Data[0].ID != chIn {
		t.Fatalf("expected channel id=%d, got %d", chIn, got.Data[0].ID)
	}
}

func TestAdminUsageSuggest_Models_RangeScoped(t *testing.T) {
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
	rootID, err := st.CreateUser(ctx, "root_suggest_model@example.com", "rootsuggestmodel", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser(root): %v", err)
	}
	tokenID, _, err := st.CreateUserToken(ctx, rootID, nil, "tok_suggest_model")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}
	ch1, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "m1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(ch1): %v", err)
	}

	createEvent := func(requestID string, model string) int64 {
		usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        requestID,
			UserID:           rootID,
			TokenID:          tokenID,
			Model:            &model,
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("ReserveUsage(%s): %v", requestID, err)
		}
		chRef := ch1
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID:      usageID,
			UpstreamChannelID: &chRef,
			CommittedUSD:      decimal.Zero,
		}); err != nil {
			t.Fatalf("CommitUsage(%s): %v", requestID, err)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID:      usageID,
			Endpoint:          "/v1/responses",
			Method:            "POST",
			StatusCode:        200,
			UpstreamChannelID: &chRef,
		}); err != nil {
			t.Fatalf("FinalizeUsageEvent(%s): %v", requestID, err)
		}
		return usageID
	}

	_ = createEvent("req_suggest_model_in", "gpt-5.2")
	outID := createEvent("req_suggest_model_out", "gpt-4.1")
	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	oldTimeStr := oldTime.Format("2006-01-02 15:04:05")
	if _, err := db.ExecContext(ctx, `UPDATE usage_events SET time=? WHERE id=?`, oldTimeStr, outID); err != nil {
		t.Fatalf("update usage_events.time: %v", err)
	}

	loc, _ := time.LoadLocation("Asia/Shanghai")
	day := time.Now().In(loc).Format("2006-01-02")

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "root_suggest_model@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage/models/suggest?q=gpt&start="+day+"&end="+day, nil)
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin usage models suggest status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    []struct {
			Model string `json:"model"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal models suggest: %v", err)
	}
	if !got.Success {
		t.Fatalf("models suggest expected success, got message=%q", got.Message)
	}
	if len(got.Data) != 1 {
		t.Fatalf("expected 1 model suggest, got %d", len(got.Data))
	}
	if got.Data[0].Model != "gpt-5.2" {
		t.Fatalf("expected model=gpt-5.2, got %q", got.Data[0].Model)
	}
}

func TestAdminUsagePage_CodexOAuthKeepsModelAndSetsAccount(t *testing.T) {
	st, closeDB := newTestSQLiteStore(t)
	defer closeDB()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userID, err := st.CreateUser(ctx, "root@example.com", "root", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_admin_usage_model")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	openAIChannelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "channel-openai", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(openai): %v", err)
	}
	codexChannelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeCodexOAuth, "channel-codex", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(codex): %v", err)
	}
	codexEndpointID, err := st.CreateUpstreamEndpoint(ctx, codexChannelID, "https://codex.example.com/v1", 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	codexAccountEmail := "team001@example.com"
	codexAccountID, err := st.CreateCodexOAuthAccount(ctx, codexEndpointID, "acct_team_001", &codexAccountEmail, "at", "rt", nil, nil)
	if err != nil {
		t.Fatalf("CreateCodexOAuthAccount: %v", err)
	}

	codexModel := "gpt-5-codex"
	codexUsageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_codex_event_1",
		UserID:           userID,
		TokenID:          tokenID,
		Model:            &codexModel,
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage(codex): %v", err)
	}
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID:      codexUsageID,
		UpstreamChannelID: &codexChannelID,
		CommittedUSD:      decimal.RequireFromString("1"),
	}); err != nil {
		t.Fatalf("CommitUsage(codex): %v", err)
	}
	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID:      codexUsageID,
		Endpoint:          "/v1/responses",
		Method:            "POST",
		StatusCode:        200,
		LatencyMS:         900,
		UpstreamChannelID: &codexChannelID,
		UpstreamCredID:    &codexAccountID,
	}); err != nil {
		t.Fatalf("FinalizeUsageEvent(codex): %v", err)
	}

	openAIModel := "gpt-4.1-mini"
	openAIUsageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_openai_event_1",
		UserID:           userID,
		TokenID:          tokenID,
		Model:            &openAIModel,
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage(openai): %v", err)
	}
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID:      openAIUsageID,
		UpstreamChannelID: &openAIChannelID,
		CommittedUSD:      decimal.RequireFromString("1"),
	}); err != nil {
		t.Fatalf("CommitUsage(openai): %v", err)
	}
	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID:      openAIUsageID,
		Endpoint:          "/v1/chat/completions",
		Method:            "POST",
		StatusCode:        200,
		LatencyMS:         500,
		UpstreamChannelID: &openAIChannelID,
	}); err != nil {
		t.Fatalf("FinalizeUsageEvent(openai): %v", err)
	}

	engine, cookieName := newTestEngine(t, st)
	sessionCookie := loginCookie(t, engine, cookieName, "root@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/admin/usage?limit=100", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin usage status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Events []struct {
				RequestID string `json:"request_id"`
				Model     string `json:"model"`
				Account   string `json:"account"`
			} `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal admin usage: %v", err)
	}
	if !got.Success {
		t.Fatalf("admin usage expected success, got message=%q", got.Message)
	}

	var (
		codexFound      bool
		codexModelGot   string
		codexAccountGot string
		openAIFound     bool
		openAIModelGot  string
		openAIAccGot    string
	)
	for i := range got.Data.Events {
		ev := got.Data.Events[i]
		switch ev.RequestID {
		case "req_codex_event_1":
			codexFound = true
			codexModelGot = ev.Model
			codexAccountGot = ev.Account
		case "req_openai_event_1":
			openAIFound = true
			openAIModelGot = ev.Model
			openAIAccGot = ev.Account
		}
	}
	if !codexFound {
		t.Fatalf("expected codex event in response")
	}
	if codexModelGot != codexModel {
		t.Fatalf("expected codex event model=%q, got %q", codexModel, codexModelGot)
	}
	if codexAccountGot != codexAccountEmail {
		t.Fatalf("expected codex event account=%q, got %q", codexAccountEmail, codexAccountGot)
	}
	if !openAIFound {
		t.Fatalf("expected openai event in response")
	}
	if openAIModelGot != openAIModel {
		t.Fatalf("expected openai event model=%q, got %q", openAIModel, openAIModelGot)
	}
	if openAIAccGot != "-" {
		t.Fatalf("expected openai event account='-', got %q", openAIAccGot)
	}
}
