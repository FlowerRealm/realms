// Package config 负责读取并合并服务配置（环境变量为主，可选读取 YAML 配置），避免在业务代码里散落解析逻辑。
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Env            string               `yaml:"env"`
	Mode           Mode                 `yaml:"mode"`
	Server         ServerConfig         `yaml:"server"`
	DB             DBConfig             `yaml:"db"`
	Redis          RedisConfig          `yaml:"redis"`
	Gateway        GatewayConfig        `yaml:"gateway"`
	CompactGateway CompactGatewayConfig `yaml:"compact_gateway"`
	Security       SecurityConfig       `yaml:"security"`
	Debug          DebugConfig          `yaml:"debug"`
	Billing        BillingConfig        `yaml:"billing"`
	SMTP           SMTPConfig           `yaml:"smtp"`
	EmailVerif     EmailVerifConfig     `yaml:"email_verification"`
	Tickets        TicketsConfig        `yaml:"tickets"`

	// ChannelTestCLIRunnerURL 是可选的 CLI Runner 服务地址（如 http://cli-runner:3100）。
	// 配置后启用基于 CLI（Codex/Claude/Gemini）的渠道测试功能。
	ChannelTestCLIRunnerURL string `yaml:"channel_test_cli_runner_url"`

	// ChannelTestCLIConcurrency 控制“渠道测试连接（CLI runner）”的模型探测并发上限。
	// 该并发仅影响单次“测试连接”的模型循环，不影响真实转发链路。
	ChannelTestCLIConcurrency int `yaml:"channel_test_cli_concurrency"`

	// AppSettingsDefaults 提供管理后台"系统设置"（app_settings）的配置文件默认值。
	// 仅当数据库未配置对应 app_settings 键时才会生效（app_settings 仍优先）。
	AppSettingsDefaults AppSettingsDefaultsConfig `yaml:"app_settings_defaults"`

	SessionSecret string `yaml:"session_secret"`
}

func (c *Config) UnmarshalYAML(value *yaml.Node) error {
	type rawConfig Config

	var raw rawConfig
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*c = Config(raw)

	if hasYAMLKey(value, "compact_gateway") {
		return nil
	}
	for i := 0; i+1 < len(value.Content); i += 2 {
		if strings.TrimSpace(value.Content[i].Value) != "sub2api" {
			continue
		}
		var legacy CompactGatewayConfig
		if err := value.Content[i+1].Decode(&legacy); err != nil {
			return err
		}
		c.CompactGateway = legacy
		return nil
	}
	return nil
}

func hasYAMLKey(node *yaml.Node, key string) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if strings.TrimSpace(node.Content[i].Value) == key {
			return true
		}
	}
	return false
}

type Mode string

const (
	ModeBusiness Mode = "business"
	ModePersonal Mode = "personal"
)

