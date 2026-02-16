package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/scheduler"
	"realms/internal/server"
	"realms/internal/store"
	"realms/internal/version"
)

func TestCodexE2E_ConcurrentWindows_ProbeDueSSE(t *testing.T) {
	upstreamBaseURL := requiredEnvOrSkip(t, "REALMS_CI_UPSTREAM_BASE_URL", "BASE_URL", "UPSTREAM_BASE_URL")
	upstreamAPIKey := requiredEnvOrSkip(t, "REALMS_CI_UPSTREAM_API_KEY", "API_KEY", "UPSTREAM_API_KEY")
	model := requiredEnvOrSkip(t, "REALMS_CI_MODEL", "MODEL", "UPSTREAM_MODEL")

	if _, err := exec.LookPath("codex"); err != nil {
		if strings.TrimSpace(os.Getenv("REALMS_CI_ENFORCE_E2E")) != "" {
			t.Fatalf("codex 未安装或不在 PATH 中（err=%v）", err)
		}
		t.Skipf("codex 未安装或不在 PATH 中（err=%v）", err)
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
	upstreamBaseURL = strings.TrimRight(strings.TrimSpace(upstreamBaseURL), "/")
	epID, err := st.CreateUpstreamEndpoint(ctx, channelID, upstreamBaseURL, 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, epID, strPtr("ci"), strings.TrimSpace(upstreamAPIKey)); err != nil {
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

	// e2e 测试应当与外部环境变量解耦：清空可能影响 Load() 的配置项。
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

	const conns = 2
	testCtx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	baseURL := strings.TrimRight(ts.URL, "/") + "/v1"

	// 强制维持 probe_due：旧实现的 probe claim 单飞会把并发请求错误变成“上游不可用”（仅 1 个能成功）。
	stopProbe := maintainChannelProbeDueForTest(t, app, channelID)
	defer stopProbe()

	prompt := "Write a minimal Go program (package main) that prints REALMS_CI_OK. Output only the code."

	homeDirs := make([]string, 0, conns)
	workDirs := make([]string, 0, conns)
	for i := 0; i < conns; i++ {
		homeDir := filepath.Join(dir, fmt.Sprintf("home-%d", i))
		if err := writeCodexConfig(homeDir, model, baseURL); err != nil {
			t.Fatalf("writeCodexConfig(%d): %v", i, err)
		}
		workDir := filepath.Join(dir, fmt.Sprintf("work-%d", i))
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(work-%d): %v", i, err)
		}
		homeDirs = append(homeDirs, homeDir)
		workDirs = append(workDirs, workDir)
	}

	start := make(chan struct{})
	errCh := make(chan error, conns)
	var wg sync.WaitGroup

	for i := 0; i < conns; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start

			cmd := exec.CommandContext(testCtx, "codex", "exec", "--skip-git-repo-check", prompt)
			cmd.Dir = workDirs[i]
			cmd.Env = append(os.Environ(),
				"HOME="+homeDirs[i],
				"OPENAI_API_KEY="+rawToken,
				// 防止 codex 在缺失配置时走默认 OpenAI/Codex 登录流。
				"CODEX_API_KEY=",
			)
			out, err := cmd.CombinedOutput()
			safeOut := redact(string(out), upstreamAPIKey, rawToken)
			if err != nil {
				errCh <- fmt.Errorf("codex[%d] exec 失败: %v\n%s", i, err, safeOut)
				return
			}
			if !strings.Contains(safeOut, "package main") || !strings.Contains(safeOut, "REALMS_CI_OK") {
				errCh <- fmt.Errorf("codex[%d] 输出不包含预期片段（package main / REALMS_CI_OK）:\n%s", i, safeOut)
				return
			}
			errCh <- nil
		}(i)
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("并发 codex 失败: %v", err)
		}
	}

	usageEvents := waitUsageEventsByUser(t, st, testCtx, userID, conns)
	streamOK := 0
	for _, ev := range usageEvents {
		if ev.Endpoint == nil || strings.TrimSpace(*ev.Endpoint) != "/v1/responses" {
			continue
		}
		if ev.StatusCode >= 200 && ev.StatusCode < 300 && ev.IsStream {
			streamOK++
		}
	}
	if streamOK < conns {
		t.Fatalf("real stream usage_events 不符合预期（应>=并发窗口数）: got=%d want>=%d", streamOK, conns)
	}
}

