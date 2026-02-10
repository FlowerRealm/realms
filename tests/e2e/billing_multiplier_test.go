package e2e_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/server"
	"realms/internal/store"
	"realms/internal/version"
)

func TestBilling_GroupMultiplierStacking_Payg_E2E(t *testing.T) {
	const model = "gpt-5.2"

	upstream := newMultiplierUpstreamServer(t, model)
	st, db, dbPath := newMultiplierSQLiteStore(t)
	ctx := context.Background()

	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}
	if _, err := st.CreateChannelGroup(ctx, "staff", nil, 1, decimal.RequireFromString("2"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(staff): %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, store.DefaultGroupName, []string{"vip", "staff"}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ci-upstream", "staff", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	epID, err := st.CreateUpstreamEndpoint(ctx, channelID, strings.TrimRight(strings.TrimSpace(upstream.URL), "/")+"/v1", 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, epID, strPtr("ci"), "sk-upstream-test"); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential: %v", err)
	}

	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            model,
		GroupName:           store.DefaultGroupName,
		OwnedBy:             strPtr("upstream"),
		InputUSDPer1M:       decimal.RequireFromString("1"),
		OutputUSDPer1M:      decimal.Zero,
		CacheInputUSDPer1M:  decimal.Zero,
		CacheOutputUSDPer1M: decimal.Zero,
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     channelID,
		PublicID:      model,
		UpstreamModel: model,
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel: %v", err)
	}

	if err := st.UpsertDecimalAppSetting(ctx, store.SettingBillingPayAsYouGoPriceMultiplier, decimal.RequireFromString("1.5")); err != nil {
		t.Fatalf("UpsertDecimalAppSetting(paygo multiplier): %v", err)
	}

	userID, err := st.CreateUser(ctx, "mul-payg@example.com", "mulpayg", []byte("x"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.RequireFromString("10")); err != nil {
		t.Fatalf("AddUserBalanceUSD: %v", err)
	}
	rawToken, err := auth.NewRandomToken("sk_", 32)
	if err != nil {
		t.Fatalf("NewRandomToken: %v", err)
	}
	tokenID, _, err := st.CreateUserToken(ctx, userID, strPtr("ci-token"), rawToken)
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}
	if err := st.ReplaceTokenGroups(ctx, tokenID, []string{"staff"}); err != nil {
		t.Fatalf("ReplaceTokenGroups: %v", err)
	}

	ts := newMultiplierAppServer(t, db, dbPath, true, false)

	callResponsesOnce(t, ts.URL, rawToken, model)

	events := waitUsageEventsByUser(t, st, ctx, userID, 1)
	ev := events[0]
	if ev.State != store.UsageStateCommitted {
		t.Fatalf("usage_event state mismatch: got=%q want=%q (id=%d)", ev.State, store.UsageStateCommitted, ev.ID)
	}
	wantCommitted := decimal.RequireFromString("3")
	if !ev.CommittedUSD.Equal(wantCommitted) {
		t.Fatalf("committed_usd mismatch: got=%s want=%s (id=%d)", ev.CommittedUSD.String(), wantCommitted.String(), ev.ID)
	}
	gotBal, err := st.GetUserBalanceUSD(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserBalanceUSD: %v", err)
	}
	wantBalance := decimal.RequireFromString("7")
	if !gotBal.Equal(wantBalance) {
		t.Fatalf("balance mismatch: got=%s want=%s", gotBal.String(), wantBalance.String())
	}
}

