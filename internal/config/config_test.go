package config_test

import (
	"testing"

	"realms/internal/config"

	"gopkg.in/yaml.v3"
)

func expectLoadEnvErrorContains(t *testing.T, want string) {
	t.Helper()

	if _, err := config.LoadFromEnv(); err == nil {
		t.Fatalf("expected LoadFromEnv to fail")
	} else if got := err.Error(); got != want {
		t.Fatalf("expected error %q, got %q", want, got)
	}
}

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

func TestLoad_DevDBEnvOverridesBaseDBSettings(t *testing.T) {
	t.Setenv("REALMS_ENV", "dev")
	t.Setenv("REALMS_DB_DRIVER", "mysql")
	t.Setenv("REALMS_DB_DSN", "user:pass@tcp(127.0.0.1:3306)/wrong?parseTime=true")
	t.Setenv("REALMS_SQLITE_PATH", "./data/wrong.db?_busy_timeout=30000")
	t.Setenv("REALMS_DB_DRIVER_DEV", "sqlite")
	t.Setenv("REALMS_DB_DSN_DEV", "")
	t.Setenv("REALMS_SQLITE_PATH_DEV", "./data/dev.db?_busy_timeout=30000")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.DB.Driver != "sqlite" {
		t.Fatalf("expected dev db.driver=sqlite, got %q", cfg.DB.Driver)
	}
	if cfg.DB.SQLitePath != "./data/dev.db?_busy_timeout=30000" {
		t.Fatalf("expected dev sqlite_path override, got %q", cfg.DB.SQLitePath)
	}
	if cfg.DB.DSN != "user:pass@tcp(127.0.0.1:3306)/wrong?parseTime=true" {
		t.Fatalf("expected base dsn to remain when REALMS_DB_DSN_DEV is empty, got %q", cfg.DB.DSN)
	}
}

