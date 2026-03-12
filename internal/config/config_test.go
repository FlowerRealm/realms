package config_test

import (
	"testing"

	"realms/internal/config"

	"gopkg.in/yaml.v3"
)

func TestLoad_DefaultsToSQLite(t *testing.T) {
	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.DB.Driver != "sqlite" {
		t.Fatalf("expected db.driver=sqlite, got %q", cfg.DB.Driver)
	}
	if cfg.DB.SQLitePath == "" {
		t.Fatalf("expected sqlite_path to be set")
	}
}

func TestLoad_InferMySQLFromDSN(t *testing.T) {
	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_SQLITE_PATH", "")
	t.Setenv("REALMS_DB_DSN", "user:pass@tcp(127.0.0.1:3306)/realms?parseTime=true")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.DB.Driver != "mysql" {
		t.Fatalf("expected db.driver=mysql, got %q", cfg.DB.Driver)
	}
	if cfg.DB.DSN == "" {
		t.Fatalf("expected dsn to be set")
	}
}

func TestLoad_EnvOverridesDBDriver(t *testing.T) {
	t.Setenv("REALMS_DB_DSN", "user:pass@tcp(127.0.0.1:3306)/realms?parseTime=true")
	t.Setenv("REALMS_DB_DRIVER", "sqlite")
	t.Setenv("REALMS_SQLITE_PATH", "./data/test.db?_busy_timeout=30000")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.DB.Driver != "sqlite" {
		t.Fatalf("expected db.driver=sqlite, got %q", cfg.DB.Driver)
	}
	if cfg.DB.SQLitePath == "" {
		t.Fatalf("expected sqlite_path to be set")
	}
}

func TestLoad_Mode_Removed(t *testing.T) {
	t.Setenv("REALMS_MODE", "personal")

	if _, err := config.LoadFromEnv(); err == nil {
		t.Fatalf("expected REALMS_MODE to be rejected")
	}
}

func TestLoad_AdminAPIKeyEnvOverride(t *testing.T) {
	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")
	t.Setenv("REALMS_ADMIN_API_KEY", "  adm_from_env  ")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.Security.AdminAPIKey != "adm_from_env" {
		t.Fatalf("expected security.admin_api_key trimmed, got %q", cfg.Security.AdminAPIKey)
	}
}

func TestLoad_RuntimeDefaultsStayAvailable(t *testing.T) {
	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.Redis.KeyPrefix != "realms" {
		t.Fatalf("expected redis.key_prefix=realms, got=%q", cfg.Redis.KeyPrefix)
	}
	if cfg.Gateway.MaxRetryAttempts != 5 {
		t.Fatalf("expected gateway.max_retry_attempts=5, got=%d", cfg.Gateway.MaxRetryAttempts)
	}
	if cfg.Gateway.RetryBaseDelayMS != 300 {
		t.Fatalf("expected gateway.retry_base_delay_ms=300, got=%d", cfg.Gateway.RetryBaseDelayMS)
	}
	if cfg.Gateway.RetryMaxDelayMS != 3000 {
		t.Fatalf("expected gateway.retry_max_delay_ms=3000, got=%d", cfg.Gateway.RetryMaxDelayMS)
	}
	if cfg.Gateway.MaxRetryElapsedMS != 10000 {
		t.Fatalf("expected gateway.max_retry_elapsed_ms=10000, got=%d", cfg.Gateway.MaxRetryElapsedMS)
	}
	if cfg.Gateway.MaxFailoverSwitches != 8 {
		t.Fatalf("expected gateway.max_failover_switches=8, got=%d", cfg.Gateway.MaxFailoverSwitches)
	}
	if cfg.Gateway.WaitTimeoutMS != 30000 {
		t.Fatalf("expected gateway.wait_timeout_ms=30000, got=%d", cfg.Gateway.WaitTimeoutMS)
	}
	if cfg.Gateway.WaitQueueExtraSlots != 20 {
		t.Fatalf("expected gateway.wait_queue_extra_slots=20, got=%d", cfg.Gateway.WaitQueueExtraSlots)
	}
	if !cfg.Gateway.EnableErrorPassthrough {
		t.Fatalf("expected gateway.enable_error_passthrough=true by default")
	}
}

