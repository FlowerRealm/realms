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
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/server"
	"realms/internal/store"
	"realms/internal/version"
)

func TestMultiInstance_UpstreamSnapshotInvalidation_DBVersionPolling_E2E(t *testing.T) {
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

	dbA, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite(A): %v", err)
	}
	defer dbA.Close()
	if err := store.EnsureSQLiteSchema(dbA); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}

	st := store.New(dbA)
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
	t.Setenv("REALMS_CACHE_INVALIDATION_POLL_MILLIS", "100")

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

	appA, err := server.NewApp(server.AppOptions{Config: appCfg, DB: dbA, Version: version.Info()})
	if err != nil {
		t.Fatalf("NewApp(A): %v", err)
	}
	tsA := httptest.NewServer(appA.Handler())
	defer tsA.Close()

	dbB, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite(B): %v", err)
	}
	defer dbB.Close()
	appB, err := server.NewApp(server.AppOptions{Config: appCfg, DB: dbB, Version: version.Info()})
	if err != nil {
		t.Fatalf("NewApp(B): %v", err)
	}
	tsB := httptest.NewServer(appB.Handler())
	defer tsB.Close()

	// login as root on instance A
	loginBody, _ := json.Marshal(map[string]any{"login": "root@example.com", "password": "password123"})
	req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(tsA.URL, "/")+"/api/user/login", bytes.NewReader(loginBody))
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

	reqBody := []byte(`{"model":"gpt-5.2","input":"hello","stream":false}`)
	// Warm snapshot on instance B: should pick channel A.
	req, _ = http.NewRequest(http.MethodPost, strings.TrimRight(tsB.URL, "/")+"/v1/responses", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/responses(B,1): %v", err)
	}
	_, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %s", resp.Status)
	}

	// Disable channel A via admin API on instance A; bumps DB invalidation version.
	disableBody, _ := json.Marshal(map[string]any{"id": channelA, "status": 0})
	req, _ = http.NewRequest(http.MethodPut, strings.TrimRight(tsA.URL, "/")+"/api/channel", bytes.NewReader(disableBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Cookie", sessionCookie)
	req.Header.Set("Realms-User", strconv.FormatInt(rootID, 10))
	disableResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/channel: %v", err)
	}
	rawDisable, _ := io.ReadAll(io.LimitReader(disableResp.Body, 1<<20))
	_ = disableResp.Body.Close()
	if disableResp.StatusCode != http.StatusOK {
		t.Fatalf("disable unexpected status: %s body=%s", disableResp.Status, string(rawDisable))
	}

	// Instance B should switch within ~poll interval + jitter.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req, _ = http.NewRequest(http.MethodPost, strings.TrimRight(tsB.URL, "/")+"/v1/responses", bytes.NewReader(reqBody))
		req.Header.Set("Authorization", "Bearer "+rawToken)
		req.Header.Set("Content-Type", "application/json")
		resp2, err := http.DefaultClient.Do(req)
		if err == nil {
			_, _ = io.ReadAll(io.LimitReader(resp2.Body, 1<<20))
			_ = resp2.Body.Close()
		}

		mu.Lock()
		n := len(auths)
		last := ""
		if n > 0 {
			last = auths[n-1]
		}
		mu.Unlock()
		if strings.Contains(strings.ToLower(last), "sk-b") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("expected instance B to switch to sk-b within 2s, auths=%v", auths)
}

func TestMultiInstance_TokenRevokeInvalidation_DBVersionPolling_E2E(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		_ = r.Body.Close()
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

	dbA, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite(A): %v", err)
	}
	defer dbA.Close()
	if err := store.EnsureSQLiteSchema(dbA); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}

	st := store.New(dbA)
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

	ch, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ci-upstream", routeGroup, 10, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	ep, err := st.CreateUpstreamEndpoint(ctx, ch, strings.TrimRight(strings.TrimSpace(upstream.URL), "/"), 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, ep, strPtr("a"), "sk-a"); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential: %v", err)
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
		ChannelID:     ch,
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
	t.Setenv("REALMS_CACHE_INVALIDATION_POLL_MILLIS", "100")
	// Make TokenAuth cache sticky so revocation relies on invalidation, not TTL.
	t.Setenv("REALMS_TOKEN_AUTH_CACHE_TTL_MILLIS", "60000")

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

	appA, err := server.NewApp(server.AppOptions{Config: appCfg, DB: dbA, Version: version.Info()})
	if err != nil {
		t.Fatalf("NewApp(A): %v", err)
	}
	tsA := httptest.NewServer(appA.Handler())
	defer tsA.Close()

	dbB, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite(B): %v", err)
	}
	defer dbB.Close()
	appB, err := server.NewApp(server.AppOptions{Config: appCfg, DB: dbB, Version: version.Info()})
	if err != nil {
		t.Fatalf("NewApp(B): %v", err)
	}
	tsB := httptest.NewServer(appB.Handler())
	defer tsB.Close()

	reqBody := []byte(`{"model":"gpt-5.2","input":"hello","stream":false}`)
	// Warm TokenAuth cache on instance B: should authorize.
	req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(tsB.URL, "/")+"/v1/responses", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/responses(B,1): %v", err)
	}
	_, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %s", resp.Status)
	}

	// Revoke token (bumps DB invalidation).
	if err := st.RevokeUserToken(ctx, userID, tokenID); err != nil {
		t.Fatalf("RevokeUserToken: %v", err)
	}

	// Instance B should reject within <=2s even though cache TTL is 60s.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(tsB.URL, "/")+"/v1/responses", bytes.NewReader(reqBody))
		req.Header.Set("Authorization", "Bearer "+rawToken)
		req.Header.Set("Content-Type", "application/json")
		resp2, err := http.DefaultClient.Do(req)
		if err == nil {
			_, _ = io.ReadAll(io.LimitReader(resp2.Body, 1<<20))
			_ = resp2.Body.Close()
			if resp2.StatusCode == http.StatusUnauthorized || resp2.StatusCode == http.StatusForbidden {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected instance B to reject revoked token within 2s")
}
