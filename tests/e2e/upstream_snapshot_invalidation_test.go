package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
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

func TestUpstreamSnapshotCache_InvalidatedByAdminWrites_E2E(t *testing.T) {
	var (
		mu    sync.Mutex
		auths []string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		_ = r.Body.Close()

		mu.Lock()
		auths = append(auths, strings.TrimSpace(r.Header.Get("Authorization")))
		mu.Unlock()

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

	channelB, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ci-upstream-b", routeGroup, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(B): %v", err)
	}
	epB, err := st.CreateUpstreamEndpoint(ctx, channelB, strings.TrimRight(strings.TrimSpace(upstream.URL), "/"), 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint(B): %v", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, epB, strPtr("b"), "sk-b"); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential(B): %v", err)
	}

	channelA, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ci-upstream-a", routeGroup, 10, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(A): %v", err)
	}
	epA, err := st.CreateUpstreamEndpoint(ctx, channelA, strings.TrimRight(strings.TrimSpace(upstream.URL), "/"), 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint(A): %v", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, epA, strPtr("a"), "sk-a"); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential(A): %v", err)
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
	for _, chID := range []int64{channelA, channelB} {
		if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
			ChannelID:     chID,
			PublicID:      model,
			UpstreamModel: model,
			Status:        1,
		}); err != nil {
			t.Fatalf("CreateChannelModel(channel=%d): %v", chID, err)
		}
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

	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	rootID, err := st.CreateUser(ctx, "root@example.com", "root", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateRoot: %v", err)
	}

	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")
	t.Setenv("REALMS_UPSTREAM_SNAPSHOT_TTL_MILLIS", "60000")

	appCfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	appCfg.Env = "dev"
	appCfg.DB.Driver = "sqlite"
	appCfg.DB.DSN = ""
	appCfg.DB.SQLitePath = dbPath
	appCfg.Security.AllowOpenRegistration = false
	appCfg.SelfMode.Enable = false

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

	// login as root (for admin channel writes)
	loginBody, _ := json.Marshal(map[string]any{"login": "root@example.com", "password": "password123"})
	req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(ts.URL, "/")+"/api/user/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	loginResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/user/login: %v", err)
	}
	_ = loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login unexpected status: %s", loginResp.Status)
	}
	sessionCookie := ""
	for _, c := range loginResp.Cookies() {
		if c.Name == server.SessionCookieName {
			sessionCookie = c.String()
			break
		}
	}
	if sessionCookie == "" {
		t.Fatalf("expected session cookie %q", server.SessionCookieName)
	}

	// Warm upstream snapshot cache with a data-plane call: should pick channel A (higher priority).
	reqBody := []byte(`{"model":"gpt-5.2","input":"hello","stream":false}`)
	req, _ = http.NewRequest(http.MethodPost, strings.TrimRight(ts.URL, "/")+"/v1/responses", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/responses(1): %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %s", resp.Status)
	}

	// Disable channel A via admin API; handler should invalidate upstream snapshot immediately.
	disableBody, _ := json.Marshal(map[string]any{"id": channelA, "status": 0})
	req, _ = http.NewRequest(http.MethodPut, strings.TrimRight(ts.URL, "/")+"/api/channel", bytes.NewReader(disableBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Cookie", sessionCookie)
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	disableResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/channel: %v", err)
	}
	defer disableResp.Body.Close()
	rawDisable, _ := io.ReadAll(io.LimitReader(disableResp.Body, 1<<20))
	if disableResp.StatusCode != http.StatusOK {
		t.Fatalf("disable unexpected status: %s body=%s", disableResp.Status, string(rawDisable))
	}
	var disableOut struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(rawDisable, &disableOut)
	if !disableOut.Success {
		t.Fatalf("disable failed: %s", disableOut.Message)
	}

	// Next data-plane call should skip channel A even though TTL is 60s.
	req, _ = http.NewRequest(http.MethodPost, strings.TrimRight(ts.URL, "/")+"/v1/responses", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/responses(2): %v", err)
	}
	defer resp2.Body.Close()
	_, _ = io.ReadAll(io.LimitReader(resp2.Body, 1<<20))
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %s", resp2.Status)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(auths) != 2 {
		t.Fatalf("expected 2 upstream calls, got %d auths=%v", len(auths), auths)
	}
	if !strings.Contains(strings.ToLower(auths[0]), "sk-a") {
		t.Fatalf("expected first upstream auth to contain sk-a, got %q", auths[0])
	}
	if !strings.Contains(strings.ToLower(auths[1]), "sk-b") {
		t.Fatalf("expected second upstream auth to contain sk-b, got %q", auths[1])
	}
}
