package router

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/config"
	"realms/internal/store"
)

type featureBanItemView struct {
	Key              string `json:"key"`
	Label            string `json:"label"`
	Hint             string `json:"hint"`
	Disabled         bool   `json:"disabled"`
	Override         bool   `json:"override"`
	Editable         bool   `json:"editable"`
	ForcedBySelfMode bool   `json:"forced_by_self_mode"`
	ForcedByBuild    bool   `json:"forced_by_build"`
}

type featureBanGroupView struct {
	Title string               `json:"title"`
	Items []featureBanItemView `json:"items"`
}

var featureBanKeys = []string{
	store.SettingFeatureDisableWebAnnouncements,
	store.SettingFeatureDisableWebTokens,
	store.SettingFeatureDisableWebUsage,
	store.SettingFeatureDisableModels,
	store.SettingFeatureDisableBilling,
	store.SettingFeatureDisableTickets,
	store.SettingFeatureDisableAdminChannels,
	store.SettingFeatureDisableAdminChannelGroups,
	store.SettingFeatureDisableAdminUsers,
	store.SettingFeatureDisableAdminUsage,
	store.SettingFeatureDisableAdminAnnouncements,
}

func featureBanGroups(selfMode bool, fs store.FeatureState) []featureBanGroupView {
	b := func(key, label, hint string, disabled, forcedBySelfMode, forcedByBuild bool) featureBanItemView {
		forced := forcedBySelfMode || forcedByBuild
		return featureBanItemView{
			Key:              key,
			Label:            label,
			Hint:             hint,
			Disabled:         disabled,
			Override:         disabled && !forced,
			Editable:         !forced,
			ForcedBySelfMode: forcedBySelfMode,
			ForcedByBuild:    forcedByBuild,
		}
	}

	return []featureBanGroupView{
		{
			Title: "用户界面",
			Items: []featureBanItemView{
				b(store.SettingFeatureDisableWebAnnouncements, "公告（Web）", "隐藏侧边栏入口，并对 /announcements* 返回 404。", fs.WebAnnouncementsDisabled, false, false),
				b(store.SettingFeatureDisableWebTokens, "API 令牌（Web）", "隐藏侧边栏入口，并对 /tokens* 返回 404。", fs.WebTokensDisabled, false, false),
				b(store.SettingFeatureDisableWebUsage, "用量统计（Web）", "隐藏侧边栏入口，并对 /usage、/api/usage/* 返回 404。", fs.WebUsageDisabled, false, false),
				b(store.SettingFeatureDisableModels, "模型（全禁）", "隐藏入口，并对 /models、/admin/models*、/v1/models 返回 404；同时数据面进入模型穿透（model passthrough）。", fs.ModelsDisabled, false, false),
			},
		},
		{
			Title: "计费与支付",
			Items: []featureBanItemView{
				b(store.SettingFeatureDisableBilling, "订阅/充值/支付", "隐藏入口，并对 /subscription、/topup、/pay、/admin/subscriptions|orders|payment-channels 及支付回调返回 404；同时数据面进入 free mode（不校验订阅/余额）。", fs.BillingDisabled, selfMode, false),
			},
		},
		{
			Title: "工单",
			Items: []featureBanItemView{
				b(store.SettingFeatureDisableTickets, "工单", "隐藏入口，并对 /tickets*、/admin/tickets* 返回 404。", fs.TicketsDisabled, selfMode, false),
			},
		},
		{
				Title: "管理后台",
				Items: []featureBanItemView{
					b(store.SettingFeatureDisableAdminChannels, "上游渠道", "隐藏入口，并对 /admin/channels* 等返回 404。", fs.AdminChannelsDisabled, false, false),
					b(store.SettingFeatureDisableAdminChannelGroups, "渠道组", "隐藏入口，并对 /admin/channel-groups* 返回 404。", fs.AdminChannelGroupsDisabled, false, false),
					b(store.SettingFeatureDisableAdminUsers, "用户管理", "隐藏入口，并对 /admin/users* 返回 404。", fs.AdminUsersDisabled, false, false),
					b(store.SettingFeatureDisableAdminUsage, "用量统计（管理后台）", "隐藏入口，并对 /admin/usage 返回 404。", fs.AdminUsageDisabled, false, false),
					b(store.SettingFeatureDisableAdminAnnouncements, "公告（管理后台）", "隐藏入口，并对 /admin/announcements* 返回 404。", fs.AdminAnnouncementsDisabled, false, false),
				},
			},
	}
}

