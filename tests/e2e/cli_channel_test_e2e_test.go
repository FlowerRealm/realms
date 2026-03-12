package e2e_test

import (
	"bufio"
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
//  1. GET /api/channel/test/:id?stream=1 delegates to the CLI runner and returns SSE events
//  2. last_test_at is NOT updated in the database (result isolation)
func TestCLIChannelTest_E2E(t *testing.T) {
	const model = "gpt-test"

	// ---------------------------------------------------------------
	// Fake CLI runner: accepts POST /v1/test, returns a canned result.
	// ---------------------------------------------------------------
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
			"output":     "OK",
			"error":      "",
		})
	}))
	defer fakeRunner.Close()

	// ---------------------------------------------------------------
	// Bootstrap: temp SQLite + seed data + full app
	// ---------------------------------------------------------------
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
	epID, err := st.CreateUpstreamEndpoint(ctx, channelID, "https://api.example.com/v1", 0)
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

	// Create a root user for admin API access.
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	rootUserID, err := st.CreateUser(ctx, "root@example.com", "root", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Record initial last_test_at.
	chBefore, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID (before): %v", err)
	}

	// ---------------------------------------------------------------
	// Start the full app with CLI runner configured.
	// ---------------------------------------------------------------
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

	// ---------------------------------------------------------------
	// Login as admin.
	// ---------------------------------------------------------------
	sessionCookie := loginAsRoot(t, ts.URL, client, rootUserID)

	// ---------------------------------------------------------------
	// Test 1: GET /api/channel/test/:id?stream=1 — SSE stream from CLI runner.
	// ---------------------------------------------------------------
	t.Run("cli_test_sse_delegation", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/channel/test/%d?stream=1", ts.URL, channelID)
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Cookie", sessionCookie)
		req.Header.Set("Realms-User", strconv.FormatInt(rootUserID, 10))

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("test channel request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		// Parse SSE events.
		events := parseSSEEvents(t, resp.Body)

		// Verify expected events.
		hasStart := false
		hasModelDone := false
		hasSummary := false
		hasCLIRunner := false

		for _, ev := range events {
			switch ev.name {
			case "start":
				hasStart = true
			case "model_done":
				hasModelDone = true
			case "summary":
				hasSummary = true
			}
			if strings.Contains(ev.data, `"cli_runner"`) {
				hasCLIRunner = true
			}
		}

		if !hasStart {
			t.Error("expected SSE 'start' event")
		}
		if !hasModelDone {
			t.Error("expected SSE 'model_done' event")
		}
		if !hasSummary {
			t.Error("expected SSE 'summary' event")
		}
		if !hasCLIRunner {
			t.Error("expected source='cli_runner' in SSE events")
		}
	})

	// ---------------------------------------------------------------
	// Test 2: Verify result isolation — last_test_at NOT updated.
	// ---------------------------------------------------------------
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

	// ---------------------------------------------------------------
	// Preflight: CLI runner healthz
	// ---------------------------------------------------------------
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

	// ---------------------------------------------------------------
	// Bootstrap
	// ---------------------------------------------------------------
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

	// ---------------------------------------------------------------
	// App
	// ---------------------------------------------------------------
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

	// ---------------------------------------------------------------
	// Test: SSE delegation → real CLI runner → real upstream
	// ---------------------------------------------------------------
	t.Run("real_cli_test_sse", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/channel/test/%d?stream=1", ts.URL, channelID)
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Cookie", sessionCookie)
		req.Header.Set("Realms-User", strconv.FormatInt(rootUserID, 10))

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("test channel request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		events := parseSSEEvents(t, resp.Body)

		hasStart := false
		hasModelDone := false
		hasSummary := false
		summaryOK := false

		for _, ev := range events {
			switch ev.name {
			case "start":
				hasStart = true
			case "model_done":
				hasModelDone = true
				// model_done 的 result 应该包含 "ok":true
				if strings.Contains(ev.data, `"ok":true`) {
					t.Logf("model_done result OK")
				} else {
					t.Logf("model_done data: %s", ev.data)
				}
			case "summary":
				hasSummary = true
				var s struct {
					Data struct {
						Summary struct {
							OK      bool   `json:"ok"`
							Source  string `json:"source"`
							Message string `json:"message"`
						} `json:"summary"`
					} `json:"data"`
				}
				if err := json.Unmarshal([]byte(ev.data), &s); err == nil {
					summaryOK = s.Data.Summary.OK
					if !summaryOK {
						t.Logf("summary 报告测试失败: source=%s message=%s", s.Data.Summary.Source, s.Data.Summary.Message)
					}
				} else {
					t.Logf("summary data (raw): %s", ev.data)
				}
			}
		}

		if !hasStart {
			t.Error("expected SSE 'start' event")
		}
		if !hasModelDone {
			t.Error("expected SSE 'model_done' event")
		}
		if !hasSummary {
			t.Error("expected SSE 'summary' event")
		}
		if !summaryOK {
			t.Error("expected summary.ok=true (real upstream test should succeed)")
		}
	})

	// ---------------------------------------------------------------
	// Verify result isolation after real test
	// ---------------------------------------------------------------
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

type sseEvent struct {
	name string
	data string
}

// parseSSEEvents reads an SSE stream and returns all events.
func parseSSEEvents(t *testing.T, r io.Reader) []sseEvent {
	t.Helper()

	var events []sseEvent
	scanner := bufio.NewScanner(r)
	var currentName, currentData string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			currentName = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			currentData = strings.TrimPrefix(line, "data: ")
		} else if line == "" && currentName != "" {
			events = append(events, sseEvent{name: currentName, data: currentData})
			currentName = ""
			currentData = ""
		}
	}
	// Flush last event if stream didn't end with empty line.
	if currentName != "" {
		events = append(events, sseEvent{name: currentName, data: currentData})
	}

	return events
}