func TestLoad_RemovedGatewayAndRedisEnvIgnored(t *testing.T) {
	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")

	t.Setenv("REALMS_REDIS_ADDR", "127.0.0.1:6380")
	t.Setenv("REALMS_REDIS_PASSWORD", "secret")
	t.Setenv("REALMS_REDIS_DB", "2")
	t.Setenv("REALMS_REDIS_KEY_PREFIX", "rt")

	t.Setenv("REALMS_GATEWAY_MAX_RETRY_ATTEMPTS", "9")
	t.Setenv("REALMS_GATEWAY_RETRY_BASE_DELAY_MS", "120")
	t.Setenv("REALMS_GATEWAY_RETRY_MAX_DELAY_MS", "980")
	t.Setenv("REALMS_GATEWAY_MAX_RETRY_ELAPSED_MS", "12000")
	t.Setenv("REALMS_GATEWAY_MAX_FAILOVER_SWITCHES", "11")
	t.Setenv("REALMS_GATEWAY_USER_MAX_CONCURRENCY", "3")
	t.Setenv("REALMS_GATEWAY_CREDENTIAL_MAX_CONCURRENCY", "7")
	t.Setenv("REALMS_GATEWAY_WAIT_TIMEOUT_MS", "1500")
	t.Setenv("REALMS_GATEWAY_WAIT_QUEUE_EXTRA_SLOTS", "13")
	t.Setenv("REALMS_GATEWAY_ENABLE_ERROR_PASSTHROUGH", "false")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.Redis.Addr != "" || cfg.Redis.Password != "" || cfg.Redis.DB != 0 || cfg.Redis.KeyPrefix != "realms" {
		t.Fatalf("expected removed redis env to be ignored, got %+v", cfg.Redis)
	}
	if cfg.Gateway.MaxRetryAttempts != 5 ||
		cfg.Gateway.RetryBaseDelayMS != 300 ||
		cfg.Gateway.RetryMaxDelayMS != 3000 ||
		cfg.Gateway.MaxRetryElapsedMS != 10000 ||
		cfg.Gateway.MaxFailoverSwitches != 8 ||
		cfg.Gateway.UserMaxConcurrency != 0 ||
		cfg.Gateway.CredentialMaxConcurrency != 0 ||
		cfg.Gateway.WaitTimeoutMS != 30000 ||
		cfg.Gateway.WaitQueueExtraSlots != 20 ||
		!cfg.Gateway.EnableErrorPassthrough {
		t.Fatalf("expected removed gateway env to be ignored, got %+v", cfg.Gateway)
	}
}

func TestLoad_CompactGatewayLegacyEnvRejected(t *testing.T) {
	t.Setenv("REALMS_COMPACT_GATEWAY_BASE_URL", "")
	t.Setenv("REALMS_COMPACT_GATEWAY_KEY", "")
	t.Setenv("REALMS_SUB2API_BASE_URL", "https://legacy-gateway.example.com")
	t.Setenv("REALMS_SUB2API_GATEWAY_KEY", "legacy-key")
	t.Setenv("REALMS_SUB2API_TIMEOUT_MS", "4321")

	if _, err := config.LoadFromEnv(); err == nil {
		t.Fatalf("expected legacy compact gateway env to be rejected")
	}
}

func TestLoad_CompactGatewayUsesOnlyNewEnv(t *testing.T) {
	t.Setenv("REALMS_COMPACT_GATEWAY_BASE_URL", "https://new-gateway.example.com")
	t.Setenv("REALMS_COMPACT_GATEWAY_KEY", "new-key")
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.CompactGateway.BaseURL != "https://new-gateway.example.com" {
		t.Fatalf("expected new base url to win, got %q", cfg.CompactGateway.BaseURL)
	}
	if cfg.CompactGateway.GatewayKey != "new-key" {
		t.Fatalf("expected new gateway key to win, got %q", cfg.CompactGateway.GatewayKey)
	}
}

func TestLoad_CodexSessionTTLEnvRejected(t *testing.T) {
	t.Setenv("REALMS_CODEX_SESSION_TTL_SECONDS", "600")

	if _, err := config.LoadFromEnv(); err == nil {
		t.Fatalf("expected legacy codex session ttl env to be rejected")
	}
}

func TestConfigYAML_RejectsLegacySub2APIKey(t *testing.T) {
	var cfg config.Config
	err := yaml.Unmarshal([]byte(`
sub2api:
  base_url: https://legacy-gateway.example.com
  gateway_key: legacy-key
`), &cfg)
	if err == nil {
		t.Fatalf("expected yaml.Unmarshal to fail")
	}
	if got := err.Error(); got != "sub2api 已移除；请改用 compact_gateway" {
		t.Fatalf("expected legacy yaml error, got %q", got)
	}
}

func TestConfigYAML_AcceptsCompactGatewayKey(t *testing.T) {
	var cfg config.Config
	err := yaml.Unmarshal([]byte(`
compact_gateway:
  base_url: https://new-gateway.example.com
  gateway_key: new-key
`), &cfg)
	if err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if cfg.CompactGateway.BaseURL != "https://new-gateway.example.com" {
		t.Fatalf("expected new base url to win, got %q", cfg.CompactGateway.BaseURL)
	}
	if cfg.CompactGateway.GatewayKey != "new-key" {
		t.Fatalf("expected new gateway key to win, got %q", cfg.CompactGateway.GatewayKey)
	}
}
