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
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/server"
	"realms/internal/store"
	"realms/internal/version"
)

func TestCodexOAuth_MultiAccount_UsageLimitMarksBalanceAndFailover_E2E(t *testing.T) {
	const model = "gpt-5.2"

	var calls atomic.Int64
	var firstAccountID atomic.Value // string
	var secondAccountID atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/backend-api/codex/v1/responses" {
			http.NotFound(w, r)
			return
		}
		_ = r.Body.Close()

		acc := strings.TrimSpace(r.Header.Get("Chatgpt-Account-Id"))
		n := calls.Add(1)
		if n == 1 {
			firstAccountID.Store(acc)
		} else if n == 2 {
			secondAccountID.Store(acc)
		}

		if acc == "acc_exhausted" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message":           "The usage limit has been reached",
					"type":              "usage_limit_reached",
					"code":              "usage_limit_reached",
					"resets_in_seconds": 120,
				},
			})
			return
		}

		resp := map[string]any{
			"id":     "resp_ok",
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
				"input_tokens":  10,
				"output_tokens": 1,
				"total_tokens":  11,
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

	const userGroup = "ug_codex"
	const routeGroup = "rg_codex"
	if _, err := st.CreateChannelGroup(ctx, routeGroup, nil, 1, store.DefaultGroupPriceMultiplier); err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}
	if err := st.CreateMainGroup(ctx, userGroup, nil, 1); err != nil {
		t.Fatalf("CreateMainGroup: %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, userGroup, []string{routeGroup}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeCodexOAuth, "ci-codex", routeGroup, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	epID, err := st.CreateUpstreamEndpoint(ctx, channelID, strings.TrimRight(strings.TrimSpace(upstream.URL), "/")+"/backend-api/codex", 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}

	// 确保 exhausted 账号优先被选中：后创建的 account ID 更大（排序优先）。
	okAccountID, err := st.CreateCodexOAuthAccount(ctx, epID, "acc_ok", strPtr("ok@example.com"), "at_ok", "rt_ok", nil, timePtr(time.Now().Add(1*time.Hour)))
	if err != nil {
		t.Fatalf("CreateCodexOAuthAccount(ok): %v", err)
	}
	exhaustedAccountID, err := st.CreateCodexOAuthAccount(ctx, epID, "acc_exhausted", strPtr("ex@example.com"), "at_ex", "rt_ex", nil, timePtr(time.Now().Add(1*time.Hour)))
	if err != nil {
		t.Fatalf("CreateCodexOAuthAccount(exhausted): %v", err)
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

	userID, err := st.CreateUser(ctx, "ci-codex-user@example.com", "cicodex", []byte("x"), store.UserRoleUser)
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
	if err := st.ReplaceTokenChannelGroups(ctx, tokenID, []string{routeGroup}); err != nil {
		t.Fatalf("ReplaceTokenChannelGroups: %v", err)
	}

	// e2e 测试应当与外部环境变量解耦：清空可能影响 Load() 的配置项。
	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")

	appCfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
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

	if calls.Load() < 2 {
		t.Fatalf("expected >=2 upstream calls, got=%d", calls.Load())
	}
	if got, _ := firstAccountID.Load().(string); got != "acc_exhausted" {
		t.Fatalf("expected first upstream call to use exhausted account, got=%q", got)
	}
	if got, _ := secondAccountID.Load().(string); got != "acc_ok" {
		t.Fatalf("expected second upstream call to use ok account, got=%q", got)
	}

	accs, err := st.ListCodexOAuthAccountsByEndpoint(ctx, epID)
	if err != nil {
		t.Fatalf("ListCodexOAuthAccountsByEndpoint: %v", err)
	}
	var exhausted store.CodexOAuthAccount
	found := false
	for _, a := range accs {
		if a.ID == exhaustedAccountID {
			exhausted = a
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("exhausted account not found (id=%d)", exhaustedAccountID)
	}
	if exhausted.Status != 1 {
		t.Fatalf("expected exhausted account status=1, got=%d", exhausted.Status)
	}
	if exhausted.CooldownUntil == nil || !exhausted.CooldownUntil.After(time.Now()) {
		t.Fatalf("expected exhausted account cooldown_until to be set and in future, got=%v", exhausted.CooldownUntil)
	}
	if exhausted.QuotaError == nil || strings.TrimSpace(*exhausted.QuotaError) != "余额用尽" {
		got := "<nil>"
		if exhausted.QuotaError != nil {
			got = *exhausted.QuotaError
		}
		t.Fatalf("expected exhausted account quota_error=余额用尽, got=%q", got)
	}

	// sanity: ok account should remain enabled.
	accs2, err := st.ListCodexOAuthAccountsByEndpoint(ctx, epID)
	if err != nil {
		t.Fatalf("ListCodexOAuthAccountsByEndpoint(2): %v", err)
	}
	for _, a := range accs2 {
		if a.ID != okAccountID {
			continue
		}
		if a.Status != 1 {
			t.Fatalf("expected ok account status=1, got=%d", a.Status)
		}
	}
}

func TestCodexOAuth_MultiAccount_InvalidTokenDisablesAndFailover_E2E(t *testing.T) {
	const model = "gpt-5.2"

	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/backend-api/codex/v1/responses" {
			http.NotFound(w, r)
			return
		}
		_ = r.Body.Close()
		_ = calls.Add(1)

		acc := strings.TrimSpace(r.Header.Get("Chatgpt-Account-Id"))
		if acc == "acc_invalid" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": "invalid token",
					"type":    "invalid_token",
					"code":    "invalid_token",
				},
			})
			return
		}

		resp := map[string]any{
			"id":     "resp_ok",
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
				"input_tokens":  1,
				"output_tokens": 1,
				"total_tokens":  2,
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

	const userGroup = "ug_codex2"
	const routeGroup = "rg_codex2"
	if _, err := st.CreateChannelGroup(ctx, routeGroup, nil, 1, store.DefaultGroupPriceMultiplier); err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}
	if err := st.CreateMainGroup(ctx, userGroup, nil, 1); err != nil {
		t.Fatalf("CreateMainGroup: %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, userGroup, []string{routeGroup}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeCodexOAuth, "ci-codex", routeGroup, 0, true, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	epID, err := st.CreateUpstreamEndpoint(ctx, channelID, strings.TrimRight(strings.TrimSpace(upstream.URL), "/")+"/backend-api/codex", 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}

	// invalid 账号优先：后创建 ID 更大。
	okAccountID, err := st.CreateCodexOAuthAccount(ctx, epID, "acc_ok", strPtr("ok@example.com"), "at_ok", "rt_ok", nil, timePtr(time.Now().Add(1*time.Hour)))
	if err != nil {
		t.Fatalf("CreateCodexOAuthAccount(ok): %v", err)
	}
	invalidAccountID, err := st.CreateCodexOAuthAccount(ctx, epID, "acc_invalid", strPtr("inv@example.com"), "at_inv", "rt_inv", nil, timePtr(time.Now().Add(1*time.Hour)))
	if err != nil {
		t.Fatalf("CreateCodexOAuthAccount(invalid): %v", err)
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

	userID, err := st.CreateUser(ctx, "ci-codex-user@example.com", "cicodex", []byte("x"), store.UserRoleUser)
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
	if err := st.ReplaceTokenChannelGroups(ctx, tokenID, []string{routeGroup}); err != nil {
		t.Fatalf("ReplaceTokenChannelGroups: %v", err)
	}

	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")

	appCfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
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
	if calls.Load() < 2 {
		t.Fatalf("expected >=2 upstream calls, got=%d", calls.Load())
	}

	accs, err := st.ListCodexOAuthAccountsByEndpoint(ctx, epID)
	if err != nil {
		t.Fatalf("ListCodexOAuthAccountsByEndpoint: %v", err)
	}
	var invalid store.CodexOAuthAccount
	found := false
	for _, a := range accs {
		if a.ID == invalidAccountID {
			invalid = a
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("invalid account not found (id=%d)", invalidAccountID)
	}
	if invalid.Status != 0 {
		t.Fatalf("expected invalid account to be disabled (status=0), got=%d", invalid.Status)
	}

	// sanity: ok account should remain enabled.
	for _, a := range accs {
		if a.ID != okAccountID {
			continue
		}
		if a.Status != 1 {
			t.Fatalf("expected ok account status=1, got=%d", a.Status)
		}
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
