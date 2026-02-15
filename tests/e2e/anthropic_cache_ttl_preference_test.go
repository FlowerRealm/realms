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

	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/server"
	"realms/internal/store"
	"realms/internal/version"
)

func anyString(v any) string {
	s, _ := v.(string)
	return s
}

func TestAnthropic_CacheTTLPreference_E2E(t *testing.T) {
	var seenBeta string
	var seenTTLs []string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		seenBeta = r.Header.Get("anthropic-beta")

		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		_ = r.Body.Close()
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)

		msgs, _ := payload["messages"].([]any)
		if len(msgs) > 0 {
			msg, _ := msgs[0].(map[string]any)
			content, _ := msg["content"].([]any)
			for _, blkAny := range content {
				blk, _ := blkAny.(map[string]any)
				cc, _ := blk["cache_control"].(map[string]any)
				if strings.TrimSpace(strings.ToLower(anyString(cc["type"]))) != "ephemeral" {
					continue
				}
				seenTTLs = append(seenTTLs, strings.TrimSpace(anyString(cc["ttl"])))
			}
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "msg_test_1",
			"type":    "message",
			"role":    "assistant",
			"content": []any{map[string]any{"type": "text", "text": "OK"}},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"
	db, err := store.OpenSQLite(dbPath)
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

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeAnthropic, "anthropic-ci", routeGroup, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	if err := st.UpdateUpstreamChannelNewAPISetting(ctx, channelID, store.UpstreamChannelSetting{CacheTTLPreference: "1h"}); err != nil {
		t.Fatalf("UpdateUpstreamChannelNewAPISetting: %v", err)
	}
	epID, err := st.CreateUpstreamEndpoint(ctx, channelID, strings.TrimRight(upstream.URL, "/"), 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	if _, _, err := st.CreateAnthropicCredential(ctx, epID, strPtr("ci"), "sk-anthropic-test"); err != nil {
		t.Fatalf("CreateAnthropicCredential: %v", err)
	}

	const model = "claude-sonnet"
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            model,
		GroupName:           routeGroup,
		OwnedBy:             strPtr("upstream"),
		InputUSDPer1M:       decimal.Zero,
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
	if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.NewFromInt(1)); err != nil {
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
	appCfg.DB.Driver = "sqlite"
	appCfg.DB.DSN = ""
	appCfg.DB.SQLitePath = dbPath
	appCfg.Security.AllowOpenRegistration = false

	app, err := server.NewApp(server.AppOptions{
		Config:  appCfg,
		DB:      db,
		Version: version.Info(),
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	reqBody := []byte(`{
  "model":"claude-sonnet",
  "max_tokens":16,
  "messages":[{"role":"user","content":[
    {"type":"text","text":"a","cache_control":{"type":"ephemeral"}},
    {"type":"text","text":"b","cache_control":{"type":"ephemeral","ttl":"5m"}},
    {"type":"text","text":"c","cache_control":{"type":"persistent"}}
  ]}]
}`)

	req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(ts.URL, "/")+"/v1/messages", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/messages: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %s", resp.Status)
	}

	if !strings.Contains(strings.ToLower(seenBeta), "extended-cache-ttl-2025-04-11") {
		t.Fatalf("expected anthropic-beta to include extended ttl flag, got %q", seenBeta)
	}
	if len(seenTTLs) != 2 || seenTTLs[0] != "1h" || seenTTLs[1] != "1h" {
		t.Fatalf("expected rewritten ephemeral cache_control.ttl=[1h 1h], got %#v", seenTTLs)
	}
}
