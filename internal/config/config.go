// Package config 负责读取并合并服务配置（环境变量为主，可选读取 YAML 配置），避免在业务代码里散落解析逻辑。
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type Config struct {
	Env        string           `yaml:"env"`
	SelfMode   SelfModeConfig   `yaml:"self_mode"`
	Server     ServerConfig     `yaml:"server"`
	DB         DBConfig         `yaml:"db"`
	Security   SecurityConfig   `yaml:"security"`
	Debug      DebugConfig      `yaml:"debug"`
	Billing    BillingConfig    `yaml:"billing"`
	SMTP       SMTPConfig       `yaml:"smtp"`
	EmailVerif EmailVerifConfig `yaml:"email_verification"`
	Tickets    TicketsConfig    `yaml:"tickets"`

	// AppSettingsDefaults 提供管理后台“系统设置”（app_settings）的配置文件默认值。
	// 仅当数据库未配置对应 app_settings 键时才会生效（app_settings 仍优先）。
	AppSettingsDefaults AppSettingsDefaultsConfig `yaml:"app_settings_defaults"`
}

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

type SelfModeConfig struct {
	// Enable 开启后进入“自用模式”：会禁用计费/支付/工单等不需要的功能域，
	// 并放宽数据面配额策略（不再要求订阅已激活）。
	Enable bool `yaml:"enable"`
}

type ServerConfig struct {
	Addr          string `yaml:"addr"`
	PublicBaseURL string `yaml:"public_base_url"`
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
}

type SecurityConfig struct {
	AllowOpenRegistration bool `yaml:"allow_open_registration"`
	DisableSecureCookies  bool `yaml:"disable_secure_cookies"`

	TrustProxyHeaders bool     `yaml:"trust_proxy_headers"`
	TrustedProxyCIDRs []string `yaml:"trusted_proxy_cidrs"`

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
	cfg := defaultConfig()
	applyEnvOverrides(&cfg)
	return normalizeAndValidate(cfg)
}