func TestCodexCLI_E2E(t *testing.T) {
	upstreamBaseURL := requiredEnvOrSkip(t, "REALMS_CI_UPSTREAM_BASE_URL", "BASE_URL", "UPSTREAM_BASE_URL")
	upstreamAPIKey := requiredEnvOrSkip(t, "REALMS_CI_UPSTREAM_API_KEY", "API_KEY", "UPSTREAM_API_KEY")
	model := requiredEnvOrSkip(t, "REALMS_CI_MODEL", "MODEL", "UPSTREAM_MODEL")

	if _, err := exec.LookPath("codex"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("codex 未安装或不在 PATH 中（err=%v）", err)
		}
		t.Skipf("codex 未安装或不在 PATH 中（err=%v）", err)
	}

	p1, p2 := buildTwoStepPrompts()
	runCodexE2E(t, codexE2EConfig{
		model:           model,
		upstreamBaseURL: upstreamBaseURL,
		upstreamAPIKey:  upstreamAPIKey,
		prompts:         []string{p1, p2},
		wantEvents:      2,
		wantCacheHit:    true,
	})
}

func TestCodexCLI_E2E_FakeUpstream_Cache(t *testing.T) {
	if strings.TrimSpace(os.Getenv("REALMS_CI_ENFORCE_E2E")) == "" {
		t.Skip("未设置 REALMS_CI_ENFORCE_E2E，跳过 E2E")
	}

	if _, err := exec.LookPath("codex"); err != nil {
		t.Fatalf("codex 未安装或不在 PATH 中（err=%v）", err)
	}

	model := strings.TrimSpace(os.Getenv("REALMS_CI_MODEL"))
	if model == "" {
		model = "gpt-5.2"
	}

	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}

		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		_ = r.Body.Close()

		stream := strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") || requestWantsStream(body)
		n := upstreamCalls.Add(1)

		// 第二次请求开始返回 cached_tokens>0，用于验证 Realms 的缓存 Token 落库口径。
		inputTokens := int64(100)
		outputTokens := int64(20)
		cachedTokens := int64(0)
		if n >= 2 {
			cachedTokens = 80
		}

		code := `package main

import "fmt"

func main() {
	fmt.Println("REALMS_CI_OK")
}
`

		usage := map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
			"total_tokens":  inputTokens + outputTokens,
			"input_tokens_details": map[string]any{
				"cached_tokens": cachedTokens,
			},
		}
		resp := map[string]any{
			"id":      fmt.Sprintf("resp_test_%d", n),
			"object":  "response",
			"created": 0,
			"model":   model,
			"output": []any{
				map[string]any{
					"id":   fmt.Sprintf("msg_test_%d", n),
					"type": "message",
					"role": "assistant",
					"content": []any{
						map[string]any{"type": "output_text", "text": code},
					},
				},
			},
			"usage":  usage,
			"status": "completed",
		}

		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)

			// 最小化的 Responses SSE：delta + completed + DONE。
			_ = writeSSEData(w, map[string]any{
				"type":          "response.output_text.delta",
				"response_id":   resp["id"],
				"output_index":  0,
				"content_index": 0,
				"delta":         code,
			})
			_ = writeSSEData(w, map[string]any{
				"type":     "response.completed",
				"response": resp,
			})
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	p1, p2 := buildTwoStepPrompts()
	runCodexE2E(t, codexE2EConfig{
		model:           model,
		upstreamBaseURL: strings.TrimRight(strings.TrimSpace(upstream.URL), "/") + "/v1",
		upstreamAPIKey:  "sk-test",
		prompts:         []string{p1, p2},
		wantEvents:      2,
		wantCacheHit:    true,
	})
}

type codexE2EConfig struct {
	model           string
	upstreamBaseURL string
	upstreamAPIKey  string

	prompts []string

	wantEvents   int
	wantCacheHit bool
}