func TestLoad_NonDevIgnoresDevDBOverrides(t *testing.T) {
	t.Setenv("REALMS_ENV", "prod")
	t.Setenv("REALMS_DB_DRIVER", "mysql")
	t.Setenv("REALMS_DB_DSN", "user:pass@tcp(127.0.0.1:3306)/realms?parseTime=true")
	t.Setenv("REALMS_SQLITE_PATH", "./data/base.db?_busy_timeout=30000")
	t.Setenv("REALMS_DB_DRIVER_DEV", "sqlite")
	t.Setenv("REALMS_DB_DSN_DEV", "user:pass@tcp(127.0.0.1:23306)/dev?parseTime=true")
	t.Setenv("REALMS_SQLITE_PATH_DEV", "./data/dev.db?_busy_timeout=30000")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.DB.Driver != "mysql" {
		t.Fatalf("expected prod db.driver=mysql, got %q", cfg.DB.Driver)
	}
	if cfg.DB.DSN != "user:pass@tcp(127.0.0.1:3306)/realms?parseTime=true" {
		t.Fatalf("expected prod dsn unchanged, got %q", cfg.DB.DSN)
	}
	if cfg.DB.SQLitePath != "./data/base.db?_busy_timeout=30000" {
		t.Fatalf("expected prod sqlite_path unchanged, got %q", cfg.DB.SQLitePath)
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

func TestLoad_GatewayAndRedisDefaults(t *testing.T) {
	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")

	t.Setenv("REALMS_REDIS_ADDR", "")
	t.Setenv("REALMS_REDIS_PASSWORD", "")
	t.Setenv("REALMS_REDIS_DB", "")
	t.Setenv("REALMS_REDIS_KEY_PREFIX", "")

	t.Setenv("REALMS_GATEWAY_MAX_RETRY_ATTEMPTS", "")
	t.Setenv("REALMS_GATEWAY_RETRY_BASE_DELAY_MS", "")
	t.Setenv("REALMS_GATEWAY_RETRY_MAX_DELAY_MS", "")
	t.Setenv("REALMS_GATEWAY_MAX_RETRY_ELAPSED_MS", "")
	t.Setenv("REALMS_GATEWAY_MAX_FAILOVER_SWITCHES", "")
	t.Setenv("REALMS_GATEWAY_USER_MAX_CONCURRENCY", "")
	t.Setenv("REALMS_GATEWAY_CREDENTIAL_MAX_CONCURRENCY", "")
	t.Setenv("REALMS_GATEWAY_WAIT_TIMEOUT_MS", "")
	t.Setenv("REALMS_GATEWAY_WAIT_QUEUE_EXTRA_SLOTS", "")
	t.Setenv("REALMS_GATEWAY_ENABLE_ERROR_PASSTHROUGH", "")

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

func TestLoad_GatewayAndRedisEnvOverrides(t *testing.T) {
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
	if cfg.Redis.Addr != "127.0.0.1:6380" || cfg.Redis.Password != "secret" || cfg.Redis.DB != 2 || cfg.Redis.KeyPrefix != "rt" {
		t.Fatalf("unexpected redis config: %+v", cfg.Redis)
	}
	if cfg.Gateway.MaxRetryAttempts != 9 ||
		cfg.Gateway.RetryBaseDelayMS != 120 ||
		cfg.Gateway.RetryMaxDelayMS != 980 ||
		cfg.Gateway.MaxRetryElapsedMS != 12000 ||
		cfg.Gateway.MaxFailoverSwitches != 11 ||
		cfg.Gateway.UserMaxConcurrency != 3 ||
		cfg.Gateway.CredentialMaxConcurrency != 7 ||
		cfg.Gateway.WaitTimeoutMS != 1500 ||
		cfg.Gateway.WaitQueueExtraSlots != 13 ||
		cfg.Gateway.EnableErrorPassthrough {
		t.Fatalf("unexpected gateway config: %+v", cfg.Gateway)
	}
}

func TestLoad_GatewayZeroEnvOverridesPreserved(t *testing.T) {
	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")

	t.Setenv("REALMS_GATEWAY_MAX_RETRY_ATTEMPTS", "0")
	t.Setenv("REALMS_GATEWAY_RETRY_BASE_DELAY_MS", "0")
	t.Setenv("REALMS_GATEWAY_RETRY_MAX_DELAY_MS", "0")
	t.Setenv("REALMS_GATEWAY_MAX_RETRY_ELAPSED_MS", "0")
	t.Setenv("REALMS_GATEWAY_MAX_FAILOVER_SWITCHES", "0")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.Gateway.MaxRetryAttempts != 0 ||
		cfg.Gateway.RetryBaseDelayMS != 0 ||
		cfg.Gateway.RetryMaxDelayMS != 0 ||
		cfg.Gateway.MaxRetryElapsedMS != 0 ||
		cfg.Gateway.MaxFailoverSwitches != 0 {
		t.Fatalf("expected explicit gateway zeros to be preserved, got %+v", cfg.Gateway)
	}
}

func TestLoad_CompactGatewayRejectsLegacyBaseURLEnv(t *testing.T) {
	t.Setenv("REALMS_SUB2API_BASE_URL", "https://legacy-gateway.example.com")
	expectLoadEnvErrorContains(t, "REALMS_SUB2API_BASE_URL 已移除；请改用 REALMS_COMPACT_GATEWAY_BASE_URL")
}

func TestLoad_CompactGatewayRejectsLegacyKeyEnv(t *testing.T) {
	t.Setenv("REALMS_SUB2API_GATEWAY_KEY", "legacy-key")
	expectLoadEnvErrorContains(t, "REALMS_SUB2API_GATEWAY_KEY 已移除；请改用 REALMS_COMPACT_GATEWAY_KEY")
}

func TestLoad_CompactGatewayRejectsLegacyTimeoutEnv(t *testing.T) {
	t.Setenv("REALMS_SUB2API_TIMEOUT_MS", "4321")
	expectLoadEnvErrorContains(t, "REALMS_SUB2API_TIMEOUT_MS 已移除；请改用 REALMS_COMPACT_GATEWAY_TIMEOUT_MS")
}

func TestLoad_CompactGatewayNewEnvStillLoads(t *testing.T) {
	t.Setenv("REALMS_COMPACT_GATEWAY_BASE_URL", "https://new-gateway.example.com")
	t.Setenv("REALMS_COMPACT_GATEWAY_KEY", "new-key")
	t.Setenv("REALMS_COMPACT_GATEWAY_TIMEOUT_MS", "9876")
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
	if cfg.CompactGateway.TimeoutMS != 9876 {
		t.Fatalf("expected new timeout to win, got %d", cfg.CompactGateway.TimeoutMS)
	}
}

func TestConfigYAML_RejectsLegacySub2APIKey(t *testing.T) {
	var cfg config.Config
	err := yaml.Unmarshal([]byte(`
sub2api:
  base_url: https://legacy-gateway.example.com
  gateway_key: legacy-key
  timeout_ms: 4321
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
  timeout_ms: 9876
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
	if cfg.CompactGateway.TimeoutMS != 9876 {
		t.Fatalf("expected new timeout to win, got %d", cfg.CompactGateway.TimeoutMS)
	}
}