func normalizeAndValidate(cfg Config) (Config, error) {
	publicBaseURL, err := NormalizeHTTPBaseURL(cfg.Server.PublicBaseURL, "server.public_base_url")
	if err != nil {
		return Config{}, err
	}
	cfg.Server.PublicBaseURL = publicBaseURL
	if cfg.Server.Addr == "" {
		return Config{}, errors.New("server.addr 不能为空")
	}

	cfg.DB.Driver = strings.ToLower(strings.TrimSpace(cfg.DB.Driver))
	cfg.DB.DSN = strings.TrimSpace(cfg.DB.DSN)
	cfg.DB.SQLitePath = strings.TrimSpace(cfg.DB.SQLitePath)

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
	cfg.Tickets.AttachmentsDir = strings.TrimSpace(cfg.Tickets.AttachmentsDir)
	if cfg.Tickets.AttachmentsDir == "" {
		cfg.Tickets.AttachmentsDir = "./data/tickets"
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
		Env: "dev",
		SelfMode: SelfModeConfig{
			Enable: false,
		},
		Server: ServerConfig{
			Addr: ":8080",
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
		Security: SecurityConfig{
			AllowOpenRegistration: true,
			TrustProxyHeaders:     false,
		},
		DB: DBConfig{
			SQLitePath: "./data/realms.db?_busy_timeout=30000",
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
		AppSettingsDefaults: AppSettingsDefaultsConfig{
			AdminTimeZone: "Asia/Shanghai",
		},
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("REALMS_ENV"); v != "" {
		cfg.Env = v
	}
	if v := os.Getenv("REALMS_SELF_MODE_ENABLE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.SelfMode.Enable = b
		}
	}
	if v := os.Getenv("REALMS_ADDR"); v != "" {
		cfg.Server.Addr = v
	}
	if v := os.Getenv("REALMS_PUBLIC_BASE_URL"); v != "" {
		cfg.Server.PublicBaseURL = v
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
	if v := os.Getenv("REALMS_ALLOW_OPEN_REGISTRATION"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Security.AllowOpenRegistration = b
		}
	}
	if v := os.Getenv("REALMS_DISABLE_SECURE_COOKIES"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Security.DisableSecureCookies = b
		}
	}
	if v := os.Getenv("REALMS_TRUST_PROXY_HEADERS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Security.TrustProxyHeaders = b
		}
	}
	if v := os.Getenv("REALMS_TRUSTED_PROXY_CIDRS"); v != "" {
		cfg.Security.TrustedProxyCIDRs = splitCSV(v)
	}
	if v := os.Getenv("REALMS_SUBSCRIPTION_ORDER_WEBHOOK_SECRET"); v != "" {
		cfg.Security.SubscriptionOrderWebhookSecret = v
	}

	if v := os.Getenv("REALMS_DEBUG_PROXY_LOG_ENABLE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Debug.ProxyLog.Enable = b
		}
	}
	if v := os.Getenv("REALMS_DEBUG_PROXY_LOG_DIR"); v != "" {
		cfg.Debug.ProxyLog.Dir = v
	}

	if v := os.Getenv("REALMS_BILLING_ENABLE_PAY_AS_YOU_GO"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Billing.EnablePayAsYouGo = b
		}
	}
	if v := os.Getenv("REALMS_BILLING_MIN_TOPUP_CNY"); v != "" {
		if d, err := parseDecimalNonNeg(v, 2); err == nil {
			cfg.Billing.MinTopupCNY = d
		}
	}
	if v := os.Getenv("REALMS_BILLING_CREDIT_USD_PER_CNY"); v != "" {
		if d, err := parseDecimalNonNeg(v, 6); err == nil {
			cfg.Billing.CreditUSDPerCNY = d
		}
	}

	if v := os.Getenv("REALMS_SMTP_SERVER"); v != "" {
		cfg.SMTP.SMTPServer = v
	}
	if v := os.Getenv("REALMS_SMTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.SMTP.SMTPPort = n
		}
	}
	if v := os.Getenv("REALMS_SMTP_SSL_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.SMTP.SMTPSSLEnabled = b
		}
	}
	if v := os.Getenv("REALMS_SMTP_ACCOUNT"); v != "" {
		cfg.SMTP.SMTPAccount = v
	}
	if v := os.Getenv("REALMS_SMTP_FROM"); v != "" {
		cfg.SMTP.SMTPFrom = v
	}
	if v := os.Getenv("REALMS_SMTP_TOKEN"); v != "" {
		cfg.SMTP.SMTPToken = v
	}
	if v := os.Getenv("REALMS_EMAIL_VERIFICATION_ENABLE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.EmailVerif.Enable = b
		}
	}

	if v := os.Getenv("REALMS_APP_SETTINGS_DEFAULTS_SITE_BASE_URL"); v != "" {
		cfg.AppSettingsDefaults.SiteBaseURL = v
	}
	if v := os.Getenv("REALMS_APP_SETTINGS_DEFAULTS_ADMIN_TIME_ZONE"); v != "" {
		cfg.AppSettingsDefaults.AdminTimeZone = v
	}
	if v := os.Getenv("REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_WEB_ANNOUNCEMENTS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AppSettingsDefaults.FeatureDisableWebAnnouncements = b
		}
	}
	if v := os.Getenv("REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_WEB_TOKENS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AppSettingsDefaults.FeatureDisableWebTokens = b
		}
	}
	if v := os.Getenv("REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_WEB_USAGE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AppSettingsDefaults.FeatureDisableWebUsage = b
		}
	}
	if v := os.Getenv("REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_MODELS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AppSettingsDefaults.FeatureDisableModels = b
		}
	}
	if v := os.Getenv("REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_BILLING"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AppSettingsDefaults.FeatureDisableBilling = b
		}
	}
	if v := os.Getenv("REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_TICKETS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AppSettingsDefaults.FeatureDisableTickets = b
		}
	}
	if v := os.Getenv("REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_ADMIN_CHANNELS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AppSettingsDefaults.FeatureDisableAdminChannels = b
		}
	}
	if v := os.Getenv("REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_ADMIN_CHANNEL_GROUPS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AppSettingsDefaults.FeatureDisableAdminChannelGroups = b
		}
	}
	if v := os.Getenv("REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_ADMIN_USERS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AppSettingsDefaults.FeatureDisableAdminUsers = b
		}
	}
	if v := os.Getenv("REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_ADMIN_USAGE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AppSettingsDefaults.FeatureDisableAdminUsage = b
		}
	}
	if v := os.Getenv("REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_ADMIN_ANNOUNCEMENTS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AppSettingsDefaults.FeatureDisableAdminAnnouncements = b
		}
	}

	if v := os.Getenv("REALMS_TICKETS_ATTACHMENTS_DIR"); v != "" {
		cfg.Tickets.AttachmentsDir = v
	}
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}