func runCodexE2E(t *testing.T, e2eCfg codexE2EConfig) {
	t.Helper()

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
	e2eCfg.upstreamBaseURL = strings.TrimRight(strings.TrimSpace(e2eCfg.upstreamBaseURL), "/")
	epID, err := st.CreateUpstreamEndpoint(ctx, channelID, e2eCfg.upstreamBaseURL, 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, epID, strPtr("ci"), strings.TrimSpace(e2eCfg.upstreamAPIKey)); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential: %v", err)
	}

	e2eCfg.model = strings.TrimSpace(e2eCfg.model)
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            e2eCfg.model,
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
		PublicID:      e2eCfg.model,
		UpstreamModel: e2eCfg.model,
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

	// e2e 测试应当与外部环境变量解耦：清空可能影响 Load() 的配置项。
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

	baseURL := strings.TrimRight(ts.URL, "/") + "/v1"
	homeDir := filepath.Join(dir, "home")
	if err := writeCodexConfig(homeDir, e2eCfg.model, baseURL); err != nil {
		t.Fatalf("writeCodexConfig: %v", err)
	}

	workDir := filepath.Join(dir, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(work): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	prompt1 := "Write a minimal Go program (package main) that prints REALMS_CI_OK. Output only the code."
	prompt2 := prompt1
	if len(e2eCfg.prompts) >= 1 {
		prompt1 = e2eCfg.prompts[0]
	}
	if len(e2eCfg.prompts) >= 2 {
		prompt2 = e2eCfg.prompts[1]
	}

	safeOut1 := runCodexExec(t, ctx, workDir, homeDir, rawToken, e2eCfg.upstreamAPIKey, prompt1)
	if !strings.Contains(safeOut1, "package main") || !strings.Contains(safeOut1, "REALMS_CI_OK") {
		t.Fatalf("codex 第一次输出不包含预期代码片段（package main / REALMS_CI_OK）:\n%s", safeOut1)
	}

	if e2eCfg.wantEvents >= 2 {
		safeOut2 := runCodexExec(t, ctx, workDir, homeDir, rawToken, e2eCfg.upstreamAPIKey, prompt2)
		if !strings.Contains(safeOut2, "package main") || !strings.Contains(safeOut2, "REALMS_CI_OK") {
			t.Fatalf("codex 第二次输出不包含预期代码片段（package main / REALMS_CI_OK）:\n%s", safeOut2)
		}
	}

	usageEvents := waitUsageEventsByUser(t, st, ctx, userID, e2eCfg.wantEvents)
	if got := len(usageEvents); got != e2eCfg.wantEvents {
		t.Fatalf("usage_events 数量不符合预期: got=%d want=%d", got, e2eCfg.wantEvents)
	}

	// ListUsageEventsByUser 按 id DESC 排序：第 0 条为第二次请求（最新）。
	if e2eCfg.wantEvents == 1 {
		ev := usageEvents[0]
		assertUsageEventTokens(t, "only", ev)
		return
	}

	if e2eCfg.wantEvents == 2 {
		second := usageEvents[0]
		first := usageEvents[1]

		assertUsageEventTokens(t, "first", first)
		assertUsageEventTokens(t, "second", second)

		if e2eCfg.wantCacheHit {
			if second.CachedInputTokens == nil || *second.CachedInputTokens <= 0 {
				t.Fatalf("第二次请求未命中缓存 Token（cached_input_tokens）: %v (id=%d)", second.CachedInputTokens, second.ID)
			}
		}
		return
	}

	t.Fatalf("不支持的 wantEvents=%d（当前仅支持 1 或 2）", e2eCfg.wantEvents)
}

// requiredEnvOrSkip returns the first non-empty env value in keys.
// When REALMS_CI_ENFORCE_E2E is set, missing envs fail the test; otherwise, they skip it.
func requiredEnvOrSkip(t *testing.T, keys ...string) string {
	t.Helper()

	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}

	if strings.TrimSpace(os.Getenv("REALMS_CI_ENFORCE_E2E")) != "" {
		t.Fatalf("缺少必需环境变量（任一即可）：%s", strings.Join(keys, ", "))
	}
	t.Skipf("缺少必需环境变量（任一即可）：%s", strings.Join(keys, ", "))
	return ""
}

