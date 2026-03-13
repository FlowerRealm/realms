package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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

// TestCLIChannelTest_E2E validates the full CLI channel test flow through the
// HTTP stack:
//  1. GET /api/channel/test/:id?stream=1 delegates to the CLI runner and returns final JSON
//  2. last_test_at is NOT updated in the database (result isolation)
func TestCLIChannelTest_E2E(t *testing.T) {
	const model = "gpt-test"

	fakeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-test-mini",
		})
	}))
	defer fakeUpstream.Close()

	fakeRunner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req struct {
			CLIType string `json:"cli_type"`
			BaseURL string `json:"base_url"`
			APIKey  string `json:"api_key"`
			Model   string `json:"model"`
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.CLIType == "" || req.APIKey == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         true,
			"latency_ms": 42,
			"ttft_ms":    8,
			"output":     "partial OK",
			"error":      "",
		})
	}))
	defer fakeRunner.Close()

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

	const routeGroup = "rg1"
	if _, err := st.CreateChannelGroup(ctx, routeGroup, nil, 1, store.DefaultGroupPriceMultiplier); err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "cli-test-channel", routeGroup, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	epID, err := st.CreateUpstreamEndpoint(ctx, channelID, fakeUpstream.URL, 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, epID, strPtr("ci"), "sk-test-key"); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential: %v", err)
	}

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

	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	rootUserID, err := st.CreateUser(ctx, "root@example.com", "root", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	chBefore, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID (before): %v", err)
	}

	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")
	t.Setenv("SESSION_SECRET", "e2e-test-secret")

	appCfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	appCfg.Env = "dev"
	appCfg.DB.Driver = "sqlite"
	appCfg.DB.DSN = ""
	appCfg.DB.SQLitePath = dbPath
	appCfg.ChannelTestCLIRunnerURL = fakeRunner.URL

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

	client := &http.Client{}
	sessionCookie := loginAsRoot(t, ts.URL, client, rootUserID)

	t.Run("cli_test_json_delegation", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/channel/test/%d?stream=1", ts.URL, channelID)
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Cookie", sessionCookie)
		req.Header.Set("Realms-User", strconv.FormatInt(rootUserID, 10))
		req.Header.Set("Accept", "text/event-stream")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("test channel request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
		}
		if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
			t.Fatalf("expected json content type, got %q", got)
		}

		var payload struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Data    struct {
				LatencyMS int `json:"latency_ms"`
				Probe     struct {
					OK                 bool   `json:"ok"`
					Source             string `json:"source"`
					Total              int    `json:"total"`
					Success            int    `json:"success"`
					ModelCheckMismatch int    `json:"model_check_mismatch"`
					Results            []struct {
						Model                 string `json:"model"`
						OK                    bool   `json:"ok"`
						SuccessPath           string `json:"success_path"`
						UpstreamResponseModel string `json:"upstream_response_model"`
					} `json:"results"`
				} `json:"probe"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !payload.Success {
			t.Fatalf("expected success payload, got %+v", payload)
		}
		if payload.Data.Probe.Source != "cli_runner" || payload.Data.Probe.Total != 1 || payload.Data.Probe.Success != 1 {
			t.Fatalf("unexpected probe summary: %+v", payload.Data.Probe)
		}
		if payload.Data.Probe.ModelCheckMismatch != 1 {
			t.Fatalf("expected model_check_mismatch=1, got %+v", payload.Data.Probe)
		}
		if len(payload.Data.Probe.Results) != 1 {
			t.Fatalf("expected one result, got %+v", payload.Data.Probe.Results)
		}
		if payload.Data.Probe.Results[0].UpstreamResponseModel != "gpt-test-mini" {
			t.Fatalf("expected upstream_response_model from probe, got %+v", payload.Data.Probe.Results[0])
		}
		if payload.Data.Probe.Results[0].SuccessPath != "/v1/responses" {
			t.Fatalf("expected success_path from probe, got %+v", payload.Data.Probe.Results[0])
		}
	})

	t.Run("result_isolation_no_db_write", func(t *testing.T) {
		chAfter, err := st.GetUpstreamChannelByID(ctx, channelID)
		if err != nil {
			t.Fatalf("GetUpstreamChannelByID (after): %v", err)
		}
		if chBefore.LastTestAt == nil && chAfter.LastTestAt != nil {
			t.Errorf("expected last_test_at to remain nil (not written to DB), got %v", chAfter.LastTestAt)
		}
		if chBefore.LastTestAt != nil && chAfter.LastTestAt != nil && !chBefore.LastTestAt.Equal(*chAfter.LastTestAt) {
			t.Errorf("expected last_test_at unchanged, before=%v after=%v", chBefore.LastTestAt, chAfter.LastTestAt)
		}
	})
}

// TestCLIChannelTest_RealUpstream_E2E validates the CLI channel test flow
// against a real upstream + real CLI runner container.
//
// Required env:
//   - REALMS_CI_ENFORCE_E2E=1
//   - REALMS_CI_CLI_RUNNER_URL (e.g. http://localhost:3100)
//   - REALMS_CI_UPSTREAM_BASE_URL
//   - REALMS_CI_UPSTREAM_API_KEY
//   - REALMS_CI_MODEL
func TestCLIChannelTest_RealUpstream_E2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("REALMS_CI_ENFORCE_E2E")) == "" {
		t.Skip("未设置 REALMS_CI_ENFORCE_E2E，跳过 E2E")
	}

	cliRunnerURL := strings.TrimSpace(os.Getenv("REALMS_CI_CLI_RUNNER_URL"))
	if cliRunnerURL == "" {
		t.Skip("未设置 REALMS_CI_CLI_RUNNER_URL，跳过 CLI runner real upstream E2E")
	}
	upstreamBaseURL := requiredEnvOrSkip(t, "REALMS_CI_UPSTREAM_BASE_URL", "UPSTREAM_BASE_URL")
	upstreamAPIKey := requiredEnvOrSkip(t, "REALMS_CI_UPSTREAM_API_KEY", "UPSTREAM_API_KEY")
	model := requiredEnvOrSkip(t, "REALMS_CI_MODEL", "MODEL")

	{
		hc, err := http.Get(strings.TrimRight(cliRunnerURL, "/") + "/healthz")
		if err != nil {
			t.Fatalf("CLI runner 不可达 (%s/healthz): %v", cliRunnerURL, err)
		}
		defer hc.Body.Close()
		if hc.StatusCode != http.StatusOK {
			t.Fatalf("CLI runner healthz 返回 %d", hc.StatusCode)
		}
		var health struct {
			Status string `json:"status"`
			CLI    struct {
				Codex bool `json:"codex"`
			} `json:"cli"`
		}
		if err := json.NewDecoder(hc.Body).Decode(&health); err != nil {
			t.Fatalf("解析 healthz: %v", err)
		}
		if !health.CLI.Codex {
			t.Fatal("CLI runner 未安装 codex CLI")
		}
	}

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

	const routeGroup = "rg1"
	if _, err := st.CreateChannelGroup(ctx, routeGroup, nil, 1, store.DefaultGroupPriceMultiplier); err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "real-upstream", routeGroup, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	upstreamBaseURL = strings.TrimRight(strings.TrimSpace(upstreamBaseURL), "/")
	epID, err := st.CreateUpstreamEndpoint(ctx, channelID, upstreamBaseURL, 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, epID, strPtr("ci"), strings.TrimSpace(upstreamAPIKey)); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential: %v", err)
	}

	model = strings.TrimSpace(model)
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

	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	rootUserID, err := st.CreateUser(ctx, "root@example.com", "root", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	chBefore, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID (before): %v", err)
	}

	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")
	t.Setenv("SESSION_SECRET", "e2e-test-secret")

	appCfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	appCfg.Env = "dev"
	appCfg.DB.Driver = "sqlite"
	appCfg.DB.DSN = ""
	appCfg.DB.SQLitePath = dbPath
	appCfg.ChannelTestCLIRunnerURL = cliRunnerURL

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

	client := &http.Client{Timeout: 90 * time.Second}
	sessionCookie := loginAsRoot(t, ts.URL, client, rootUserID)

	t.Run("real_cli_test_json", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/channel/test/%d?stream=1", ts.URL, channelID)
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Cookie", sessionCookie)
		req.Header.Set("Realms-User", strconv.FormatInt(rootUserID, 10))
		req.Header.Set("Accept", "text/event-stream")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("test channel request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
		}
		var payload struct {
			Success bool `json:"success"`
			Data    struct {
				Probe struct {
					OK      bool   `json:"ok"`
					Source  string `json:"source"`
					Message string `json:"message"`
					Results []struct {
						OK bool `json:"ok"`
					} `json:"results"`
				} `json:"probe"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !payload.Success || !payload.Data.Probe.OK {
			t.Fatalf("expected successful summary, got %+v", payload)
		}
		if payload.Data.Probe.Source != "cli_runner" {
			t.Fatalf("expected source=cli_runner, got %+v", payload)
		}
		if len(payload.Data.Probe.Results) == 0 || !payload.Data.Probe.Results[0].OK {
			t.Fatalf("expected successful model result, got %+v", payload)
		}
	})

	t.Run("real_result_isolation", func(t *testing.T) {
		chAfter, err := st.GetUpstreamChannelByID(ctx, channelID)
		if err != nil {
			t.Fatalf("GetUpstreamChannelByID (after): %v", err)
		}
		if chBefore.LastTestAt == nil && chAfter.LastTestAt != nil {
			t.Errorf("expected last_test_at to remain nil, got %v", chAfter.LastTestAt)
		}
		if chBefore.LastTestAt != nil && chAfter.LastTestAt != nil && !chBefore.LastTestAt.Equal(*chAfter.LastTestAt) {
			t.Errorf("expected last_test_at unchanged, before=%v after=%v", chBefore.LastTestAt, chAfter.LastTestAt)
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// loginAsRoot performs a POST /api/user/login and returns the session cookie string.
func loginAsRoot(t *testing.T, baseURL string, client *http.Client, rootUserID int64) string {
	t.Helper()

	loginBody, _ := json.Marshal(map[string]any{
		"login":    "root@example.com",
		"password": "password123",
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/user/login", strings.NewReader(string(loginBody)))
	if err != nil {
		t.Fatalf("NewRequest login: %v", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("login status=%d body=%s", resp.StatusCode, string(body))
	}

	var loginResp struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if !loginResp.Success {
		t.Fatal("login was not successful")
	}

	for _, c := range resp.Cookies() {
		if c.Name == "realms_session" {
			return c.Name + "=" + c.Value
		}
	}
	t.Fatal("expected realms_session cookie after login")
	return ""
}
