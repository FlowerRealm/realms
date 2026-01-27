package e2e_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
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
	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ci-upstream", store.DefaultGroupName, 0, false, false, false, false)
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

	userID, err := st.CreateUser(ctx, "ci-user@example.com", "ci-user", []byte("x"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.NewFromInt(1)); err != nil {
		t.Fatalf("AddUserBalanceUSD: %v", err)
	}
	rawToken, err := auth.NewRandomToken("rlm_", 32)
	if err != nil {
		t.Fatalf("NewRandomToken: %v", err)
	}
	if _, _, err := st.CreateUserToken(ctx, userID, strPtr("ci-token"), rawToken); err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	cfg, err := config.LoadFromFile(filepath.Join(dir, "config-does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	cfg.Env = "dev"
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = ""
	cfg.DB.SQLitePath = dbPath
	cfg.Security.AllowOpenRegistration = false
	cfg.CodexOAuth.Enable = false

	app, err := server.NewApp(server.AppOptions{
		Config:  cfg,
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
	if err := writeCodexConfig(homeDir, model, baseURL); err != nil {
		t.Fatalf("writeCodexConfig: %v", err)
	}

	workDir := filepath.Join(dir, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(work): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "codex", "exec", "Reply with exactly: REALMS_CI_OK")
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
	if !strings.Contains(safeOut, "REALMS_CI_OK") {
		t.Fatalf("codex 输出不包含预期标记（REALMS_CI_OK）:\n%s", safeOut)
	}
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
