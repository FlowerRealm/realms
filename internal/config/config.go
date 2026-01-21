// Package config 负责读取并合并服务配置（文件 + 环境变量覆盖），避免在业务代码里散落解析逻辑。
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
	"gopkg.in/yaml.v3"
)

type Config struct {
	Env        string           `yaml:"env"`
	SelfMode   SelfModeConfig   `yaml:"self_mode"`
	Server     ServerConfig     `yaml:"server"`
	DB         DBConfig         `yaml:"db"`
	Security   SecurityConfig   `yaml:"security"`
	Limits     LimitsConfig     `yaml:"limits"`
	Debug      DebugConfig      `yaml:"debug"`
	Billing    BillingConfig    `yaml:"billing"`
	Payment    PaymentConfig    `yaml:"payment"`
	SMTP       SMTPConfig       `yaml:"smtp"`
	EmailVerif EmailVerifConfig `yaml:"email_verification"`
	CodexOAuth CodexOAuthConfig `yaml:"codex_oauth"`
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
	Addr              string        `yaml:"addr"`
	PublicBaseURL     string        `yaml:"public_base_url"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
}

type DBConfig struct {
	// Driver 支持 mysql/sqlite；为空时会根据 dsn 自动推断（兼容旧配置）。
	// - 当 dsn 非空且 driver 为空：推断为 mysql
	// - 其他情况默认 sqlite
	Driver string `yaml:"driver"`
	// DSN 仅用于 MySQL（示例：user:pass@tcp(127.0.0.1:3306)/realms?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci）
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

type LimitsConfig struct {
	MaxBodyBytes       int64         `yaml:"max_body_bytes"`
	MaxRequestDuration time.Duration `yaml:"max_request_duration"`
	// MaxStreamDuration 仅用于流式（SSE）请求的“最大总时长”上限；0 表示不限制（由 idle 超时兜底）。
	MaxStreamDuration time.Duration `yaml:"max_stream_duration"`

	MaxInflightPerToken       int `yaml:"max_inflight_per_token"`
	MaxSSEConnectionsPerToken int `yaml:"max_sse_connections_per_token"`
	MaxInflightPerCredential  int `yaml:"max_inflight_per_credential"`
	DefaultMaxOutputTokens    int `yaml:"default_max_output_tokens"`

	// StreamIdleTimeout 控制上游 SSE 在长时间无输出时的中断阈值；0 表示不启用 idle 超时。
	StreamIdleTimeout time.Duration `yaml:"stream_idle_timeout"`
	// SSEPingInterval 控制向下游发送 SSE ping 的间隔；0 表示不发送 ping。
	SSEPingInterval time.Duration `yaml:"sse_ping_interval"`
	// SSEMaxEventBytes 控制 SSE 单行最大长度（Scanner 上限）；过小会导致大事件行断流。
	SSEMaxEventBytes int64 `yaml:"sse_max_event_bytes"`

	UpstreamRequestTimeout       time.Duration `yaml:"upstream_request_timeout"`
	UpstreamDialTimeout          time.Duration `yaml:"upstream_dial_timeout"`
	UpstreamTLSHandshakeTimeout  time.Duration `yaml:"upstream_tls_handshake_timeout"`
	UpstreamResponseHeaderTimout time.Duration `yaml:"upstream_response_header_timeout"`
}

type DebugConfig struct {
	ProxyLog ProxyLogConfig `yaml:"proxy_log"`
}

type ProxyLogConfig struct {
	Enable   bool   `yaml:"enable"`
	Dir      string `yaml:"dir"`
	MaxBytes int64  `yaml:"max_bytes"`
	MaxFiles int    `yaml:"max_files"`
}

type BillingConfig struct {
	EnablePayAsYouGo bool            `yaml:"enable_pay_as_you_go"`
	MinTopupCNY      decimal.Decimal `yaml:"min_topup_cny"`
	CreditUSDPerCNY  decimal.Decimal `yaml:"credit_usd_per_cny"`
}

type PaymentConfig struct {
	EPay   PaymentEPayConfig   `yaml:"epay"`
	Stripe PaymentStripeConfig `yaml:"stripe"`
}

type PaymentEPayConfig struct {
	Enable    bool   `yaml:"enable"`
	Gateway   string `yaml:"gateway"`
	PartnerID string `yaml:"partner_id"`
	Key       string `yaml:"key"`
}

type PaymentStripeConfig struct {
	Enable        bool   `yaml:"enable"`
	SecretKey     string `yaml:"secret_key"`
	WebhookSecret string `yaml:"webhook_secret"`
	Currency      string `yaml:"currency"`
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

type CodexOAuthConfig struct {
	Enable bool `yaml:"enable"`

	ClientID     string `yaml:"client_id"`
	AuthorizeURL string `yaml:"authorize_url"`
	TokenURL     string `yaml:"token_url"`

	// HTTPTimeout 是 Codex OAuth 相关 HTTP 请求（token exchange / quota refresh 等）的总超时（含 TLS 握手）。
	HTTPTimeout time.Duration `yaml:"http_timeout"`
	// TLSHandshakeTimeout 是 TLS 握手超时；网络较慢或代理较慢时可适当调大以避免握手超时失败。
	TLSHandshakeTimeout time.Duration `yaml:"tls_handshake_timeout"`

	// RequestPassthrough 表示 Codex OAuth 请求是否“直通上游”（不改写 URL path 与 request body）。
	// 默认 true：保持与 Codex CLI 的请求形态一致，把兼容性与校验交给上游处理。
	RequestPassthrough bool `yaml:"request_passthrough"`

	CallbackListenAddr string `yaml:"callback_listen_addr"`
	RedirectURI        string `yaml:"redirect_uri"`

	Scope  string `yaml:"scope"`
	Prompt string `yaml:"prompt"`
}

type TicketsConfig struct {
	AttachmentsDir string        `yaml:"attachments_dir"`
	AttachmentTTL  time.Duration `yaml:"attachment_ttl"`
	MaxUploadBytes int64         `yaml:"max_upload_bytes"`
}

func Load(path string) (Config, error) {
	return load(path, true)
}

// LoadFromFile 与 Load 的区别是：不会应用环境变量覆盖（env overrides）。
// 用途：在“写回配置文件”前做严格校验，避免环境变量掩盖配置文件本身的问题。
func LoadFromFile(path string) (Config, error) {
	return load(path, false)
}

func load(path string, applyEnv bool) (Config, error) {
	cfg := defaultConfig()
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("读取配置文件: %w", err)
	}
	if len(b) > 0 {
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return Config{}, fmt.Errorf("解析 YAML: %w", err)
		}
	}
	if applyEnv {
		applyEnvOverrides(&cfg)
	}
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
	if cfg.Tickets.AttachmentTTL <= 0 {
		cfg.Tickets.AttachmentTTL = 7 * 24 * time.Hour
	}
	if cfg.Tickets.MaxUploadBytes <= 0 {
		cfg.Tickets.MaxUploadBytes = 100 << 20
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
	if cfg.Debug.ProxyLog.MaxBytes <= 0 {
		cfg.Debug.ProxyLog.MaxBytes = 128 << 10
	}
	if cfg.Debug.ProxyLog.MaxBytes > 10<<20 {
		cfg.Debug.ProxyLog.MaxBytes = 10 << 20
	}
	if cfg.Debug.ProxyLog.MaxFiles <= 0 {
		cfg.Debug.ProxyLog.MaxFiles = 200
	}
	if cfg.Debug.ProxyLog.MaxFiles > 2000 {
		cfg.Debug.ProxyLog.MaxFiles = 2000
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
			Addr:              ":8080",
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      0,
			IdleTimeout:       2 * time.Minute,
		},
		Limits: LimitsConfig{
			MaxBodyBytes:                 4 << 20,
			MaxRequestDuration:           2 * time.Minute,
			MaxStreamDuration:            30 * time.Minute,
			MaxInflightPerToken:          8,
			MaxSSEConnectionsPerToken:    4,
			MaxInflightPerCredential:     16,
			DefaultMaxOutputTokens:       1024,
			StreamIdleTimeout:            2 * time.Minute,
			SSEPingInterval:              0,
			SSEMaxEventBytes:             4 << 20,
			UpstreamRequestTimeout:       2 * time.Minute,
			UpstreamDialTimeout:          10 * time.Second,
			UpstreamTLSHandshakeTimeout:  10 * time.Second,
			UpstreamResponseHeaderTimout: 30 * time.Second,
		},
		Debug: DebugConfig{
			ProxyLog: ProxyLogConfig{
				Enable:   false,
				Dir:      "./out/proxy",
				MaxBytes: 128 << 10,
				MaxFiles: 200,
			},
		},
		Billing: BillingConfig{
			EnablePayAsYouGo: true,
			MinTopupCNY:      decimal.NewFromInt(10),
			CreditUSDPerCNY:  decimal.NewFromInt(14).Div(decimal.NewFromInt(100)),
		},
		Payment: PaymentConfig{
			EPay: PaymentEPayConfig{
				Enable: false,
			},
			Stripe: PaymentStripeConfig{
				Enable:   false,
				Currency: "cny",
			},
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
		CodexOAuth: CodexOAuthConfig{
			Enable:              true,
			ClientID:            "app_EMoamEEZ73f0CkXaXp7hrann",
			AuthorizeURL:        "https://auth.openai.com/oauth/authorize",
			TokenURL:            "https://auth.openai.com/oauth/token",
			HTTPTimeout:         2 * time.Minute,
			TLSHandshakeTimeout: 60 * time.Second,
			RequestPassthrough:  true,
			CallbackListenAddr:  "",
			RedirectURI:         "http://localhost:1455/auth/callback",
			Scope:               "openid email profile offline_access",
			Prompt:              "login",
		},
		Tickets: TicketsConfig{
			AttachmentsDir: "./data/tickets",
			AttachmentTTL:  7 * 24 * time.Hour,
			MaxUploadBytes: 100 << 20, // 100MB
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

	if v := os.Getenv("REALMS_PAYMENT_EPAY_ENABLE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Payment.EPay.Enable = b
		}
	}
	if v := os.Getenv("REALMS_PAYMENT_EPAY_GATEWAY"); v != "" {
		cfg.Payment.EPay.Gateway = v
	}
	if v := os.Getenv("REALMS_PAYMENT_EPAY_PARTNER_ID"); v != "" {
		cfg.Payment.EPay.PartnerID = v
	}
	if v := os.Getenv("REALMS_PAYMENT_EPAY_KEY"); v != "" {
		cfg.Payment.EPay.Key = v
	}

	if v := os.Getenv("REALMS_PAYMENT_STRIPE_ENABLE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Payment.Stripe.Enable = b
		}
	}
	if v := os.Getenv("REALMS_PAYMENT_STRIPE_SECRET_KEY"); v != "" {
		cfg.Payment.Stripe.SecretKey = v
	}
	if v := os.Getenv("REALMS_PAYMENT_STRIPE_WEBHOOK_SECRET"); v != "" {
		cfg.Payment.Stripe.WebhookSecret = v
	}
	if v := os.Getenv("REALMS_PAYMENT_STRIPE_CURRENCY"); v != "" {
		cfg.Payment.Stripe.Currency = v
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

	if v := os.Getenv("REALMS_TICKETS_ATTACHMENTS_DIR"); v != "" {
		cfg.Tickets.AttachmentsDir = v
	}
	if v := os.Getenv("REALMS_TICKETS_ATTACHMENT_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Tickets.AttachmentTTL = d
		}
	}
	if v := os.Getenv("REALMS_TICKETS_MAX_UPLOAD_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Tickets.MaxUploadBytes = n
		}
	}

	if v := os.Getenv("REALMS_CODEX_OAUTH_ENABLE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.CodexOAuth.Enable = b
		}
	}
	if v := os.Getenv("REALMS_CODEX_OAUTH_CLIENT_ID"); v != "" {
		cfg.CodexOAuth.ClientID = v
	}
	if v := os.Getenv("REALMS_CODEX_OAUTH_AUTHORIZE_URL"); v != "" {
		cfg.CodexOAuth.AuthorizeURL = v
	}
	if v := os.Getenv("REALMS_CODEX_OAUTH_TOKEN_URL"); v != "" {
		cfg.CodexOAuth.TokenURL = v
	}
	if v := os.Getenv("REALMS_CODEX_OAUTH_HTTP_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.CodexOAuth.HTTPTimeout = d
		}
	}
	if v := os.Getenv("REALMS_CODEX_OAUTH_TLS_HANDSHAKE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.CodexOAuth.TLSHandshakeTimeout = d
		}
	}
	if v := os.Getenv("REALMS_CODEX_OAUTH_REQUEST_PASSTHROUGH"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.CodexOAuth.RequestPassthrough = b
		}
	}
	if v := os.Getenv("REALMS_CODEX_OAUTH_CALLBACK_LISTEN_ADDR"); v != "" {
		cfg.CodexOAuth.CallbackListenAddr = v
	}
	if v := os.Getenv("REALMS_CODEX_OAUTH_REDIRECT_URI"); v != "" {
		cfg.CodexOAuth.RedirectURI = v
	}
	if v := os.Getenv("REALMS_CODEX_OAUTH_SCOPE"); v != "" {
		cfg.CodexOAuth.Scope = v
	}
	if v := os.Getenv("REALMS_CODEX_OAUTH_PROMPT"); v != "" {
		cfg.CodexOAuth.Prompt = v
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