func TestBilling_GroupMultiplierStacking_Subscription_E2E(t *testing.T) {
	const model = "gpt-5.2"

	upstream := newMultiplierUpstreamServer(t, model)
	st, db, dbPath := newMultiplierSQLiteStore(t)
	ctx := context.Background()

	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}
	if _, err := st.CreateChannelGroup(ctx, "staff", nil, 1, decimal.RequireFromString("2"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(staff): %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, store.DefaultGroupName, []string{"vip", "staff"}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ci-upstream", "staff", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	epID, err := st.CreateUpstreamEndpoint(ctx, channelID, strings.TrimRight(strings.TrimSpace(upstream.URL), "/")+"/v1", 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, epID, strPtr("ci"), "sk-upstream-test"); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential: %v", err)
	}

	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            model,
		GroupName:           store.DefaultGroupName,
		OwnedBy:             strPtr("upstream"),
		InputUSDPer1M:       decimal.RequireFromString("1"),
		OutputUSDPer1M:      decimal.Zero,
		CacheInputUSDPer1M:  decimal.Zero,
		CacheOutputUSDPer1M: decimal.Zero,
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     channelID,
		PublicID:      model,
		UpstreamModel: model,
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel: %v", err)
	}

	userID, err := st.CreateUser(ctx, "mul-sub@example.com", "mulsub", []byte("x"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	// 套餐的 group_name 只用于“可购买范围”，计费倍率取订阅倍率 × 请求最终成功分组倍率（不叠加其他分组）。

	planID, err := st.CreateSubscriptionPlan(ctx, store.SubscriptionPlanCreate{
		Code:         "ci-vip-plan",
		Name:         "CI VIP Plan",
		GroupName:    "vip",
		PriceCNY:     decimal.Zero,
		Limit5HUSD:   decimal.RequireFromString("100"),
		Limit1DUSD:   decimal.RequireFromString("100"),
		Limit7DUSD:   decimal.RequireFromString("100"),
		Limit30DUSD:  decimal.RequireFromString("100"),
		DurationDays: 30,
		Status:       1,
	})
	if err != nil {
		t.Fatalf("CreateSubscriptionPlan: %v", err)
	}
	us, _, err := st.PurchaseSubscriptionByPlanID(ctx, userID, planID, time.Now())
	if err != nil {
		t.Fatalf("PurchaseSubscriptionByPlanID: %v", err)
	}

	rawToken, err := auth.NewRandomToken("sk_", 32)
	if err != nil {
		t.Fatalf("NewRandomToken: %v", err)
	}
	tokenID, _, err := st.CreateUserToken(ctx, userID, strPtr("ci-token"), rawToken)
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}
	if err := st.ReplaceTokenGroups(ctx, tokenID, []string{"staff"}); err != nil {
		t.Fatalf("ReplaceTokenGroups: %v", err)
	}

	ts := newMultiplierAppServer(t, db, dbPath, false, false)

	callResponsesOnce(t, ts.URL, rawToken, model)

	events := waitUsageEventsByUser(t, st, ctx, userID, 1)
	ev := events[0]
	if ev.State != store.UsageStateCommitted {
		t.Fatalf("usage_event state mismatch: got=%q want=%q (id=%d)", ev.State, store.UsageStateCommitted, ev.ID)
	}
	if ev.SubscriptionID == nil || *ev.SubscriptionID != us.ID {
		t.Fatalf("subscription_id mismatch: got=%v want=%d", ev.SubscriptionID, us.ID)
	}
	wantCommitted := decimal.RequireFromString("2")
	if !ev.CommittedUSD.Equal(wantCommitted) {
		t.Fatalf("committed_usd mismatch: got=%s want=%s (id=%d)", ev.CommittedUSD.String(), wantCommitted.String(), ev.ID)
	}
}

func newMultiplierSQLiteStore(t *testing.T) (*store.Store, *sql.DB, string) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"
	db, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}
	st := store.New(db)
	st.SetDialect(store.DialectSQLite)
	return st, db, dbPath
}

func newMultiplierUpstreamServer(t *testing.T, model string) *httptest.Server {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		_ = r.Body.Close()

		resp := map[string]any{
			"id":     "resp_multiplier_1",
			"object": "response",
			"model":  model,
			"output": []any{
				map[string]any{
					"type": "message",
					"role": "assistant",
					"content": []any{
						map[string]any{"type": "output_text", "text": "OK"},
					},
				},
			},
			"usage": map[string]any{
				"input_tokens":  1_000_000,
				"output_tokens": 1,
			},
			"status": "completed",
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(upstream.Close)
	return upstream
}

func newMultiplierAppServer(t *testing.T, db *sql.DB, dbPath string, paygEnabled bool, selfMode bool) *httptest.Server {
	t.Helper()

	// e2e 测试应当与外部环境变量解耦：清空可能影响 Load() 的配置项。
	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")

	appCfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	appCfg.Env = "dev"
	appCfg.SelfMode.Enable = selfMode
	appCfg.DB.Driver = "sqlite"
	appCfg.DB.DSN = ""
	appCfg.DB.SQLitePath = dbPath
	appCfg.Security.AllowOpenRegistration = false
	appCfg.Billing.EnablePayAsYouGo = paygEnabled

	app, err := server.NewApp(server.AppOptions{
		Config:  appCfg,
		DB:      db,
		Version: version.Info(),
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func callResponsesOnce(t *testing.T, baseURL string, rawToken string, model string) {
	t.Helper()

	reqBody := []byte(`{"model":"` + model + `","input":"hi","stream":false}`)
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1/responses", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+rawToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", resp.StatusCode, string(bodyBytes))
	}
}
