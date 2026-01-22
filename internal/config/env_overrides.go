package config

import (
	"os"
	"strconv"
	"time"
)

func applyCoreEnvOverrides(cfg *Config) {
	if v := os.Getenv("REALMS_ENV"); v != "" {
		cfg.Env = v
	}
	if v := os.Getenv("REALMS_SELF_MODE_ENABLE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.SelfMode.Enable = b
		}
	}
	if v := os.Getenv("REALMS_DB_DSN"); v != "" {
		cfg.DB.DSN = v
	}
}

func applyServerEnvOverrides(cfg *Config) {
	if v := os.Getenv("REALMS_ADDR"); v != "" {
		cfg.Server.Addr = v
	}
	if v := os.Getenv("REALMS_PUBLIC_BASE_URL"); v != "" {
		cfg.Server.PublicBaseURL = v
	}
	if v := os.Getenv("REALMS_SERVER_READ_HEADER_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.ReadHeaderTimeout = d
		}
	}
	if v := os.Getenv("REALMS_SERVER_READ_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.ReadTimeout = d
		}
	}
	if v := os.Getenv("REALMS_SERVER_WRITE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.WriteTimeout = d
		}
	}
	if v := os.Getenv("REALMS_SERVER_IDLE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.IdleTimeout = d
		}
	}
}

func applySecurityEnvOverrides(cfg *Config) {
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
}

func applyLimitsEnvOverrides(cfg *Config) {
	if v := os.Getenv("REALMS_LIMITS_MAX_BODY_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Limits.MaxBodyBytes = n
		}
	}
	if v := os.Getenv("REALMS_LIMITS_MAX_REQUEST_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Limits.MaxRequestDuration = d
		}
	}
	if v := os.Getenv("REALMS_LIMITS_MAX_STREAM_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Limits.MaxStreamDuration = d
		}
	}
	if v := os.Getenv("REALMS_LIMITS_MAX_INFLIGHT_PER_TOKEN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Limits.MaxInflightPerToken = n
		}
	}
	if v := os.Getenv("REALMS_LIMITS_MAX_SSE_CONNECTIONS_PER_TOKEN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Limits.MaxSSEConnectionsPerToken = n
		}
	}
	if v := os.Getenv("REALMS_LIMITS_MAX_INFLIGHT_PER_CREDENTIAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Limits.MaxInflightPerCredential = n
		}
	}
	if v := os.Getenv("REALMS_LIMITS_DEFAULT_MAX_OUTPUT_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Limits.DefaultMaxOutputTokens = n
		}
	}
	if v := os.Getenv("REALMS_LIMITS_STREAM_IDLE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Limits.StreamIdleTimeout = d
		}
	}
	if v := os.Getenv("REALMS_LIMITS_SSE_PING_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Limits.SSEPingInterval = d
		}
	}
	if v := os.Getenv("REALMS_LIMITS_SSE_MAX_EVENT_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Limits.SSEMaxEventBytes = n
		}
	}
	if v := os.Getenv("REALMS_LIMITS_UPSTREAM_REQUEST_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Limits.UpstreamRequestTimeout = d
		}
	}
	if v := os.Getenv("REALMS_LIMITS_UPSTREAM_DIAL_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Limits.UpstreamDialTimeout = d
		}
	}
	if v := os.Getenv("REALMS_LIMITS_UPSTREAM_TLS_HANDSHAKE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Limits.UpstreamTLSHandshakeTimeout = d
		}
	}
	if v := os.Getenv("REALMS_LIMITS_UPSTREAM_RESPONSE_HEADER_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Limits.UpstreamResponseHeaderTimout = d
		}
	}
}

func applyDebugEnvOverrides(cfg *Config) {
	if v := os.Getenv("REALMS_DEBUG_PROXY_LOG_ENABLE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Debug.ProxyLog.Enable = b
		}
	}
	if v := os.Getenv("REALMS_DEBUG_PROXY_LOG_DIR"); v != "" {
		cfg.Debug.ProxyLog.Dir = v
	}
	if v := os.Getenv("REALMS_DEBUG_PROXY_LOG_MAX_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Debug.ProxyLog.MaxBytes = n
		}
	}
	if v := os.Getenv("REALMS_DEBUG_PROXY_LOG_MAX_FILES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Debug.ProxyLog.MaxFiles = n
		}
	}
}

func applyBillingEnvOverrides(cfg *Config) {
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
}

func applyPaymentEnvOverrides(cfg *Config) {
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
}

func applySMTPEnvOverrides(cfg *Config) {
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
}

func applyEmailVerificationEnvOverrides(cfg *Config) {
	if v := os.Getenv("REALMS_EMAIL_VERIFICATION_ENABLE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.EmailVerif.Enable = b
		}
	}
}

func applyTicketsEnvOverrides(cfg *Config) {
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
}

func applyCodexOAuthEnvOverrides(cfg *Config) {
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

func applyAppSettingsDefaultsEnvOverrides(cfg *Config) {
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
}