type AppSettingsDefaultsConfig struct {
	SiteBaseURL   string `yaml:"site_base_url"`
	AdminTimeZone string `yaml:"admin_time_zone"`

	FeatureDisableWebAnnouncements bool `yaml:"feature_disable_web_announcements"`
	FeatureDisableWebTokens        bool `yaml:"feature_disable_web_tokens"`
	FeatureDisableWebUsage         bool `yaml:"feature_disable_web_usage"`

	FeatureDisableModels bool `yaml:"feature_disable_models"`

	FeatureDisableBilling bool `yaml:"feature_disable_billing"`
	FeatureDisableTickets bool `yaml:"feature_disable_tickets"`

	FeatureDisableAdminChannels      bool `yaml:"feature_disable_admin_channels"`
	FeatureDisableAdminChannelGroups bool `yaml:"feature_disable_admin_channel_groups"`
	FeatureDisableAdminUsers         bool `yaml:"feature_disable_admin_users"`
	FeatureDisableAdminUsage         bool `yaml:"feature_disable_admin_usage"`
	FeatureDisableAdminAnnouncements bool `yaml:"feature_disable_admin_announcements"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
}

type DBConfig struct {
	// Driver 支持 mysql/sqlite；为空时会根据 dsn 自动推断（兼容旧配置）。
	// - 当 dsn 非空且 driver 为空：推断为 mysql
	// - 其他情况默认 sqlite
	Driver string `yaml:"driver"`
	// DSN 仅用于 MySQL（示例：user:pass@tcp(127.0.0.1:3306)/realms?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&time_zone=%27%2B00%3A00%27）
	DSN string `yaml:"dsn"`
	// SQLitePath 是 SQLite 数据库文件路径（可包含 DSN query，如 ?_busy_timeout=30000）。
	SQLitePath string `yaml:"sqlite_path"`

	// MigrationLockName 是 MySQL 启动迁移的全局互斥锁名（用于多实例并发启动）。
	MigrationLockName string `yaml:"migration_lock_name"`
	// MigrationLockTimeoutSeconds 是 MySQL 启启动迁移等待锁的超时（秒）。
	// - 0 表示不等待（立即失败）
	MigrationLockTimeoutSeconds int `yaml:"migration_lock_timeout_seconds"`
}

type CompactGatewayConfig struct {
	BaseURL    string `yaml:"base_url"`
	GatewayKey string `yaml:"gateway_key"`
}

type RedisConfig struct {
	Addr      string `yaml:"addr"`
	Password  string `yaml:"password"`
	DB        int    `yaml:"db"`
	KeyPrefix string `yaml:"key_prefix"`
}

type GatewayConfig struct {
	MaxRetryAttempts  int `yaml:"max_retry_attempts"`
	RetryBaseDelayMS  int `yaml:"retry_base_delay_ms"`
	RetryMaxDelayMS   int `yaml:"retry_max_delay_ms"`
	MaxRetryElapsedMS int `yaml:"max_retry_elapsed_ms"`

	MaxFailoverSwitches int `yaml:"max_failover_switches"`

	UserMaxConcurrency       int `yaml:"user_max_concurrency"`
	CredentialMaxConcurrency int `yaml:"credential_max_concurrency"`
	WaitTimeoutMS            int `yaml:"wait_timeout_ms"`
	WaitQueueExtraSlots      int `yaml:"wait_queue_extra_slots"`

	EnableErrorPassthrough bool `yaml:"enable_error_passthrough"`
}

type SecurityConfig struct {
	AdminAPIKey string `yaml:"admin_api_key"`

	// SubscriptionOrderWebhookSecret 用于支付回调等“系统侧”操作的简单鉴权。
	// 为空表示禁用相关 webhook（避免未配置时被外部直接调用）。
	SubscriptionOrderWebhookSecret string `yaml:"subscription_order_webhook_secret"`
}

type DebugConfig struct {
	ProxyLog ProxyLogConfig `yaml:"proxy_log"`
}

type ProxyLogConfig struct {
	Enable bool   `yaml:"enable"`
	Dir    string `yaml:"dir"`
}

type BillingConfig struct {
	EnablePayAsYouGo bool            `yaml:"enable_pay_as_you_go"`
	MinTopupCNY      decimal.Decimal `yaml:"min_topup_cny"`
	CreditUSDPerCNY  decimal.Decimal `yaml:"credit_usd_per_cny"`
}

type SMTPConfig struct {
	SMTPServer     string `yaml:"SMTPServer"`
	SMTPPort       int    `yaml:"SMTPPort"`
	SMTPSSLEnabled bool   `yaml:"SMTPSSLEnabled"`
	SMTPAccount    string `yaml:"SMTPAccount"`
	SMTPFrom       string `yaml:"SMTPFrom"`
	SMTPToken      string `yaml:"SMTPToken"`
}

type EmailVerifConfig struct {
	Enable bool `yaml:"enable"`
}

type TicketsConfig struct {
	AttachmentsDir string `yaml:"attachments_dir"`
}

// LoadFromEnv 仅从环境变量加载配置（不读取任何配置文件）。
func LoadFromEnv() (Config, error) {
	if v := strings.TrimSpace(os.Getenv("REALMS_MODE")); v != "" {
		return Config{}, fmt.Errorf("REALMS_MODE 已移除（检测到 %q）；请删除该配置并使用统一模式启动", v)
	}
	for _, removed := range []struct {
		name    string
		message string
	}{
		{
			name:    "REALMS_SUB2API_BASE_URL",
			message: "请改用 REALMS_COMPACT_GATEWAY_BASE_URL",
		},
		{
			name:    "REALMS_SUB2API_GATEWAY_KEY",
			message: "请改用 REALMS_COMPACT_GATEWAY_KEY",
		},
		{
			name:    "REALMS_SUB2API_TIMEOUT_MS",
			message: "请删除该配置；compact gateway 启动期超时覆盖已移除，当前固定为 300s",
		},
		{
			name:    "REALMS_CODEX_SESSION_TTL_SECONDS",
			message: "请删除该配置；Codex session TTL 启动期覆盖已移除，当前固定为 300 秒",
		},
	} {
		if v := strings.TrimSpace(os.Getenv(removed.name)); v != "" {
			return Config{}, fmt.Errorf("%s 已移除（检测到 %q）；%s", removed.name, v, removed.message)
		}
	}
	cfg := defaultConfig()
	applyEnvOverrides(&cfg)
	return normalizeAndValidate(cfg)
}

func normalizeAndValidate(cfg Config) (Config, error) {
	mode, err := canonicalizeMode(cfg.Mode)
	if err != nil {
		return Config{}, err
	}
	cfg.Mode = mode

	if cfg.Server.Addr == "" {
		return Config{}, errors.New("server.addr 不能为空")
	}

	cfg.DB.Driver = strings.ToLower(strings.TrimSpace(cfg.DB.Driver))
	cfg.DB.DSN = strings.TrimSpace(cfg.DB.DSN)
	cfg.DB.SQLitePath = strings.TrimSpace(cfg.DB.SQLitePath)
	cfg.DB.MigrationLockName = strings.TrimSpace(cfg.DB.MigrationLockName)
	if cfg.DB.MigrationLockName == "" {
		cfg.DB.MigrationLockName = "realms.schema_migrations"
	}
	if cfg.DB.MigrationLockTimeoutSeconds < 0 {
		return Config{}, errors.New("db.migration_lock_timeout_seconds 不能为负数")
	}

	// 兼容旧配置：历史仅配置 db.dsn（无 db.driver）。
	if cfg.DB.Driver == "" {
		if cfg.DB.DSN != "" {
			cfg.DB.Driver = "mysql"
		} else {
			cfg.DB.Driver = "sqlite"
		}
	}

	switch cfg.DB.Driver {
	case "sqlite":
		if cfg.DB.SQLitePath == "" {
			cfg.DB.SQLitePath = "./data/realms.db?_busy_timeout=30000"
		}
	case "mysql":
		if cfg.DB.DSN == "" {
			return Config{}, errors.New("db.dsn 不能为空（db.driver=mysql）")
		}
	default:
		return Config{}, fmt.Errorf("db.driver 不支持：%s（仅支持 mysql/sqlite）", cfg.DB.Driver)
	}

	cfg.CompactGateway.BaseURL = strings.TrimSpace(cfg.CompactGateway.BaseURL)
	if cfg.CompactGateway.BaseURL != "" {
		compactGatewayBaseURL, err := NormalizeHTTPBaseURL(cfg.CompactGateway.BaseURL, "compact_gateway.base_url")
		if err != nil {
			return Config{}, err
		}
		cfg.CompactGateway.BaseURL = compactGatewayBaseURL
	}
	cfg.CompactGateway.GatewayKey = strings.TrimSpace(cfg.CompactGateway.GatewayKey)

	cfg.Redis.Addr = strings.TrimSpace(cfg.Redis.Addr)
	cfg.Redis.Password = strings.TrimSpace(cfg.Redis.Password)
	cfg.Redis.KeyPrefix = strings.TrimSpace(cfg.Redis.KeyPrefix)
	if cfg.Redis.KeyPrefix == "" {
		cfg.Redis.KeyPrefix = "realms"
	}
	if cfg.Redis.DB < 0 {
		return Config{}, errors.New("redis.db 不能为负数")
	}

	if cfg.Gateway.MaxRetryAttempts < 0 {
		cfg.Gateway.MaxRetryAttempts = 0
	}
	if cfg.Gateway.MaxRetryAttempts > 20 {
		cfg.Gateway.MaxRetryAttempts = 20
	}
	if cfg.Gateway.RetryBaseDelayMS < 0 {
		cfg.Gateway.RetryBaseDelayMS = 0
	}
	if cfg.Gateway.RetryMaxDelayMS < 0 {
		cfg.Gateway.RetryMaxDelayMS = 0
	}
	if cfg.Gateway.RetryMaxDelayMS < cfg.Gateway.RetryBaseDelayMS {
		cfg.Gateway.RetryMaxDelayMS = cfg.Gateway.RetryBaseDelayMS
	}
	if cfg.Gateway.MaxRetryElapsedMS < 0 {
		cfg.Gateway.MaxRetryElapsedMS = 0
	}
	if cfg.Gateway.MaxFailoverSwitches < 0 {
		cfg.Gateway.MaxFailoverSwitches = 0
	}
	if cfg.Gateway.WaitTimeoutMS <= 0 {
		cfg.Gateway.WaitTimeoutMS = 30000
	}
	if cfg.Gateway.WaitQueueExtraSlots < 0 {
		cfg.Gateway.WaitQueueExtraSlots = 0
	}

	cfg.Security.AdminAPIKey = strings.TrimSpace(cfg.Security.AdminAPIKey)
	cfg.SessionSecret = strings.TrimSpace(cfg.SessionSecret)
	cfg.Tickets.AttachmentsDir = strings.TrimSpace(cfg.Tickets.AttachmentsDir)
	if cfg.Tickets.AttachmentsDir == "" {
		cfg.Tickets.AttachmentsDir = "./data/tickets"
	}

	if cfg.ChannelTestCLIConcurrency <= 0 {
		cfg.ChannelTestCLIConcurrency = 4
	}
	if cfg.ChannelTestCLIConcurrency > 16 {
		cfg.ChannelTestCLIConcurrency = 16
	}

	cfg.AppSettingsDefaults.SiteBaseURL = strings.TrimSpace(cfg.AppSettingsDefaults.SiteBaseURL)
	siteBaseURL, err := NormalizeHTTPBaseURL(cfg.AppSettingsDefaults.SiteBaseURL, "app_settings_defaults.site_base_url")
	if err != nil {
		return Config{}, err
	}
	cfg.AppSettingsDefaults.SiteBaseURL = siteBaseURL

	cfg.AppSettingsDefaults.AdminTimeZone = strings.TrimSpace(cfg.AppSettingsDefaults.AdminTimeZone)
	if cfg.AppSettingsDefaults.AdminTimeZone != "" {
		if _, err := time.LoadLocation(cfg.AppSettingsDefaults.AdminTimeZone); err != nil {
			return Config{}, fmt.Errorf("app_settings_defaults.admin_time_zone 不合法: %w", err)
		}
	}

	cfg.Debug.ProxyLog.Dir = strings.TrimSpace(cfg.Debug.ProxyLog.Dir)
	if cfg.Debug.ProxyLog.Dir == "" {
		cfg.Debug.ProxyLog.Dir = "./out/proxy"
	}

	return cfg, nil
}

func canonicalizeMode(mode Mode) (Mode, error) {
	raw := strings.ToLower(strings.TrimSpace(string(mode)))
	switch raw {
	case "", string(ModeBusiness):
		return ModeBusiness, nil
	case string(ModePersonal):
		return "", fmt.Errorf("mode=%q 已移除；请删除该配置并使用统一模式启动", raw)
	default:
		return "", fmt.Errorf("mode 不支持：%s（统一模式下仅支持 business）", raw)
	}
}

func NormalizeHTTPBaseURL(raw string, label string) (string, error) {
	v := strings.TrimRight(strings.TrimSpace(raw), "/")
	if v == "" {
		return "", nil
	}
	u, err := url.Parse(v)
	if err != nil {
		if strings.TrimSpace(label) == "" {
			return "", fmt.Errorf("解析 base_url 失败: %w", err)
		}
		return "", fmt.Errorf("解析 %s 失败: %w", label, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		if strings.TrimSpace(label) == "" {
			return "", errors.New("base_url 仅支持 http/https")
		}
		return "", fmt.Errorf("%s 仅支持 http/https", label)
	}
	if u.Host == "" {
		if strings.TrimSpace(label) == "" {
			return "", errors.New("base_url host 不能为空")
		}
		return "", fmt.Errorf("%s host 不能为空", label)
	}
	return v, nil
}

func parseDecimalNonNeg(raw string, scale int32) (decimal.Decimal, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return decimal.Zero, errors.New("金额为空")
	}
	if strings.HasPrefix(s, "+") {
		s = strings.TrimSpace(strings.TrimPrefix(s, "+"))
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, errors.New("金额格式不合法")
	}
	if d.IsNegative() {
		return decimal.Zero, errors.New("金额不能为负数")
	}
	if d.Exponent() < -scale {
		return decimal.Zero, fmt.Errorf("最多支持 %d 位小数", scale)
	}
	return d.Truncate(scale), nil
}

func defaultConfig() Config {
	return Config{
		Env:  "dev",
		Mode: ModeBusiness,
		Server: ServerConfig{
			Addr: ":8080",
		},
		Redis: RedisConfig{
			KeyPrefix: "realms",
		},
		Gateway: GatewayConfig{
			MaxRetryAttempts:       5,
			RetryBaseDelayMS:       300,
			RetryMaxDelayMS:        3000,
			MaxRetryElapsedMS:      10000,
			MaxFailoverSwitches:    8,
			WaitTimeoutMS:          30000,
			WaitQueueExtraSlots:    20,
			EnableErrorPassthrough: true,
		},
		CompactGateway: CompactGatewayConfig{
			BaseURL:    "",
			GatewayKey: "",
		},
		Debug: DebugConfig{
			ProxyLog: ProxyLogConfig{
				Enable: false,
				Dir:    "./out/proxy",
			},
		},
		Billing: BillingConfig{
			EnablePayAsYouGo: true,
			MinTopupCNY:      decimal.NewFromInt(10),
			CreditUSDPerCNY:  decimal.NewFromInt(14).Div(decimal.NewFromInt(100)),
		},
		SMTP: SMTPConfig{
			SMTPPort: 587,
		},
		EmailVerif: EmailVerifConfig{
			Enable: false,
		},
		Tickets: TicketsConfig{
			AttachmentsDir: "./data/tickets",
		},
		ChannelTestCLIConcurrency: 4,
		AppSettingsDefaults: AppSettingsDefaultsConfig{
			AdminTimeZone: "Asia/Shanghai",
		},
		DB: DBConfig{
			SQLitePath:                  "./data/realms.db?_busy_timeout=30000",
			MigrationLockName:           "realms.schema_migrations",
			MigrationLockTimeoutSeconds: 30,
		},
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("REALMS_ENV"); v != "" {
		cfg.Env = v
	}
	if v := os.Getenv("REALMS_ADDR"); v != "" {
		cfg.Server.Addr = v
	}
	if v := os.Getenv("REALMS_DB_DRIVER"); v != "" {
		cfg.DB.Driver = v
	}
	if v := os.Getenv("REALMS_DB_DSN"); v != "" {
		cfg.DB.DSN = v
	}
	if v := os.Getenv("REALMS_SQLITE_PATH"); v != "" {
		cfg.DB.SQLitePath = v
	}
	if v := os.Getenv("REALMS_COMPACT_GATEWAY_BASE_URL"); v != "" {
		cfg.CompactGateway.BaseURL = v
	}
	if v := os.Getenv("REALMS_COMPACT_GATEWAY_KEY"); v != "" {
		cfg.CompactGateway.GatewayKey = v
	}
	if v := os.Getenv("REALMS_ADMIN_API_KEY"); v != "" {
		cfg.Security.AdminAPIKey = v
	}
	if v := os.Getenv("REALMS_SUBSCRIPTION_ORDER_WEBHOOK_SECRET"); v != "" {
		cfg.Security.SubscriptionOrderWebhookSecret = v
	}
	if v := os.Getenv("SESSION_SECRET"); v != "" {
		cfg.SessionSecret = v
	}

	if v := os.Getenv("REALMS_CHANNEL_TEST_CLI_RUNNER_URL"); v != "" {
		cfg.ChannelTestCLIRunnerURL = v
	}
}
