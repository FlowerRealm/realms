package e2e_test

import (
	"bytes"
	"context"
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

func TestBilling_PaygBalanceDebitsByTokensPricing_E2E(t *testing.T) {
	const (
		model = "gpt-5.2"
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		_ = r.Body.Close()

		resp := map[string]any{
			"id":     "resp_test_1",
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
				"input_tokens":  1000,
				"output_tokens": 1,
				// 回归保障：即使上游返回“金额/费用”字段，本项目也应忽略并仅按 tokens + 本地定价计费。
				"total_cost": 123.45,
				"cost_usd":   123.45,
			},
			"status": "completed",
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(upstream.Close)

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

	ctx := context.Background()

	const userGroup = "ug1"
	const routeGroup = "rg1"
	if _, err := st.CreateChannelGroup(ctx, routeGroup, nil, 1, store.DefaultGroupPriceMultiplier, 5); err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}
	if err := st.CreateMainGroup(ctx, userGroup, nil, 1); err != nil {
		t.Fatalf("CreateMainGroup: %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, userGroup, []string{routeGroup}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ci-upstream", routeGroup, 0, false, false, false, false)
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
		GroupName:           routeGroup,
		OwnedBy:             strPtr("upstream"),
		InputUSDPer1M:       decimal.RequireFromString("10"),
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

	userID, err := st.CreateUser(ctx, "ci-user@example.com", "ciuser", []byte("x"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, userGroup); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}
	if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.RequireFromString("1")); err != nil {
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
	if err := st.ReplaceTokenGroups(ctx, tokenID, []string{routeGroup}); err != nil {
		t.Fatalf("ReplaceTokenGroups: %v", err)
	}

	// e2e 测试应当与外部环境变量解耦：清空可能影响 Load() 的配置项。
	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")

	appCfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	appCfg.Env = "dev"
	appCfg.SelfMode.Enable = false
	appCfg.DB.Driver = "sqlite"
	appCfg.DB.DSN = ""
	appCfg.DB.SQLitePath = dbPath
	appCfg.Security.AllowOpenRegistration = false
	appCfg.Billing.EnablePayAsYouGo = true

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

	reqBody := []byte(`{"model":"` + model + `","input":"hi","stream":false}`)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/responses", bytes.NewReader(reqBody))
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

	events := waitUsageEventsByUser(t, st, ctx, userID, 1)
	ev := events[0]
	if ev.State != store.UsageStateCommitted {
		t.Fatalf("usage_event state mismatch: got=%q want=%q (id=%d)", ev.State, store.UsageStateCommitted, ev.ID)
	}
	if ev.InputTokens == nil || *ev.InputTokens != 1000 {
		t.Fatalf("input_tokens mismatch: got=%v want=%d (id=%d)", ev.InputTokens, 1000, ev.ID)
	}
	if ev.OutputTokens == nil || *ev.OutputTokens != 1 {
		t.Fatalf("output_tokens mismatch: got=%v want=%d (id=%d)", ev.OutputTokens, 1, ev.ID)
	}

	// 计费口径：本地按 tokens + managed_models 定价计算；此用例确保 committed_usd > reserved_usd 时也能补扣差额。
	//
	// reserved_usd：未传 max_output_tokens，默认预留 0.001 USD
	// committed_usd：input_tokens=1000 且 input_usd_per_1m=10 => 0.01 USD
	wantCommitted := decimal.RequireFromString("0.01")
	if !ev.CommittedUSD.Equal(wantCommitted) {
		t.Fatalf("committed_usd mismatch: got=%s want=%s (id=%d)", ev.CommittedUSD.String(), wantCommitted.String(), ev.ID)
	}
	wantBalance := decimal.RequireFromString("0.99")
	gotBal, err := st.GetUserBalanceUSD(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserBalanceUSD: %v", err)
	}
	if !gotBal.Equal(wantBalance) {
		t.Fatalf("balance mismatch: got=%s want=%s", gotBal.String(), wantBalance.String())
	}
}