var startupConfigKeys = []string{
	"REALMS_ENV",
	"REALMS_DB_DSN",
	"REALMS_DB_DRIVER",
	"REALMS_SQLITE_PATH",
	"REALMS_ADDR",
	"REALMS_PUBLIC_BASE_URL",
	"SESSION_SECRET",
	"FRONTEND_DIST_DIR",
	"FRONTEND_BASE_URL",
	"REALMS_ALLOW_OPEN_REGISTRATION",
	"REALMS_DISABLE_SECURE_COOKIES",
	"REALMS_TRUST_PROXY_HEADERS",
	"REALMS_TRUSTED_PROXY_CIDRS",
	"REALMS_SUBSCRIPTION_ORDER_WEBHOOK_SECRET",
	"REALMS_DEBUG_PROXY_LOG_ENABLE",
	"REALMS_DEBUG_PROXY_LOG_DIR",
	"REALMS_BILLING_ENABLE_PAY_AS_YOU_GO",
	"REALMS_BILLING_MIN_TOPUP_CNY",
	"REALMS_BILLING_CREDIT_USD_PER_CNY",
	"REALMS_SMTP_SERVER",
	"REALMS_SMTP_PORT",
	"REALMS_SMTP_SSL_ENABLED",
	"REALMS_SMTP_ACCOUNT",
	"REALMS_SMTP_FROM",
	"REALMS_SMTP_TOKEN",
	"REALMS_EMAIL_VERIFICATION_ENABLE",
	"REALMS_TICKETS_ATTACHMENTS_DIR",
	"REALMS_SELF_MODE_ENABLE",
	"REALMS_APP_SETTINGS_DEFAULTS_SITE_BASE_URL",
	"REALMS_APP_SETTINGS_DEFAULTS_ADMIN_TIME_ZONE",
	"REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_WEB_ANNOUNCEMENTS",
	"REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_WEB_TOKENS",
	"REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_WEB_USAGE",
	"REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_MODELS",
	"REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_BILLING",
	"REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_TICKETS",
	"REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_ADMIN_CHANNELS",
	"REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_ADMIN_CHANNEL_GROUPS",
	"REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_ADMIN_USERS",
	"REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_ADMIN_USAGE",
	"REALMS_APP_SETTINGS_DEFAULTS_FEATURE_DISABLE_ADMIN_ANNOUNCEMENTS",
	"REALMS_BUILD_TAGS",
}