// writeCodexConfig writes a minimal Codex CLI config.toml into homeDir.
func writeCodexConfig(homeDir string, model string, baseURL string) error {
	if err := os.MkdirAll(filepath.Join(homeDir, ".codex"), 0o755); err != nil {
		return err
	}
	cfg := fmt.Sprintf(`disable_response_storage = true
model_provider = "realms"
model = %q

[model_providers.realms]
name = "Realms"
base_url = %q
wire_api = "responses"
requires_openai_auth = true
env_key = "OPENAI_API_KEY"
	`, model, baseURL)
	return os.WriteFile(filepath.Join(homeDir, ".codex", "config.toml"), []byte(cfg), 0o600)
}

// redact replaces sensitive substrings in s for safer logs.
func redact(s string, secrets ...string) string {
	out := s
	for _, sec := range secrets {
		sec = strings.TrimSpace(sec)
		if sec == "" {
			continue
		}
		out = strings.ReplaceAll(out, sec, "<redacted>")
	}
	return out
}

// strPtr returns nil for blank strings, otherwise a pointer to the trimmed value.
func strPtr(s string) *string {
	v := strings.TrimSpace(s)
	if v == "" {
		return nil
	}
	return &v
}

func runCodexExec(t *testing.T, ctx context.Context, workDir, homeDir, rawToken, upstreamAPIKey, prompt string) string {
	t.Helper()

	cmd := exec.CommandContext(ctx, "codex", "exec", "--skip-git-repo-check", prompt)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"HOME="+homeDir,
		"OPENAI_API_KEY="+rawToken,
		// 防止 codex 在缺失配置时走默认 OpenAI/Codex 登录流。
		"CODEX_API_KEY=",
	)
	out, err := cmd.CombinedOutput()
	safeOut := redact(string(out), upstreamAPIKey, rawToken)
	if err != nil {
		t.Fatalf("codex exec 失败: %v\n%s", err, safeOut)
	}
	return safeOut
}

func waitUsageEventsByUser(t *testing.T, st *store.Store, ctx context.Context, userID int64, minCount int) []store.UsageEvent {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for {
		events, err := st.ListUsageEventsByUser(ctx, userID, 10, nil)
		if err != nil {
			t.Fatalf("ListUsageEventsByUser: %v", err)
		}
		if len(events) >= minCount {
			return events
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待 usage_events 超时: got=%d want>=%d", len(events), minCount)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func assertUsageEventTokens(t *testing.T, label string, ev store.UsageEvent) {
	t.Helper()

	if ev.State != store.UsageStateCommitted {
		t.Fatalf("%s: usage_event state 不符合预期: got=%q want=%q (id=%d)", label, ev.State, store.UsageStateCommitted, ev.ID)
	}
	if ev.InputTokens == nil || *ev.InputTokens <= 0 {
		t.Fatalf("%s: input_tokens 未记录或不合法: %v (id=%d)", label, ev.InputTokens, ev.ID)
	}
	if ev.OutputTokens == nil || *ev.OutputTokens <= 0 {
		t.Fatalf("%s: output_tokens 未记录或不合法: %v (id=%d)", label, ev.OutputTokens, ev.ID)
	}
	if ev.CachedInputTokens != nil {
		if *ev.CachedInputTokens < 0 {
			t.Fatalf("%s: cached_input_tokens 不应为负数: %d (id=%d)", label, *ev.CachedInputTokens, ev.ID)
		}
		if ev.InputTokens != nil && *ev.CachedInputTokens > *ev.InputTokens {
			t.Fatalf("%s: cached_input_tokens 不应大于 input_tokens: cached=%d input=%d (id=%d)", label, *ev.CachedInputTokens, *ev.InputTokens, ev.ID)
		}
	}
	if ev.CachedOutputTokens != nil {
		if *ev.CachedOutputTokens < 0 {
			t.Fatalf("%s: cached_output_tokens 不应为负数: %d (id=%d)", label, *ev.CachedOutputTokens, ev.ID)
		}
		if ev.OutputTokens != nil && *ev.CachedOutputTokens > *ev.OutputTokens {
			t.Fatalf("%s: cached_output_tokens 不应大于 output_tokens: cached=%d output=%d (id=%d)", label, *ev.CachedOutputTokens, *ev.OutputTokens, ev.ID)
		}
	}
}

func requestWantsStream(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	switch v := payload["stream"].(type) {
	case bool:
		return v
	case string:
		v = strings.TrimSpace(v)
		return v == "1" || strings.EqualFold(v, "true")
	default:
		return false
	}
}

func writeSSEData(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, "data: "+string(b)+"\n\n")
	return err
}

func maintainChannelProbeDueForTest(t *testing.T, app *server.App, channelID int64) func() {
	t.Helper()
	if app == nil || channelID <= 0 {
		t.Fatalf("invalid probe_due target (app_nil=%v channel_id=%d)", app == nil, channelID)
	}

	sched := unexportedField(t, app, "sched").Interface().(*scheduler.Scheduler)
	st := unexportedField(t, sched, "state").Interface().(*scheduler.State)

	mu := unexportedField(t, st, "mu").Addr().Interface().(*sync.Mutex)
	probes := unexportedField(t, st, "channelProbeDueAt").Interface().(map[int64]time.Time)

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				close(doneCh)
				return
			case <-ticker.C:
				mu.Lock()
				probes[channelID] = time.Now()
				mu.Unlock()
			}
		}
	}()

	mu.Lock()
	probes[channelID] = time.Now()
	mu.Unlock()

	return func() {
		close(stopCh)
		<-doneCh
	}
}

