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
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/server"
	"realms/internal/store"
	"realms/internal/version"
)

func TestUpstream429_FailoverToOtherCredential_E2E(t *testing.T) {
	var (
		mu     sync.Mutex
		auths  []string
		calls  int
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		_ = r.Body.Close()

		mu.Lock()
		calls++
		auths = append(auths, strings.TrimSpace(r.Header.Get("Authorization")))
		thisCall := calls
		mu.Unlock()

		// First selected credential is the newest one (higher id). Force a 429 so Realms cools it down and failovers.
		if strings.Contains(strings.ToLower(r.Header.Get("Authorization")), "sk-b") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limit","type":"rate_limit"}}`))
			return
		}

		resp := map[string]any{
			"id":     "resp_ok",
			"object": "response",
			"model":  "gpt-5.2",
			"output": []any{map[string]any{
				"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": "OK"}},
			}},
			"status": "completed",
			"usage":  map[string]any{"input_tokens": 1, "output_tokens": 1},
			"meta":   map[string]any{"call": thisCall},
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
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

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ci-upstream", routeGroup, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	epID, err := st.CreateUpstreamEndpoint(ctx, channelID, strings.TrimRight(strings.TrimSpace(upstream.URL), "/"), 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	// older credential (selected after failover)
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, epID, strPtr("a"), "sk-a"); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential(a): %v", err)
	}
	// newer credential (selected first, returns 429)
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, epID, strPtr("b"), "sk-b"); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential(b): %v", err)
	}

	const model = "gpt-5.2"
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

	reqBody := []byte(`{"model":"gpt-5.2","input":"hello","stream":false}`)
	req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(ts.URL, "/")+"/v1/responses", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/responses: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %s", resp.Status)
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected 2 upstream calls (429 then success), got %d auths=%v", calls, auths)
	}
	if len(auths) != 2 || !strings.Contains(strings.ToLower(auths[0]), "sk-b") || !strings.Contains(strings.ToLower(auths[1]), "sk-a") {
		t.Fatalf("expected auth sequence [sk-b, sk-a], got %v", auths)
	}
}