type adminSettingsResponse struct {
	SelfMode          bool                  `json:"self_mode"`
	Features          store.FeatureState    `json:"features"`
	FeatureBanGroups  []featureBanGroupView `json:"feature_ban_groups"`
	StartupConfigKeys []string              `json:"startup_config_keys"`

	SiteBaseURL          string `json:"site_base_url"`
	SiteBaseURLOverride  bool   `json:"site_base_url_override"`
	SiteBaseURLEffective string `json:"site_base_url_effective"`
	SiteBaseURLInvalid   bool   `json:"site_base_url_invalid"`

	AdminTimeZone          string `json:"admin_time_zone"`
	AdminTimeZoneOverride  bool   `json:"admin_time_zone_override"`
	AdminTimeZoneEffective string `json:"admin_time_zone_effective"`
	AdminTimeZoneInvalid   bool   `json:"admin_time_zone_invalid"`

	EmailVerificationEnabled  bool `json:"email_verification_enabled"`
	EmailVerificationOverride bool `json:"email_verification_override"`

	SMTPServer             string `json:"smtp_server"`
	SMTPServerOverride     bool   `json:"smtp_server_override"`
	SMTPPort               int    `json:"smtp_port"`
	SMTPPortOverride       bool   `json:"smtp_port_override"`
	SMTPSSLEnabled         bool   `json:"smtp_ssl_enabled"`
	SMTPSSLEnabledOverride bool   `json:"smtp_ssl_enabled_override"`
	SMTPAccount            string `json:"smtp_account"`
	SMTPAccountOverride    bool   `json:"smtp_account_override"`
	SMTPFrom               string `json:"smtp_from"`
	SMTPFromOverride       bool   `json:"smtp_from_override"`
	SMTPTokenSet           bool   `json:"smtp_token_set"`
	SMTPTokenOverride      bool   `json:"smtp_token_override"`

	BillingEnablePayAsYouGo             bool   `json:"billing_enable_pay_as_you_go"`
	BillingEnablePayAsYouGoOverride     bool   `json:"billing_enable_pay_as_you_go_override"`
	BillingMinTopupCNY                  string `json:"billing_min_topup_cny"`
	BillingMinTopupCNYOverride          bool   `json:"billing_min_topup_cny_override"`
	BillingCreditUSDPerCNY              string `json:"billing_credit_usd_per_cny"`
	BillingCreditUSDPerCNYOverride      bool   `json:"billing_credit_usd_per_cny_override"`
	BillingPaygoPriceMultiplier         string `json:"billing_paygo_price_multiplier"`
	BillingPaygoPriceMultiplierOverride bool   `json:"billing_paygo_price_multiplier_override"`
}

type adminSettingsUpdateRequest struct {
	SiteBaseURL   string `json:"site_base_url"`
	AdminTimeZone string `json:"admin_time_zone"`

	EmailVerificationEnabled bool `json:"email_verification_enable"`

	SMTPServer     string `json:"smtp_server"`
	SMTPPort       int    `json:"smtp_port"`
	SMTPSSLEnabled bool   `json:"smtp_ssl_enabled"`
	SMTPAccount    string `json:"smtp_account"`
	SMTPFrom       string `json:"smtp_from"`
	SMTPToken      string `json:"smtp_token"`

	BillingEnablePayAsYouGo     bool   `json:"billing_enable_pay_as_you_go"`
	BillingMinTopupCNY          string `json:"billing_min_topup_cny"`
	BillingCreditUSDPerCNY      string `json:"billing_credit_usd_per_cny"`
	BillingPaygoPriceMultiplier string `json:"billing_paygo_price_multiplier"`

	FeatureEnabled map[string]bool `json:"feature_enabled"`
}

func setAdminSettingsAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/settings", adminSettingsGetHandler(opts))
	r.PUT("/settings", adminSettingsUpdateHandler(opts))
	r.POST("/settings/reset", adminSettingsResetHandler(opts))
}

func adminSettingsGetHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		ctx := c.Request.Context()

		fs := opts.Store.FeatureStateEffective(ctx, opts.SelfMode)

		siteBaseURL := strings.TrimSpace(opts.PublicBaseURLDefault)
		if siteBaseURL == "" {
			siteBaseURL = strings.TrimRight(strings.TrimSpace(opts.FrontendBaseURL), "/")
		}
		siteBaseURLOK := false
		siteBaseURLInvalid := false
		siteBaseURLRaw, ok, err := opts.Store.GetStringAppSetting(ctx, store.SettingSiteBaseURL)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}
		if ok {
			siteBaseURLOK = true
			siteBaseURL = siteBaseURLRaw
			if normalized, err := config.NormalizeHTTPBaseURL(siteBaseURLRaw, "site_base_url"); err == nil {
				siteBaseURL = normalized
			} else {
				siteBaseURLInvalid = true
			}
		}

		adminTZ := strings.TrimSpace(opts.AdminTimeZoneDefault)
		if adminTZ == "" {
			adminTZ = defaultAdminTimeZone
		}
		adminTZOverride := false
		adminTZInvalid := false
		adminTZRaw, tzOK, err := opts.Store.GetStringAppSetting(ctx, store.SettingAdminTimeZone)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}
		if tzOK && strings.TrimSpace(adminTZRaw) != "" {
			adminTZOverride = true
			adminTZ = normalizeAdminTimeZoneName(adminTZRaw)
			if adminTZ == "" {
				adminTZ = strings.TrimSpace(opts.AdminTimeZoneDefault)
			}
			if _, err := loadAdminLocation(adminTZ); err != nil {
				adminTZInvalid = true
			}
		}
		adminTZEffective := adminTZ
		if adminTZInvalid {
			adminTZEffective = strings.TrimSpace(opts.AdminTimeZoneDefault)
			if adminTZEffective == "" {
				adminTZEffective = defaultAdminTimeZone
			}
		}

		enabled, ok, err := opts.Store.GetBoolAppSetting(ctx, store.SettingEmailVerificationEnable)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}
		emailVerif := opts.EmailVerificationEnabledDefault
		if ok {
			emailVerif = enabled
		}

		smtpEffective := opts.SMTPDefault
		if smtpEffective.SMTPPort == 0 {
			smtpEffective.SMTPPort = 587
		}
		smtpServer, smtpServerOK, err := opts.Store.GetStringAppSetting(ctx, store.SettingSMTPServer)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}
		if smtpServerOK {
			smtpEffective.SMTPServer = smtpServer
		}
		smtpPort, smtpPortOK, err := opts.Store.GetIntAppSetting(ctx, store.SettingSMTPPort)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}
		if smtpPortOK {
			smtpEffective.SMTPPort = smtpPort
		}
		if smtpEffective.SMTPPort == 0 {
			smtpEffective.SMTPPort = 587
		}
		smtpSSL, smtpSSLOK, err := opts.Store.GetBoolAppSetting(ctx, store.SettingSMTPSSLEnabled)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}
		if smtpSSLOK {
			smtpEffective.SMTPSSLEnabled = smtpSSL
		}
		smtpAccount, smtpAccountOK, err := opts.Store.GetStringAppSetting(ctx, store.SettingSMTPAccount)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}
		if smtpAccountOK {
			smtpEffective.SMTPAccount = smtpAccount
		}
		smtpFrom, smtpFromOK, err := opts.Store.GetStringAppSetting(ctx, store.SettingSMTPFrom)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}
		if smtpFromOK {
			smtpEffective.SMTPFrom = smtpFrom
		}
		smtpToken, smtpTokenOK, err := opts.Store.GetStringAppSetting(ctx, store.SettingSMTPToken)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}
		if smtpTokenOK {
			smtpEffective.SMTPToken = smtpToken
		}

		billingEffective := opts.BillingDefault
		billingEnable, billingEnableOK, err := opts.Store.GetBoolAppSetting(ctx, store.SettingBillingEnablePayAsYouGo)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}
		if billingEnableOK {
			billingEffective.EnablePayAsYouGo = billingEnable
		}
		minTopup, minTopupOK, err := opts.Store.GetDecimalAppSetting(ctx, store.SettingBillingMinTopupCNY)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}
		if minTopupOK {
			billingEffective.MinTopupCNY = minTopup
		}
		creditRatio, creditRatioOK, err := opts.Store.GetDecimalAppSetting(ctx, store.SettingBillingCreditUSDPerCNY)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}
		if creditRatioOK {
			billingEffective.CreditUSDPerCNY = creditRatio
		}
		paygoMult := store.DefaultGroupPriceMultiplier
		paygoMultOK := false
		if v, ok, err := opts.Store.GetDecimalAppSetting(ctx, store.SettingBillingPayAsYouGoPriceMultiplier); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		} else if ok {
			paygoMultOK = true
			paygoMult = v
		}
		if paygoMult.IsNegative() || paygoMult.LessThanOrEqual(decimal.Zero) {
			paygoMult = store.DefaultGroupPriceMultiplier
		}
		paygoMult = paygoMult.Truncate(store.PriceMultiplierScale)
		if billingEffective.MinTopupCNY.IsNegative() {
			billingEffective.MinTopupCNY = decimal.Zero
		}
		if billingEffective.CreditUSDPerCNY.IsNegative() {
			billingEffective.CreditUSDPerCNY = decimal.Zero
		}
		billingEffective.MinTopupCNY = billingEffective.MinTopupCNY.Truncate(store.CNYScale)
		billingEffective.CreditUSDPerCNY = billingEffective.CreditUSDPerCNY.Truncate(store.USDScale)

		resp := adminSettingsResponse{
			SelfMode:          opts.SelfMode,
			Features:          fs,
			FeatureBanGroups:  featureBanGroups(opts.SelfMode, fs),
			StartupConfigKeys: startupConfigKeys,

			SiteBaseURL:          siteBaseURL,
			SiteBaseURLOverride:  siteBaseURLOK,
			SiteBaseURLEffective: uiBaseURLFromRequest(ctx, opts, c.Request),
			SiteBaseURLInvalid:   siteBaseURLInvalid,

			AdminTimeZone:          adminTZ,
			AdminTimeZoneOverride:  adminTZOverride,
			AdminTimeZoneEffective: adminTZEffective,
			AdminTimeZoneInvalid:   adminTZInvalid,

			EmailVerificationEnabled:  emailVerif,
			EmailVerificationOverride: ok,

			SMTPServer:             smtpEffective.SMTPServer,
			SMTPServerOverride:     smtpServerOK,
			SMTPPort:               smtpEffective.SMTPPort,
			SMTPPortOverride:       smtpPortOK,
			SMTPSSLEnabled:         smtpEffective.SMTPSSLEnabled,
			SMTPSSLEnabledOverride: smtpSSLOK,
			SMTPAccount:            smtpEffective.SMTPAccount,
			SMTPAccountOverride:    smtpAccountOK,
			SMTPFrom:               smtpEffective.SMTPFrom,
			SMTPFromOverride:       smtpFromOK,
			SMTPTokenSet:           strings.TrimSpace(smtpEffective.SMTPToken) != "",
			SMTPTokenOverride:      smtpTokenOK,

			BillingEnablePayAsYouGo:             billingEffective.EnablePayAsYouGo,
			BillingEnablePayAsYouGoOverride:     billingEnableOK,
			BillingMinTopupCNY:                  formatCNYFixed(billingEffective.MinTopupCNY),
			BillingMinTopupCNYOverride:          minTopupOK,
			BillingCreditUSDPerCNY:              formatUSDPlain(billingEffective.CreditUSDPerCNY),
			BillingCreditUSDPerCNYOverride:      creditRatioOK,
			BillingPaygoPriceMultiplier:         formatDecimalPlain(paygoMult, store.PriceMultiplierScale),
			BillingPaygoPriceMultiplierOverride: paygoMultOK,
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": resp})
	}
}

func adminSettingsResetHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		ctx := c.Request.Context()

		if err := opts.Store.DeleteAppSettings(ctx,
			store.SettingSiteBaseURL,
			store.SettingAdminTimeZone,
			store.SettingEmailVerificationEnable,
			store.SettingSMTPServer,
			store.SettingSMTPPort,
			store.SettingSMTPSSLEnabled,
			store.SettingSMTPAccount,
			store.SettingSMTPFrom,
			store.SettingSMTPToken,
			store.SettingBillingEnablePayAsYouGo,
			store.SettingBillingMinTopupCNY,
			store.SettingBillingCreditUSDPerCNY,
			store.SettingBillingPayAsYouGoPriceMultiplier,
			store.SettingFeatureDisableWebAnnouncements,
			store.SettingFeatureDisableWebTokens,
			store.SettingFeatureDisableWebUsage,
			store.SettingFeatureDisableModels,
			store.SettingFeatureDisableBilling,
			store.SettingFeatureDisableTickets,
			store.SettingFeatureDisableAdminChannels,
			store.SettingFeatureDisableAdminChannelGroups,
			store.SettingFeatureDisableAdminUsers,
			store.SettingFeatureDisableAdminUsage,
			store.SettingFeatureDisableAdminAnnouncements,
		); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "恢复默认失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已恢复为配置文件默认"})
	}
}

func adminSettingsUpdateHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		var req adminSettingsUpdateRequest
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		ctx := c.Request.Context()

		siteBaseURLRaw := strings.TrimSpace(req.SiteBaseURL)
		siteBaseURL, err := config.NormalizeHTTPBaseURL(siteBaseURLRaw, "site_base_url")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "站点地址不合法：" + err.Error()})
			return
		}
		publicBaseURL := strings.TrimSpace(opts.PublicBaseURLDefault)
		if siteBaseURL == "" || (publicBaseURL != "" && siteBaseURL == publicBaseURL) {
			if err := opts.Store.DeleteAppSetting(ctx, store.SettingSiteBaseURL); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		} else if err := opts.Store.UpsertStringAppSetting(ctx, store.SettingSiteBaseURL, siteBaseURL); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}

		adminTZ := normalizeAdminTimeZoneName(req.AdminTimeZone)
		adminTZDefault := strings.TrimSpace(opts.AdminTimeZoneDefault)
		if adminTZDefault == "" {
			adminTZDefault = defaultAdminTimeZone
		}
		if adminTZ == "" || adminTZ == adminTZDefault {
			if err := opts.Store.DeleteAppSetting(ctx, store.SettingAdminTimeZone); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		} else {
			if _, err := loadAdminLocation(adminTZ); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "时区不合法：" + err.Error()})
				return
			}
			if err := opts.Store.UpsertStringAppSetting(ctx, store.SettingAdminTimeZone, adminTZ); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		}

		if err := opts.Store.UpsertBoolAppSetting(ctx, store.SettingEmailVerificationEnable, req.EmailVerificationEnabled); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}

		defaultSMTPPort := opts.SMTPDefault.SMTPPort
		if defaultSMTPPort == 0 {
			defaultSMTPPort = 587
		}

		smtpServer := strings.TrimSpace(req.SMTPServer)
		if smtpServer == "" || smtpServer == strings.TrimSpace(opts.SMTPDefault.SMTPServer) {
			if err := opts.Store.DeleteAppSetting(ctx, store.SettingSMTPServer); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		} else if err := opts.Store.UpsertStringAppSetting(ctx, store.SettingSMTPServer, smtpServer); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}

		if req.SMTPPort == 0 || req.SMTPPort == defaultSMTPPort {
			if err := opts.Store.DeleteAppSetting(ctx, store.SettingSMTPPort); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		} else if req.SMTPPort < 1 || req.SMTPPort > 65535 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "SMTPPort 不合法"})
			return
		} else if err := opts.Store.UpsertIntAppSetting(ctx, store.SettingSMTPPort, req.SMTPPort); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}

		if req.SMTPSSLEnabled == opts.SMTPDefault.SMTPSSLEnabled {
			if err := opts.Store.DeleteAppSetting(ctx, store.SettingSMTPSSLEnabled); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		} else if err := opts.Store.UpsertBoolAppSetting(ctx, store.SettingSMTPSSLEnabled, req.SMTPSSLEnabled); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}

		smtpAccount := strings.TrimSpace(req.SMTPAccount)
		if smtpAccount == "" || smtpAccount == strings.TrimSpace(opts.SMTPDefault.SMTPAccount) {
			if err := opts.Store.DeleteAppSetting(ctx, store.SettingSMTPAccount); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		} else if err := opts.Store.UpsertStringAppSetting(ctx, store.SettingSMTPAccount, smtpAccount); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}

		smtpFrom := strings.TrimSpace(req.SMTPFrom)
		if smtpFrom == "" || smtpFrom == strings.TrimSpace(opts.SMTPDefault.SMTPFrom) {
			if err := opts.Store.DeleteAppSetting(ctx, store.SettingSMTPFrom); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		} else if err := opts.Store.UpsertStringAppSetting(ctx, store.SettingSMTPFrom, smtpFrom); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}

		smtpToken := strings.TrimSpace(req.SMTPToken)
		if smtpToken != "" {
			if smtpToken == strings.TrimSpace(opts.SMTPDefault.SMTPToken) {
				if err := opts.Store.DeleteAppSetting(ctx, store.SettingSMTPToken); err != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
					return
				}
			} else if err := opts.Store.UpsertStringAppSetting(ctx, store.SettingSMTPToken, smtpToken); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		}

		// billing
		if req.BillingEnablePayAsYouGo == opts.BillingDefault.EnablePayAsYouGo {
			if err := opts.Store.DeleteAppSetting(ctx, store.SettingBillingEnablePayAsYouGo); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		} else if err := opts.Store.UpsertBoolAppSetting(ctx, store.SettingBillingEnablePayAsYouGo, req.BillingEnablePayAsYouGo); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}

		if strings.TrimSpace(req.BillingMinTopupCNY) == "" {
			if err := opts.Store.DeleteAppSetting(ctx, store.SettingBillingMinTopupCNY); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		} else {
			d, err := parseCNY(req.BillingMinTopupCNY)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "min_topup_cny 不合法"})
				return
			}
			if d.Equal(opts.BillingDefault.MinTopupCNY) {
				if err := opts.Store.DeleteAppSetting(ctx, store.SettingBillingMinTopupCNY); err != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
					return
				}
			} else if err := opts.Store.UpsertDecimalAppSetting(ctx, store.SettingBillingMinTopupCNY, d); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		}

		if strings.TrimSpace(req.BillingCreditUSDPerCNY) == "" {
			if err := opts.Store.DeleteAppSetting(ctx, store.SettingBillingCreditUSDPerCNY); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		} else {
			d, err := parseDecimalNonNeg(req.BillingCreditUSDPerCNY, store.USDScale)
			if err != nil || d.Sign() <= 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "credit_usd_per_cny 不合法"})
				return
			}
			if d.Equal(opts.BillingDefault.CreditUSDPerCNY) {
				if err := opts.Store.DeleteAppSetting(ctx, store.SettingBillingCreditUSDPerCNY); err != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
					return
				}
			} else if err := opts.Store.UpsertDecimalAppSetting(ctx, store.SettingBillingCreditUSDPerCNY, d); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		}

		if strings.TrimSpace(req.BillingPaygoPriceMultiplier) == "" {
			if err := opts.Store.DeleteAppSetting(ctx, store.SettingBillingPayAsYouGoPriceMultiplier); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		} else {
			d, err := parseDecimalNonNeg(req.BillingPaygoPriceMultiplier, store.PriceMultiplierScale)
			if err != nil || d.Sign() <= 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "billing_paygo_price_multiplier 不合法"})
				return
			}
			if d.Equal(store.DefaultGroupPriceMultiplier) {
				if err := opts.Store.DeleteAppSetting(ctx, store.SettingBillingPayAsYouGoPriceMultiplier); err != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
					return
				}
			} else if err := opts.Store.UpsertDecimalAppSetting(ctx, store.SettingBillingPayAsYouGoPriceMultiplier, d); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		}

		for _, key := range featureBanKeys {
			if opts.SelfMode && (key == store.SettingFeatureDisableBilling || key == store.SettingFeatureDisableTickets) {
				continue
			}
			enabled := false
			if req.FeatureEnabled != nil {
				enabled = req.FeatureEnabled[key]
			}
			if enabled {
				if err := opts.Store.DeleteAppSetting(ctx, key); err != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
					return
				}
				continue
			}
			if err := opts.Store.UpsertBoolAppSetting(ctx, key, true); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}