func setChannelProbeDueForTest(t *testing.T, app *server.App, channelID int64, dueAt time.Time) {
	t.Helper()
	if app == nil || channelID <= 0 || dueAt.IsZero() {
		t.Fatalf("invalid probe due input (app_nil=%v channel_id=%d due_at=%v)", app == nil, channelID, dueAt)
	}

	sched := unexportedField(t, app, "sched").Interface().(*scheduler.Scheduler)
	st := unexportedField(t, sched, "state").Interface().(*scheduler.State)

	mu := unexportedField(t, st, "mu").Addr().Interface().(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	probes := unexportedField(t, st, "channelProbeDueAt").Interface().(map[int64]time.Time)
	probes[channelID] = dueAt
}

func unexportedField(t *testing.T, ptr any, field string) reflect.Value {
	t.Helper()
	v := reflect.ValueOf(ptr)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		t.Fatalf("unexportedField expects pointer, got %T", ptr)
	}
	e := v.Elem()
	f := e.FieldByName(field)
	if !f.IsValid() {
		t.Fatalf("field %q not found on %T", field, ptr)
	}
	if !f.CanAddr() {
		t.Fatalf("field %q on %T is not addressable", field, ptr)
	}
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
}

func buildTwoStepPrompts() (string, string) {
	shared := buildCacheBait(256)
	code := `package main

import "fmt"

func featureA(x int) int {
	return x + 1
}

func featureB(x int) int {
	return x * 2
}

func main() {
	fmt.Println("REALMS_CI_OK", featureA(1), featureB(2))
}
`

	prefix := shared + "\nHere is the current code:\n\n```go\n" + code + "```\n\n"
	p1 := prefix + "Task 1: Modify featureA to return x+2. Output only the full updated code (no markdown)."
	p2 := prefix + "Task 2: Modify featureB to return x*3. Output only the full updated code (no markdown)."
	return p1, p2
}

func buildCacheBait(lines int) string {
	if lines <= 0 {
		lines = 1
	}
	var b strings.Builder
	b.WriteString("CACHE_BAIT_BEGIN\n")
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&b, "CACHE_BAIT_LINE_%04d: This is stable context to encourage prompt caching across two requests.\n", i+1)
	}
	b.WriteString("CACHE_BAIT_END\n")
	return b.String()
}
