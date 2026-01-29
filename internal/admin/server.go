// Package admin 提供最小的 SSR 管理后台：上游资源配置（channel/endpoint/credential/account）。
package admin

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"net/mail"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"realms/internal/auth"
	"realms/internal/codexoauth"
	"realms/internal/config"
	"realms/internal/icons"
	"realms/internal/modellibrary"
	"realms/internal/scheduler"
	"realms/internal/security"
	"realms/internal/store"
	ticketspkg "realms/internal/tickets"
)

//go:embed templates/*.html
var templatesFS embed.FS

type Server struct {
	st *store.Store

	codexOAuth *codexoauth.Flow
	exec       UpstreamDoer
	sched      *scheduler.Scheduler
	modelsDev  *modellibrary.ModelsDevCatalog

	selfMode bool

	publicBaseURL        string
	adminTimeZoneDefault string
	trustProxyHeaders    bool
	trustedProxies       []netip.Prefix

	emailVerifDefault bool
	smtpDefault       config.SMTPConfig
	billingDefault    config.BillingConfig
	paymentDefault    config.PaymentConfig

	ticketsCfg    config.TicketsConfig
	ticketStorage *ticketspkg.Storage

	tmpl *template.Template

	// 批量注册任务管理器
	batchRegisterTaskManager *TaskManager
	goRegisterExecutor       *GoRegisterExecutor
}

func NewServer(st *store.Store, codexOAuth *codexoauth.Flow, exec UpstreamDoer, selfMode bool, emailVerifDefault bool, smtpDefault config.SMTPConfig, billingDefault config.BillingConfig, paymentDefault config.PaymentConfig, publicBaseURL string, adminTimeZoneDefault string, trustProxyHeaders bool, trustedProxyCIDRs []string, ticketsCfg config.TicketsConfig, ticketStorage *ticketspkg.Storage, sched *scheduler.Scheduler) (*Server, error) {
	if ticketStorage == nil {
		ticketStorage = ticketspkg.NewStorage(ticketsCfg.AttachmentsDir)
	}
	t, err := template.New("admin").Funcs(template.FuncMap{
		"modelIconURL":          icons.ModelIconURL,
		"csvHasGroup":           csvHasGroup,
		"csvHasOptionalGroup":   csvHasOptionalGroup,
		"isDefaultGroup":        isDefaultGroup,
		"formatMultiplierPlain": formatMultiplierPlain,
		"formatOptionalInt":     formatOptionalInt,
		"formatOptionalInt64":   formatOptionalInt64,
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	var trustedProxies []netip.Prefix
	for _, raw := range trustedProxyCIDRs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		pfx, err := netip.ParsePrefix(s)
		if err != nil {
			// 兼容单个 IP（不带 /32 或 /128）。
			addr, err2 := netip.ParseAddr(s)
			if err2 != nil {
				return nil, fmt.Errorf("解析 trusted_proxy_cidrs[%q] 失败: %w", s, err2)
			}
			pfx = netip.PrefixFrom(addr, addr.BitLen())
		}
		trustedProxies = append(trustedProxies, pfx)
	}
	adminTZDefault := normalizeAdminTimeZoneName(adminTimeZoneDefault)
	if adminTZDefault == "" {
		adminTZDefault = defaultAdminTimeZone
	}

	// 初始化批量注册任务管理器
	// 最多2个并发任务，已完成任务保留1小时
	taskManager := NewTaskManager(2, 1*time.Hour)

	// 初始化Go执行器
	goExec := NewGoRegisterExecutor()

	modelsDevCatalog := modellibrary.NewModelsDevCatalog(modellibrary.ModelsDevCatalogOptions{})

	return &Server{
		st:                       st,
		codexOAuth:               codexOAuth,
		exec:                     exec,
		sched:                    sched,
		modelsDev:                modelsDevCatalog,
		selfMode:                 selfMode,
		publicBaseURL:            strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"),
		adminTimeZoneDefault:     adminTZDefault,
		trustProxyHeaders:        trustProxyHeaders,
		trustedProxies:           trustedProxies,
		emailVerifDefault:        emailVerifDefault,
		smtpDefault:              smtpDefault,
		billingDefault:           billingDefault,
		paymentDefault:           paymentDefault,
		ticketsCfg:               ticketsCfg,
		ticketStorage:            ticketStorage,
		tmpl:                     t,
		batchRegisterTaskManager: taskManager,
		goRegisterExecutor:       goExec,
	}, nil
}

func (s *Server) emailVerificationEnabled(ctx context.Context) bool {
	v, ok, err := s.st.GetBoolAppSetting(ctx, store.SettingEmailVerificationEnable)
	if err != nil {
		return s.emailVerifDefault
	}
	if ok {
		return v
	}
	return s.emailVerifDefault
}

func (s *Server) smtpConfigEffective(ctx context.Context) config.SMTPConfig {
	cfg := s.smtpDefault
	if cfg.SMTPPort == 0 {
		cfg.SMTPPort = 587
	}

	server, ok, err := s.st.GetStringAppSetting(ctx, store.SettingSMTPServer)
	if err == nil && ok {
		cfg.SMTPServer = server
	}
	port, ok, err := s.st.GetIntAppSetting(ctx, store.SettingSMTPPort)
	if err == nil && ok {
		cfg.SMTPPort = port
	}
	if cfg.SMTPPort == 0 {
		cfg.SMTPPort = 587
	}
	ssl, ok, err := s.st.GetBoolAppSetting(ctx, store.SettingSMTPSSLEnabled)
	if err == nil && ok {
		cfg.SMTPSSLEnabled = ssl
	}
	account, ok, err := s.st.GetStringAppSetting(ctx, store.SettingSMTPAccount)
	if err == nil && ok {
		cfg.SMTPAccount = account
	}
	from, ok, err := s.st.GetStringAppSetting(ctx, store.SettingSMTPFrom)
	if err == nil && ok {
		cfg.SMTPFrom = from
	}
	token, ok, err := s.st.GetStringAppSetting(ctx, store.SettingSMTPToken)
	if err == nil && ok {
		cfg.SMTPToken = token
	}
	return cfg
}

type userView struct {
	ID    int64
	Email string
	Role  string
}

type featureBanItemView struct {
	Key              string
	Label            string
	Hint             string
	Disabled         bool
	Override         bool
	Editable         bool
	ForcedBySelfMode bool
	ForcedByBuild    bool
}

type featureBanGroupView struct {
	Title string
	Items []featureBanItemView
}

type oauthAppView struct {
	ID               int64
	ClientID         string
	Name             string
	Status           int
	StatusLabel      string
	HasSecret        bool
	RedirectURIs     []string
	RedirectURIsText string
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
				b(store.SettingFeatureDisableBilling, "订阅/充值/支付", "隐藏入口，并对 /subscription、/topup、/pay、/admin/subscriptions|orders|payment-channels|settings/payment-channels 及支付回调返回 404；同时数据面进入 free mode（不校验订阅/余额）。", fs.BillingDisabled, selfMode, false),
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
				b(store.SettingFeatureDisableAdminChannels, "上游渠道", "隐藏入口，并对 /admin/channels*、/admin/endpoints* 等返回 404。", fs.AdminChannelsDisabled, false, false),
				b(store.SettingFeatureDisableAdminChannelGroups, "分组", "隐藏入口，并对 /admin/channel-groups* 返回 404。", fs.AdminChannelGroupsDisabled, false, false),
				b(store.SettingFeatureDisableAdminUsers, "用户管理", "隐藏入口，并对 /admin/users* 返回 404。", fs.AdminUsersDisabled, false, false),
				b(store.SettingFeatureDisableAdminUsage, "用量统计（管理后台）", "隐藏入口，并对 /admin/usage 返回 404。", fs.AdminUsageDisabled, false, false),
				b(store.SettingFeatureDisableAdminAnnouncements, "公告（管理后台）", "隐藏入口，并对 /admin/announcements* 返回 404。", fs.AdminAnnouncementsDisabled, false, false),
			},
		},
	}
}

type templateData struct {
	Title       string
	ContentHTML template.HTML
	Error       string
	Notice      string
	User        userView
	IsRoot      bool
	CSRFToken   string
	SelfMode    bool
	Features    store.FeatureState

	FeatureBanGroups  []featureBanGroupView
	StartupConfigKeys []string

	SiteBaseURL          string
	SiteBaseURLOverride  bool
	SiteBaseURLEffective string
	SiteBaseURLInvalid   bool

	AdminTimeZone          string
	AdminTimeZoneOverride  bool
	AdminTimeZoneEffective string
	AdminTimeZoneInvalid   bool

	EmailVerificationEnabled  bool
	EmailVerificationOverride bool

	SMTPServer             string
	SMTPServerOverride     bool
	SMTPPort               int
	SMTPPortOverride       bool
	SMTPSSLEnabled         bool
	SMTPSSLEnabledOverride bool
	SMTPAccount            string
	SMTPAccountOverride    bool
	SMTPFrom               string
	SMTPFromOverride       bool
	SMTPTokenSet           bool
	SMTPTokenOverride      bool

	BillingEnablePayAsYouGo         bool
	BillingEnablePayAsYouGoOverride bool
	BillingMinTopupCNY              string
	BillingMinTopupCNYOverride      bool
	BillingCreditUSDPerCNY          string
	BillingCreditUSDPerCNYOverride  bool

	PaymentEPayEnable            bool
	PaymentEPayEnableOverride    bool
	PaymentEPayGateway           string
	PaymentEPayGatewayOverride   bool
	PaymentEPayPartnerID         string
	PaymentEPayPartnerIDOverride bool
	PaymentEPayKeySet            bool
	PaymentEPayKeyOverride       bool

	PaymentStripeEnable                bool
	PaymentStripeEnableOverride        bool
	PaymentStripeCurrency              string
	PaymentStripeCurrencyOverride      bool
	PaymentStripeSecretKeySet          bool
	PaymentStripeSecretKeyOverride     bool
	PaymentStripeWebhookSecretSet      bool
	PaymentStripeWebhookSecretOverride bool

	Users         []adminUserView
	ChannelGroups []store.ChannelGroup

	CurrentGroup        *store.ChannelGroup
	GroupBreadcrumb     []store.ChannelGroup
	ChannelGroupMembers []store.ChannelGroupMemberDetail

	SubscriptionPlans  []subscriptionPlanView
	SubscriptionPlan   subscriptionPlanView
	SubscriptionOrders []subscriptionOrderView
	PaymentChannels    []paymentChannelView
	PaymentChannel     *paymentChannelView

	UsageNow          string
	UsageStart        string
	UsageEnd          string
	UsageWins         []usageWindowView
	UsageTop          []usageUserView
	UsageEvents       []usageEventView
	UsageNextBeforeID string
	UsagePrevAfterID  string
	UsageCursorActive bool
	UsageLimit        int

	ChannelUsageWindow string
	ChannelUsageSince  string
	ChannelUsageUntil  string
	ChannelKeyUsage    []adminChannelKeyUsageView

	Channels         []store.UpstreamChannel
	ChannelViews     []adminChannelView
	SchedulerRuntime adminSchedulerRuntimeView
	Channel          store.UpstreamChannel
	ChannelRuntime   adminChannelRuntimeView
	Endpoints        []store.UpstreamEndpoint
	Endpoint         store.UpstreamEndpoint
	Credentials      []adminCredentialView
	Accounts         []adminCodexAccountView

	ManagedModels []managedModelView
	ManagedModel  managedModelView

	ChannelModels []channelModelView
	ChannelModel  channelModelView

	Stats *dashboardStats

	Announcements []adminAnnouncementView

	Tickets        []adminTicketListItemView
	Ticket         *adminTicketDetailView
	TicketMessages []adminTicketMessageView

	OAuthApps      []oauthAppView
	OAuthApp       *oauthAppView
	OAuthAppSecret string
}

var startupConfigKeys = []string{
	"REALMS_ENV",
	"REALMS_DB_DSN",
	"REALMS_ADDR",
	"REALMS_PUBLIC_BASE_URL",
	"REALMS_SERVER_READ_HEADER_TIMEOUT",
	"REALMS_SERVER_READ_TIMEOUT",
	"REALMS_SERVER_WRITE_TIMEOUT",
	"REALMS_SERVER_IDLE_TIMEOUT",
	"REALMS_LIMITS_DEFAULT_MAX_OUTPUT_TOKENS",
	"REALMS_LIMITS_MAX_BODY_BYTES",
	"REALMS_LIMITS_MAX_INFLIGHT_PER_CREDENTIAL",
	"REALMS_LIMITS_MAX_INFLIGHT_PER_TOKEN",
	"REALMS_LIMITS_MAX_REQUEST_DURATION",
	"REALMS_LIMITS_MAX_STREAM_DURATION",
	"REALMS_LIMITS_MAX_SSE_CONNECTIONS_PER_TOKEN",
	"REALMS_LIMITS_STREAM_IDLE_TIMEOUT",
	"REALMS_LIMITS_SSE_PING_INTERVAL",
	"REALMS_LIMITS_SSE_MAX_EVENT_BYTES",
	"REALMS_LIMITS_UPSTREAM_DIAL_TIMEOUT",
	"REALMS_LIMITS_UPSTREAM_REQUEST_TIMEOUT",
	"REALMS_LIMITS_UPSTREAM_RESPONSE_HEADER_TIMEOUT",
	"REALMS_LIMITS_UPSTREAM_TLS_HANDSHAKE_TIMEOUT",
	"REALMS_ALLOW_OPEN_REGISTRATION",
	"REALMS_DISABLE_SECURE_COOKIES",
	"REALMS_TRUST_PROXY_HEADERS",
	"REALMS_TRUSTED_PROXY_CIDRS",
	"REALMS_SUBSCRIPTION_ORDER_WEBHOOK_SECRET",
	"REALMS_TICKETS_ATTACHMENTS_DIR",
	"REALMS_TICKETS_ATTACHMENT_TTL",
	"REALMS_TICKETS_MAX_UPLOAD_BYTES",
	"REALMS_CODEX_OAUTH_ENABLE",
	"REALMS_CODEX_OAUTH_CLIENT_ID",
	"REALMS_CODEX_OAUTH_AUTHORIZE_URL",
	"REALMS_CODEX_OAUTH_TOKEN_URL",
	"REALMS_CODEX_OAUTH_HTTP_TIMEOUT",
	"REALMS_CODEX_OAUTH_TLS_HANDSHAKE_TIMEOUT",
	"REALMS_CODEX_OAUTH_REQUEST_PASSTHROUGH",
	"REALMS_CODEX_OAUTH_CALLBACK_LISTEN_ADDR",
	"REALMS_CODEX_OAUTH_REDIRECT_URI",
	"REALMS_CODEX_OAUTH_SCOPE",
	"REALMS_CODEX_OAUTH_PROMPT",
	"REALMS_SELF_MODE_ENABLE",
}

type dashboardStats struct {
	UsersCount              int64
	ChannelsCount           int64
	EndpointsCount          int64
	RequestsToday           int64
	TokensToday             int64
	InputTokensToday        int64
	OutputTokensToday       int64
	CachedInputTokensToday  int64
	CachedOutputTokensToday int64
	CacheRatioToday         string
	CostToday               string
}

type adminChannelView struct {
	Channel  store.UpstreamChannel
	Endpoint store.UpstreamEndpoint
	Usage    adminChannelUsageView
	Runtime  adminChannelRuntimeView

	Credentials   []adminCredentialView
	Accounts      []adminCodexAccountView
	ChannelModels []channelModelView
}

type adminChannelUsageView struct {
	CommittedUSD string
	Tokens       int64
	CacheRatio   string
}

type adminRuntimeLimitsView struct {
	Available bool

	RPM      int
	TPM      int
	Sessions int

	CoolingUntil string
	FailScore    int
}

type adminChannelRuntimeView struct {
	Available bool

	FailScore       int
	BannedUntil     string
	BannedRemaining string
	BanStreak       int
	BannedActive    bool

	PinnedActive bool
}

type adminSchedulerRuntimeView struct {
	Available bool

	PinnedActive    bool
	PinnedChannelID int64
	PinnedChannel   string

	LastSuccessActive    bool
	LastSuccessChannelID int64
	LastSuccessChannel   string
	LastSuccessAt        string
}

type adminCredentialView struct {
	ID         int64
	Name       *string
	APIKeyHint *string
	MaskedKey  string
	Status     int

	Runtime adminRuntimeLimitsView
}

type adminUserView struct {
	ID         int64
	Email      string
	Username   string
	Groups     string
	Role       string
	Status     int
	BalanceUSD string
	CreatedAt  string
}

type adminCodexAccountView struct {
	ID                      int64
	AccountID               string
	Email                   *string
	Status                  int
	InCooldown              bool
	ExpiresAt               string
	LastRefreshAt           string
	CooldownUntil           string
	PlanType                string
	SubscriptionActiveStart string
	SubscriptionActiveUntil string
	SubscriptionPercent     int
	SubscriptionActive      bool
	SubscriptionDaysLeft    string

	QuotaCredits   string
	QuotaPrimary   string
	QuotaSecondary string
	QuotaUpdatedAt string
	QuotaError     string

	QuotaPrimaryUsed     int
	QuotaPrimaryShow     bool
	QuotaPrimaryDetail   string
	QuotaSecondaryUsed   int
	QuotaSecondaryShow   bool
	QuotaSecondaryDetail string

	Runtime adminRuntimeLimitsView
}

func (s *Server) withFeatures(ctx context.Context, data templateData) templateData {
	data.Features = s.st.FeatureStateEffective(ctx, s.selfMode)
	return data
}

func (s *Server) render(w http.ResponseWriter, name string, data templateData) {
	data.SelfMode = s.selfMode

	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "页面渲染失败", http.StatusInternalServerError)
		return
	}
	data.ContentHTML = template.HTML(buf.String())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "admin_base", data); err != nil {
		http.Error(w, "页面渲染失败", http.StatusInternalServerError)
	}
}

func (s *Server) Home(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	// Fetch dashboard stats
	ctx := r.Context()
	usersCount, _ := s.st.CountUsers(ctx)
	channelsCount, _ := s.st.CountUpstreamChannels(ctx)
	endpointsCount, _ := s.st.CountUpstreamEndpoints(ctx)

	loc, tzName := s.adminTimeLocation(ctx)

	// Stats for today (Admin Time Zone)
	nowUTC := time.Now().UTC()
	now := nowUTC.In(loc)
	todayStartLocal := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	todayStartUTC := todayStartLocal.UTC()
	usageStats, _ := s.st.GetGlobalUsageStats(ctx, todayStartUTC)
	cacheRatio := "0%"
	if usageStats.Tokens > 0 {
		cacheRatio = fmt.Sprintf("%.2f%%", usageStats.CacheRatio*100)
	}

	stats := &dashboardStats{
		UsersCount:              usersCount,
		ChannelsCount:           channelsCount,
		EndpointsCount:          endpointsCount,
		RequestsToday:           usageStats.Requests,
		TokensToday:             usageStats.Tokens,
		InputTokensToday:        usageStats.InputTokens,
		OutputTokensToday:       usageStats.OutputTokens,
		CachedInputTokensToday:  usageStats.CachedInputTokens,
		CachedOutputTokensToday: usageStats.CachedOutputTokens,
		CacheRatioToday:         cacheRatio,
		CostToday:               formatUSDPlain(usageStats.CostUSD),
	}

	s.render(w, "admin_home", s.withFeatures(r.Context(), templateData{
		Title:                  "管理后台 - Realms",
		User:                   u,
		IsRoot:                 isRoot,
		CSRFToken:              csrf,
		AdminTimeZoneEffective: tzName,
		Stats:                  stats,
	}))
}

func (s *Server) Settings(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()

	siteBaseURL := s.publicBaseURL
	siteBaseURLOK := false
	siteBaseURLInvalid := false
	siteBaseURLRaw, ok, err := s.st.GetStringAppSetting(ctx, store.SettingSiteBaseURL)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
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

	adminTZ := s.adminTimeZoneDefault
	adminTZOverride := false
	adminTZInvalid := false
	adminTZRaw, tzOK, err := s.st.GetStringAppSetting(ctx, store.SettingAdminTimeZone)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if tzOK && strings.TrimSpace(adminTZRaw) != "" {
		adminTZOverride = true
		adminTZ = normalizeAdminTimeZoneName(adminTZRaw)
		if adminTZ == "" {
			adminTZ = s.adminTimeZoneDefault
		}
		if _, err := loadAdminLocation(adminTZ); err != nil {
			adminTZInvalid = true
		}
	}
	adminTZEffective := adminTZ
	if adminTZInvalid {
		adminTZEffective = s.adminTimeZoneDefault
	}

	enabled, ok, err := s.st.GetBoolAppSetting(ctx, store.SettingEmailVerificationEnable)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	effective := s.emailVerifDefault
	if ok {
		effective = enabled
	}

	smtpEffective := s.smtpDefault
	if smtpEffective.SMTPPort == 0 {
		smtpEffective.SMTPPort = 587
	}
	smtpServer, smtpServerOK, err := s.st.GetStringAppSetting(ctx, store.SettingSMTPServer)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if smtpServerOK {
		smtpEffective.SMTPServer = smtpServer
	}
	smtpPort, smtpPortOK, err := s.st.GetIntAppSetting(ctx, store.SettingSMTPPort)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if smtpPortOK {
		smtpEffective.SMTPPort = smtpPort
	}
	if smtpEffective.SMTPPort == 0 {
		smtpEffective.SMTPPort = 587
	}
	smtpSSL, smtpSSLOK, err := s.st.GetBoolAppSetting(ctx, store.SettingSMTPSSLEnabled)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if smtpSSLOK {
		smtpEffective.SMTPSSLEnabled = smtpSSL
	}
	smtpAccount, smtpAccountOK, err := s.st.GetStringAppSetting(ctx, store.SettingSMTPAccount)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if smtpAccountOK {
		smtpEffective.SMTPAccount = smtpAccount
	}
	smtpFrom, smtpFromOK, err := s.st.GetStringAppSetting(ctx, store.SettingSMTPFrom)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if smtpFromOK {
		smtpEffective.SMTPFrom = smtpFrom
	}
	smtpToken, smtpTokenOK, err := s.st.GetStringAppSetting(ctx, store.SettingSMTPToken)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if smtpTokenOK {
		smtpEffective.SMTPToken = smtpToken
	}

	billingEffective := s.billingDefault
	billingEnable, billingEnableOK, err := s.st.GetBoolAppSetting(ctx, store.SettingBillingEnablePayAsYouGo)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if billingEnableOK {
		billingEffective.EnablePayAsYouGo = billingEnable
	}
	minTopup, minTopupOK, err := s.st.GetDecimalAppSetting(ctx, store.SettingBillingMinTopupCNY)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if minTopupOK {
		billingEffective.MinTopupCNY = minTopup
	}
	creditRatio, creditRatioOK, err := s.st.GetDecimalAppSetting(ctx, store.SettingBillingCreditUSDPerCNY)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if creditRatioOK {
		billingEffective.CreditUSDPerCNY = creditRatio
	}

	epayEffective := s.paymentDefault.EPay
	epayEnable, epayEnableOK, err := s.st.GetBoolAppSetting(ctx, store.SettingPaymentEPayEnable)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if epayEnableOK {
		epayEffective.Enable = epayEnable
	}
	epayGateway, epayGatewayOK, err := s.st.GetStringAppSetting(ctx, store.SettingPaymentEPayGateway)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if epayGatewayOK {
		epayEffective.Gateway = epayGateway
	}
	epayPartnerID, epayPartnerIDOK, err := s.st.GetStringAppSetting(ctx, store.SettingPaymentEPayPartnerID)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if epayPartnerIDOK {
		epayEffective.PartnerID = epayPartnerID
	}
	epayKey, epayKeyOK, err := s.st.GetStringAppSetting(ctx, store.SettingPaymentEPayKey)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if epayKeyOK {
		epayEffective.Key = epayKey
	}

	stripeEffective := s.paymentDefault.Stripe
	if strings.TrimSpace(stripeEffective.Currency) == "" {
		stripeEffective.Currency = "cny"
	}
	stripeEnable, stripeEnableOK, err := s.st.GetBoolAppSetting(ctx, store.SettingPaymentStripeEnable)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if stripeEnableOK {
		stripeEffective.Enable = stripeEnable
	}
	stripeCurrency, stripeCurrencyOK, err := s.st.GetStringAppSetting(ctx, store.SettingPaymentStripeCurrency)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if stripeCurrencyOK {
		stripeEffective.Currency = stripeCurrency
	}
	stripeSecret, stripeSecretOK, err := s.st.GetStringAppSetting(ctx, store.SettingPaymentStripeSecretKey)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if stripeSecretOK {
		stripeEffective.SecretKey = stripeSecret
	}
	stripeWebhookSecret, stripeWebhookSecretOK, err := s.st.GetStringAppSetting(ctx, store.SettingPaymentStripeWebhookSecret)
	if err != nil {
		http.Error(w, "查询配置失败", http.StatusInternalServerError)
		return
	}
	if stripeWebhookSecretOK {
		stripeEffective.WebhookSecret = stripeWebhookSecret
	}

	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}

	data := s.withFeatures(r.Context(), templateData{
		Title:                     "系统设置 - Realms",
		Error:                     errMsg,
		Notice:                    notice,
		User:                      u,
		IsRoot:                    isRoot,
		CSRFToken:                 csrf,
		StartupConfigKeys:         startupConfigKeys,
		SiteBaseURL:               siteBaseURL,
		SiteBaseURLOverride:       siteBaseURLOK,
		SiteBaseURLEffective:      s.baseURLFromRequest(r),
		SiteBaseURLInvalid:        siteBaseURLInvalid,
		AdminTimeZone:             adminTZ,
		AdminTimeZoneOverride:     adminTZOverride,
		AdminTimeZoneEffective:    adminTZEffective,
		AdminTimeZoneInvalid:      adminTZInvalid,
		EmailVerificationEnabled:  effective,
		EmailVerificationOverride: ok,
		SMTPServer:                smtpEffective.SMTPServer,
		SMTPServerOverride:        smtpServerOK,
		SMTPPort:                  smtpEffective.SMTPPort,
		SMTPPortOverride:          smtpPortOK,
		SMTPSSLEnabled:            smtpEffective.SMTPSSLEnabled,
		SMTPSSLEnabledOverride:    smtpSSLOK,
		SMTPAccount:               smtpEffective.SMTPAccount,
		SMTPAccountOverride:       smtpAccountOK,
		SMTPFrom:                  smtpEffective.SMTPFrom,
		SMTPFromOverride:          smtpFromOK,
		SMTPTokenSet:              strings.TrimSpace(smtpEffective.SMTPToken) != "",
		SMTPTokenOverride:         smtpTokenOK,

		BillingEnablePayAsYouGo:         billingEffective.EnablePayAsYouGo,
		BillingEnablePayAsYouGoOverride: billingEnableOK,
		BillingMinTopupCNY:              formatCNYPlain(billingEffective.MinTopupCNY),
		BillingMinTopupCNYOverride:      minTopupOK,
		BillingCreditUSDPerCNY:          formatUSDPlain(billingEffective.CreditUSDPerCNY),
		BillingCreditUSDPerCNYOverride:  creditRatioOK,

		PaymentEPayEnable:            epayEffective.Enable,
		PaymentEPayEnableOverride:    epayEnableOK,
		PaymentEPayGateway:           epayEffective.Gateway,
		PaymentEPayGatewayOverride:   epayGatewayOK,
		PaymentEPayPartnerID:         epayEffective.PartnerID,
		PaymentEPayPartnerIDOverride: epayPartnerIDOK,
		PaymentEPayKeySet:            strings.TrimSpace(epayEffective.Key) != "",
		PaymentEPayKeyOverride:       epayKeyOK,

		PaymentStripeEnable:                stripeEffective.Enable,
		PaymentStripeEnableOverride:        stripeEnableOK,
		PaymentStripeCurrency:              stripeEffective.Currency,
		PaymentStripeCurrencyOverride:      stripeCurrencyOK,
		PaymentStripeSecretKeySet:          strings.TrimSpace(stripeEffective.SecretKey) != "",
		PaymentStripeSecretKeyOverride:     stripeSecretOK,
		PaymentStripeWebhookSecretSet:      strings.TrimSpace(stripeEffective.WebhookSecret) != "",
		PaymentStripeWebhookSecretOverride: stripeWebhookSecretOK,
	})
	data.FeatureBanGroups = featureBanGroups(s.selfMode, data.Features)
	s.render(w, "admin_settings", data)
}

func (s *Server) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	action := strings.TrimSpace(r.FormValue("action"))
	switch action {
	case "reset":
		if err := s.st.DeleteAppSettings(r.Context(),
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
			store.SettingPaymentEPayEnable,
			store.SettingPaymentEPayGateway,
			store.SettingPaymentEPayPartnerID,
			store.SettingPaymentEPayKey,
			store.SettingPaymentStripeEnable,
			store.SettingPaymentStripeCurrency,
			store.SettingPaymentStripeSecretKey,
			store.SettingPaymentStripeWebhookSecret,
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
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "恢复默认失败")
				return
			}
			http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("恢复默认失败"), http.StatusFound)
			return
		}
		if isAjax(r) {
			ajaxOK(w, "已恢复为配置文件默认")
			return
		}
		http.Redirect(w, r, "/admin/settings?msg="+url.QueryEscape("已恢复为配置文件默认"), http.StatusFound)
	default:
		ctx := r.Context()
		defaultSMTPPort := s.smtpDefault.SMTPPort
		if defaultSMTPPort == 0 {
			defaultSMTPPort = 587
		}

		siteBaseURLRaw := strings.TrimSpace(r.FormValue("site_base_url"))
		siteBaseURL, err := config.NormalizeHTTPBaseURL(siteBaseURLRaw, "site_base_url")
		if err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "站点地址不合法："+err.Error())
				return
			}
			http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("站点地址不合法："+err.Error()), http.StatusFound)
			return
		}
		if siteBaseURL == "" || siteBaseURL == s.publicBaseURL {
			if err := s.st.DeleteAppSetting(ctx, store.SettingSiteBaseURL); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		} else if err := s.st.UpsertStringAppSetting(ctx, store.SettingSiteBaseURL, siteBaseURL); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "保存失败")
				return
			}
			http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
			return
		}

		adminTZ := normalizeAdminTimeZoneName(r.FormValue("admin_time_zone"))
		if adminTZ == "" || adminTZ == s.adminTimeZoneDefault {
			if err := s.st.DeleteAppSetting(ctx, store.SettingAdminTimeZone); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		} else {
			if _, err := loadAdminLocation(adminTZ); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusBadRequest, "时区不合法："+err.Error())
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("时区不合法："+err.Error()), http.StatusFound)
				return
			}
			if err := s.st.UpsertStringAppSetting(ctx, store.SettingAdminTimeZone, adminTZ); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		}

		enabled := strings.TrimSpace(r.FormValue("email_verification_enable")) != ""
		if err := s.st.UpsertBoolAppSetting(ctx, store.SettingEmailVerificationEnable, enabled); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "保存失败")
				return
			}
			http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
			return
		}

		smtpServer := strings.TrimSpace(r.FormValue("SMTPServer"))
		if smtpServer == "" || smtpServer == strings.TrimSpace(s.smtpDefault.SMTPServer) {
			if err := s.st.DeleteAppSetting(ctx, store.SettingSMTPServer); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		} else if err := s.st.UpsertStringAppSetting(ctx, store.SettingSMTPServer, smtpServer); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "保存失败")
				return
			}
			http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
			return
		}

		smtpPortRaw := strings.TrimSpace(r.FormValue("SMTPPort"))
		if smtpPortRaw == "" {
			if err := s.st.DeleteAppSetting(ctx, store.SettingSMTPPort); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		} else {
			n, err := strconv.Atoi(smtpPortRaw)
			if err != nil || n < 1 || n > 65535 {
				if isAjax(r) {
					ajaxError(w, http.StatusBadRequest, "SMTPPort 不合法")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("SMTPPort 不合法"), http.StatusFound)
				return
			}
			if n == defaultSMTPPort {
				if err := s.st.DeleteAppSetting(ctx, store.SettingSMTPPort); err != nil {
					if isAjax(r) {
						ajaxError(w, http.StatusInternalServerError, "保存失败")
						return
					}
					http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
					return
				}
			} else if err := s.st.UpsertIntAppSetting(ctx, store.SettingSMTPPort, n); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		}

		smtpSSL := strings.TrimSpace(r.FormValue("SMTPSSLEnabled")) != ""
		if smtpSSL == s.smtpDefault.SMTPSSLEnabled {
			if err := s.st.DeleteAppSetting(ctx, store.SettingSMTPSSLEnabled); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		} else if err := s.st.UpsertBoolAppSetting(ctx, store.SettingSMTPSSLEnabled, smtpSSL); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "保存失败")
				return
			}
			http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
			return
		}

		smtpAccount := strings.TrimSpace(r.FormValue("SMTPAccount"))
		if smtpAccount == "" || smtpAccount == strings.TrimSpace(s.smtpDefault.SMTPAccount) {
			if err := s.st.DeleteAppSetting(ctx, store.SettingSMTPAccount); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		} else if err := s.st.UpsertStringAppSetting(ctx, store.SettingSMTPAccount, smtpAccount); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "保存失败")
				return
			}
			http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
			return
		}

		smtpFrom := strings.TrimSpace(r.FormValue("SMTPFrom"))
		if smtpFrom == "" || smtpFrom == strings.TrimSpace(s.smtpDefault.SMTPFrom) {
			if err := s.st.DeleteAppSetting(ctx, store.SettingSMTPFrom); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		} else if err := s.st.UpsertStringAppSetting(ctx, store.SettingSMTPFrom, smtpFrom); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "保存失败")
				return
			}
			http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
			return
		}

		smtpToken := strings.TrimSpace(r.FormValue("SMTPToken"))
		if smtpToken != "" {
			if smtpToken == strings.TrimSpace(s.smtpDefault.SMTPToken) {
				if err := s.st.DeleteAppSetting(ctx, store.SettingSMTPToken); err != nil {
					if isAjax(r) {
						ajaxError(w, http.StatusInternalServerError, "保存失败")
						return
					}
					http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
					return
				}
			} else if err := s.st.UpsertStringAppSetting(ctx, store.SettingSMTPToken, smtpToken); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		}

		billingEnable := strings.TrimSpace(r.FormValue("billing_enable_pay_as_you_go")) != ""
		if billingEnable == s.billingDefault.EnablePayAsYouGo {
			if err := s.st.DeleteAppSetting(ctx, store.SettingBillingEnablePayAsYouGo); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		} else if err := s.st.UpsertBoolAppSetting(ctx, store.SettingBillingEnablePayAsYouGo, billingEnable); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "保存失败")
				return
			}
			http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
			return
		}

		minTopupRaw := strings.TrimSpace(r.FormValue("billing_min_topup_cny"))
		if minTopupRaw == "" {
			if err := s.st.DeleteAppSetting(ctx, store.SettingBillingMinTopupCNY); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		} else {
			d, err := parseCNY(minTopupRaw)
			if err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusBadRequest, "min_topup_cny 不合法")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("min_topup_cny 不合法"), http.StatusFound)
				return
			}
			if d.Equal(s.billingDefault.MinTopupCNY) {
				if err := s.st.DeleteAppSetting(ctx, store.SettingBillingMinTopupCNY); err != nil {
					if isAjax(r) {
						ajaxError(w, http.StatusInternalServerError, "保存失败")
						return
					}
					http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
					return
				}
			} else if err := s.st.UpsertDecimalAppSetting(ctx, store.SettingBillingMinTopupCNY, d); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		}

		creditRatioRaw := strings.TrimSpace(r.FormValue("billing_credit_usd_per_cny"))
		if creditRatioRaw == "" {
			if err := s.st.DeleteAppSetting(ctx, store.SettingBillingCreditUSDPerCNY); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		} else {
			d, err := parseDecimalNonNeg(creditRatioRaw, store.USDScale)
			if err != nil || d.Sign() <= 0 {
				if isAjax(r) {
					ajaxError(w, http.StatusBadRequest, "credit_usd_per_cny 不合法")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("credit_usd_per_cny 不合法"), http.StatusFound)
				return
			}
			if d.Equal(s.billingDefault.CreditUSDPerCNY) {
				if err := s.st.DeleteAppSetting(ctx, store.SettingBillingCreditUSDPerCNY); err != nil {
					if isAjax(r) {
						ajaxError(w, http.StatusInternalServerError, "保存失败")
						return
					}
					http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
					return
				}
			} else if err := s.st.UpsertDecimalAppSetting(ctx, store.SettingBillingCreditUSDPerCNY, d); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		}

		// 支付渠道配置已迁移到独立页面（/admin/settings/payment-channels）。
		// 这里保留旧字段的写入逻辑，但仅在表单显式提交这些字段时才处理，
		// 避免在“系统设置”保存/自动保存时误清空旧支付配置。
		_, hasPaymentEPayGateway := r.PostForm["payment_epay_gateway"]
		_, hasPaymentEPayPartnerID := r.PostForm["payment_epay_partner_id"]
		_, hasPaymentEPayKey := r.PostForm["payment_epay_key"]
		_, hasPaymentEPayEnable := r.PostForm["payment_epay_enable"]
		_, hasPaymentStripeCurrency := r.PostForm["payment_stripe_currency"]
		_, hasPaymentStripeSecretKey := r.PostForm["payment_stripe_secret_key"]
		_, hasPaymentStripeWebhookSecret := r.PostForm["payment_stripe_webhook_secret"]
		_, hasPaymentStripeEnable := r.PostForm["payment_stripe_enable"]
		if hasPaymentEPayGateway || hasPaymentEPayPartnerID || hasPaymentEPayKey || hasPaymentEPayEnable ||
			hasPaymentStripeCurrency || hasPaymentStripeSecretKey || hasPaymentStripeWebhookSecret || hasPaymentStripeEnable {
			epayEnable := strings.TrimSpace(r.FormValue("payment_epay_enable")) != ""
			if epayEnable == s.paymentDefault.EPay.Enable {
				if err := s.st.DeleteAppSetting(ctx, store.SettingPaymentEPayEnable); err != nil {
					if isAjax(r) {
						ajaxError(w, http.StatusInternalServerError, "保存失败")
						return
					}
					http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
					return
				}
			} else if err := s.st.UpsertBoolAppSetting(ctx, store.SettingPaymentEPayEnable, epayEnable); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}

			epayGateway := strings.TrimSpace(r.FormValue("payment_epay_gateway"))
			if epayGateway == "" || epayGateway == strings.TrimSpace(s.paymentDefault.EPay.Gateway) {
				if err := s.st.DeleteAppSetting(ctx, store.SettingPaymentEPayGateway); err != nil {
					if isAjax(r) {
						ajaxError(w, http.StatusInternalServerError, "保存失败")
						return
					}
					http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
					return
				}
			} else if err := s.st.UpsertStringAppSetting(ctx, store.SettingPaymentEPayGateway, epayGateway); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}

			epayPartnerID := strings.TrimSpace(r.FormValue("payment_epay_partner_id"))
			if epayPartnerID == "" || epayPartnerID == strings.TrimSpace(s.paymentDefault.EPay.PartnerID) {
				if err := s.st.DeleteAppSetting(ctx, store.SettingPaymentEPayPartnerID); err != nil {
					if isAjax(r) {
						ajaxError(w, http.StatusInternalServerError, "保存失败")
						return
					}
					http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
					return
				}
			} else if err := s.st.UpsertStringAppSetting(ctx, store.SettingPaymentEPayPartnerID, epayPartnerID); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}

			epayKey := strings.TrimSpace(r.FormValue("payment_epay_key"))
			if epayKey != "" {
				if epayKey == strings.TrimSpace(s.paymentDefault.EPay.Key) {
					if err := s.st.DeleteAppSetting(ctx, store.SettingPaymentEPayKey); err != nil {
						if isAjax(r) {
							ajaxError(w, http.StatusInternalServerError, "保存失败")
							return
						}
						http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
						return
					}
				} else if err := s.st.UpsertStringAppSetting(ctx, store.SettingPaymentEPayKey, epayKey); err != nil {
					if isAjax(r) {
						ajaxError(w, http.StatusInternalServerError, "保存失败")
						return
					}
					http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
					return
				}
			}

			stripeEnable := strings.TrimSpace(r.FormValue("payment_stripe_enable")) != ""
			if stripeEnable == s.paymentDefault.Stripe.Enable {
				if err := s.st.DeleteAppSetting(ctx, store.SettingPaymentStripeEnable); err != nil {
					if isAjax(r) {
						ajaxError(w, http.StatusInternalServerError, "保存失败")
						return
					}
					http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
					return
				}
			} else if err := s.st.UpsertBoolAppSetting(ctx, store.SettingPaymentStripeEnable, stripeEnable); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}

			stripeCurrency := strings.TrimSpace(r.FormValue("payment_stripe_currency"))
			if stripeCurrency == "" || stripeCurrency == strings.TrimSpace(s.paymentDefault.Stripe.Currency) {
				if err := s.st.DeleteAppSetting(ctx, store.SettingPaymentStripeCurrency); err != nil {
					if isAjax(r) {
						ajaxError(w, http.StatusInternalServerError, "保存失败")
						return
					}
					http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
					return
				}
			} else if err := s.st.UpsertStringAppSetting(ctx, store.SettingPaymentStripeCurrency, stripeCurrency); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}

			stripeSecret := strings.TrimSpace(r.FormValue("payment_stripe_secret_key"))
			if stripeSecret != "" {
				if stripeSecret == strings.TrimSpace(s.paymentDefault.Stripe.SecretKey) {
					if err := s.st.DeleteAppSetting(ctx, store.SettingPaymentStripeSecretKey); err != nil {
						if isAjax(r) {
							ajaxError(w, http.StatusInternalServerError, "保存失败")
							return
						}
						http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
						return
					}
				} else if err := s.st.UpsertStringAppSetting(ctx, store.SettingPaymentStripeSecretKey, stripeSecret); err != nil {
					if isAjax(r) {
						ajaxError(w, http.StatusInternalServerError, "保存失败")
						return
					}
					http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
					return
				}
			}

			stripeWebhookSecret := strings.TrimSpace(r.FormValue("payment_stripe_webhook_secret"))
			if stripeWebhookSecret != "" {
				if stripeWebhookSecret == strings.TrimSpace(s.paymentDefault.Stripe.WebhookSecret) {
					if err := s.st.DeleteAppSetting(ctx, store.SettingPaymentStripeWebhookSecret); err != nil {
						if isAjax(r) {
							ajaxError(w, http.StatusInternalServerError, "保存失败")
							return
						}
						http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
						return
					}
				} else if err := s.st.UpsertStringAppSetting(ctx, store.SettingPaymentStripeWebhookSecret, stripeWebhookSecret); err != nil {
					if isAjax(r) {
						ajaxError(w, http.StatusInternalServerError, "保存失败")
						return
					}
					http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
					return
				}
			}
		}

		for _, key := range featureBanKeys {
			if s.selfMode && (key == store.SettingFeatureDisableBilling || key == store.SettingFeatureDisableTickets) {
				continue
			}
			enabled := strings.TrimSpace(r.FormValue(key)) != ""
			if enabled {
				if err := s.st.DeleteAppSetting(ctx, key); err != nil {
					if isAjax(r) {
						ajaxError(w, http.StatusInternalServerError, "保存失败")
						return
					}
					http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
					return
				}
				continue
			}
			if err := s.st.UpsertBoolAppSetting(ctx, key, true); err != nil {
				if isAjax(r) {
					ajaxError(w, http.StatusInternalServerError, "保存失败")
					return
				}
				http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("保存失败"), http.StatusFound)
				return
			}
		}

		if isAjax(r) {
			ajaxOK(w, "已保存")
			return
		}
		http.Redirect(w, r, "/admin/settings?msg="+url.QueryEscape("已保存"), http.StatusFound)
	}
}

func (s *Server) Users(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	loc, _ := s.adminTimeLocation(ctx)
	users, err := s.st.ListUsers(ctx)
	if err != nil {
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}
	userIDs := make([]int64, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	balances, err := s.st.GetUserBalancesUSD(ctx, userIDs)
	if err != nil {
		http.Error(w, "余额查询失败", http.StatusInternalServerError)
		return
	}
	var uv []adminUserView
	for _, user := range users {
		uv = append(uv, adminUserView{
			ID:         user.ID,
			Email:      user.Email,
			Username:   user.Username,
			Groups:     strings.Join(user.Groups, ","),
			Role:       user.Role,
			Status:     user.Status,
			BalanceUSD: formatUSDPlain(balances[user.ID]),
			CreatedAt:  formatTimeIn(user.CreatedAt, "2006-01-02 15:04", loc),
		})
	}

	channelGroups, err := s.st.ListChannelGroups(ctx)
	if err != nil {
		http.Error(w, "查询渠道分组失败", http.StatusInternalServerError)
		return
	}
	var usedGroups []string
	for _, user := range users {
		usedGroups = append(usedGroups, user.Groups...)
	}
	channelGroups = mergeChannelGroupsOptions(channelGroups, usedGroups)

	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}
	s.render(w, "admin_users", s.withFeatures(r.Context(), templateData{
		Title:                    "用户管理 - Realms",
		Error:                    errMsg,
		Notice:                   notice,
		User:                     u,
		IsRoot:                   isRoot,
		CSRFToken:                csrf,
		EmailVerificationEnabled: s.emailVerificationEnabled(ctx),
		ChannelGroups:            channelGroups,
		Users:                    uv,
	}))
}

func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request) {
	_, _, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	username, err := store.NormalizeUsername(r.FormValue("username"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	password := r.FormValue("password")
	if email == "" || password == "" {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	role := store.UserRoleUser
	if isRoot {
		if v := strings.TrimSpace(r.FormValue("role")); v != "" {
			role = v
		}
	}
	if !isRoot {
		role = store.UserRoleUser
	}
	if !isValidUserRole(role) {
		http.Error(w, "role 不合法", http.StatusBadRequest)
		return
	}

	userGroupsCSV, err := normalizeUserGroupsValues(r.Form["groups"])
	if err != nil {
		http.Error(w, "groups 不合法: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateChannelGroupsSelectable(r.Context(), s.st, userGroupsCSV); err != nil {
		http.Error(w, "groups 不合法: "+err.Error(), http.StatusBadRequest)
		return
	}

	pwHash, err := auth.HashPassword(password)
	if err != nil {
		http.Error(w, "密码不合法", http.StatusBadRequest)
		return
	}
	if _, err := s.st.GetUserByUsername(r.Context(), username); err == nil {
		http.Error(w, "账号名已被占用", http.StatusBadRequest)
		return
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "查询账号名失败", http.StatusInternalServerError)
		return
	}
	userID, err := s.st.CreateUser(r.Context(), email, username, pwHash, role)
	if err != nil {
		http.Error(w, "创建失败（可能邮箱或账号名已存在）", http.StatusInternalServerError)
		return
	}
	// CreateUser 已写入 default；这里根据表单重设用户组（包含 default 强制规则）。
	if err := s.st.ReplaceUserGroups(r.Context(), userID, splitGroups(userGroupsCSV)); err != nil {
		http.Error(w, "设置用户分组失败", http.StatusInternalServerError)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已创建")
		return
	}
	http.Redirect(w, r, "/admin/users", http.StatusFound)
}

func (s *Server) UpdateUser(w http.ResponseWriter, r *http.Request) {
	u, _, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	userID, err := parseInt64(r.PathValue("user_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	target, err := s.st.GetUserByID(r.Context(), userID)
	if err != nil {
		http.Error(w, "用户不存在", http.StatusNotFound)
		return
	}

	status, err := parseInt(r.FormValue("status"))
	if err != nil || (status != 0 && status != 1) {
		http.Error(w, "status 不合法", http.StatusBadRequest)
		return
	}
	if target.ID == u.ID && status == 0 {
		http.Error(w, "不能禁用当前登录用户", http.StatusBadRequest)
		return
	}

	role := strings.TrimSpace(r.FormValue("role"))
	if role == "" {
		role = target.Role
	}
	if !isRoot {
		role = target.Role
	}
	if !isValidUserRole(role) {
		http.Error(w, "role 不合法", http.StatusBadRequest)
		return
	}
	if target.ID == u.ID && role != store.UserRoleRoot {
		http.Error(w, "不能修改当前登录用户的 root 角色", http.StatusBadRequest)
		return
	}

	if err := s.st.SetUserStatus(r.Context(), target.ID, status); err != nil {
		http.Error(w, "更新失败", http.StatusInternalServerError)
		return
	}
	if isRoot && role != target.Role {
		if err := s.st.SetUserRole(r.Context(), target.ID, role); err != nil {
			http.Error(w, "更新失败", http.StatusInternalServerError)
			return
		}
	}
	if isAjax(r) {
		ajaxOK(w, "已保存")
		return
	}
	http.Redirect(w, r, "/admin/users", http.StatusFound)
}

func (s *Server) UpdateUserProfile(w http.ResponseWriter, r *http.Request) {
	u, _, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !isRoot {
		http.Error(w, "无权限", http.StatusForbidden)
		return
	}
	userID, err := parseInt64(r.PathValue("user_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	target, err := s.st.GetUserByID(ctx, userID)
	if err != nil {
		http.Error(w, "用户不存在", http.StatusNotFound)
		return
	}

	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	if email == "" {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "邮箱不能为空")
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("邮箱不能为空"), http.StatusFound)
		return
	}
	if _, err := mail.ParseAddress(email); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "邮箱不合法")
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("邮箱不合法"), http.StatusFound)
		return
	}

	username, err := store.NormalizeUsername(r.FormValue("username"))
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	if other, err := s.st.GetUserByUsername(ctx, username); err == nil && other.ID != target.ID {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "账号名已被占用")
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("账号名已被占用"), http.StatusFound)
		return
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "查询账号名失败", http.StatusInternalServerError)
		return
	}

	statusStr := r.FormValue("status")
	newStatus := target.Status
	if statusStr != "" {
		v, err := parseInt(statusStr)
		if err == nil && (v == 0 || v == 1) {
			newStatus = v
		}
	}
	if newStatus == 0 && target.ID == u.ID {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "不能禁用当前登录用户")
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("不能禁用当前登录用户"), http.StatusFound)
		return
	}

	role := strings.TrimSpace(r.FormValue("role"))
	if role == "" {
		role = target.Role
	}
	if role != target.Role {
		if !isValidUserRole(role) {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "角色不合法")
				return
			}
			http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("角色不合法"), http.StatusFound)
			return
		}
		if target.ID == u.ID && role != store.UserRoleRoot {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "不能移除自己的管理员权限")
				return
			}
			http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("不能移除自己的管理员权限"), http.StatusFound)
			return
		}
	}

	emailChanged := strings.ToLower(target.Email) != email
	usernameChanged := target.Username != username

	statusChanged := target.Status != newStatus
	roleChanged := target.Role != role

	groupsCSV, err := normalizeUserGroupsValues(r.Form["groups"])
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "groups 不合法: "+err.Error())
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("groups 不合法: "+err.Error()), http.StatusFound)
		return
	}
	newGroups := splitGroups(groupsCSV)
	sort.Strings(newGroups)
	groupsChanged := strings.Join(newGroups, ",") != strings.Join(target.Groups, ",")

	changed := emailChanged || usernameChanged || groupsChanged || statusChanged || roleChanged
	if !changed {
		if isAjax(r) {
			ajaxOK(w, "无变更")
			return
		}
		http.Redirect(w, r, "/admin/users", http.StatusFound)
		return
	}

	if emailChanged {
		if other, err := s.st.GetUserByEmail(ctx, email); err == nil && other.ID != target.ID {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "邮箱地址已被占用")
				return
			}
			http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("邮箱地址已被占用"), http.StatusFound)
			return
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "查询邮箱失败", http.StatusInternalServerError)
			return
		}
		if err := s.st.UpdateUserEmail(ctx, target.ID, email); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "保存失败")
				return
			}
			http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("保存失败"), http.StatusFound)
			return
		}
	}

	if usernameChanged {
		if err := s.st.UpdateUserUsername(ctx, target.ID, username); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "保存失败")
				return
			}
			http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("保存失败"), http.StatusFound)
			return
		}
	}

	if statusChanged {
		if err := s.st.SetUserStatus(ctx, target.ID, newStatus); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "保存失败")
				return
			}
			http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("保存失败"), http.StatusFound)
			return
		}
	}

	if roleChanged {
		if err := s.st.SetUserRole(ctx, target.ID, role); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "保存失败")
				return
			}
			http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("保存失败"), http.StatusFound)
			return
		}
	}

	if groupsChanged {
		if err := validateChannelGroupsSelectable(ctx, s.st, groupsCSV); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, err.Error())
				return
			}
			http.Redirect(w, r, "/admin/users?err="+url.QueryEscape(err.Error()), http.StatusFound)
			return
		}
		if err := s.st.ReplaceUserGroups(ctx, target.ID, newGroups); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "保存失败")
				return
			}
			http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("保存失败"), http.StatusFound)
			return
		}
	}

	if !emailChanged && !usernameChanged && !statusChanged && !roleChanged {
		if isAjax(r) {
			ajaxOK(w, "已保存")
			return
		}
		http.Redirect(w, r, "/admin/users?msg="+url.QueryEscape("已保存"), http.StatusFound)
		return
	}

	if err := s.st.DeleteSessionsByUserID(ctx, target.ID); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "强制登出失败")
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("强制登出失败"), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已保存，并已强制登出该用户")
		return
	}
	http.Redirect(w, r, "/admin/users?msg="+url.QueryEscape("已保存，并已强制登出该用户"), http.StatusFound)
}

func (s *Server) UpdateUserPassword(w http.ResponseWriter, r *http.Request) {
	_, _, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !isRoot {
		http.Error(w, "无权限", http.StatusForbidden)
		return
	}
	userID, err := parseInt64(r.PathValue("user_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	target, err := s.st.GetUserByID(ctx, userID)
	if err != nil {
		http.Error(w, "用户不存在", http.StatusNotFound)
		return
	}

	password := r.FormValue("password")
	if strings.TrimSpace(password) == "" {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "新密码不能为空")
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("新密码不能为空"), http.StatusFound)
		return
	}
	pwHash, err := auth.HashPassword(password)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	if err := s.st.UpdateUserPasswordHash(ctx, target.ID, pwHash); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "保存失败")
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("保存失败"), http.StatusFound)
		return
	}
	if err := s.st.DeleteSessionsByUserID(ctx, target.ID); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "强制登出失败")
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("强制登出失败"), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "密码已重置，并已强制登出该用户")
		return
	}
	http.Redirect(w, r, "/admin/users?msg="+url.QueryEscape("密码已重置，并已强制登出该用户"), http.StatusFound)
}

func (s *Server) DeleteUser(w http.ResponseWriter, r *http.Request) {
	u, _, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !isRoot {
		http.Error(w, "无权限", http.StatusForbidden)
		return
	}
	userID, err := parseInt64(r.PathValue("user_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	target, err := s.st.GetUserByID(r.Context(), userID)
	if err != nil {
		http.Error(w, "用户不存在", http.StatusNotFound)
		return
	}
	if target.ID == u.ID {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "不能删除当前登录用户")
			return
		}
		http.Error(w, "不能删除当前登录用户", http.StatusBadRequest)
		return
	}
	if err := s.st.DeleteUser(r.Context(), target.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if isAjax(r) {
				ajaxError(w, http.StatusNotFound, "用户不存在")
				return
			}
			http.Error(w, "用户不存在", http.StatusNotFound)
			return
		}
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "删除失败")
			return
		}
		http.Error(w, "删除失败", http.StatusInternalServerError)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已删除")
		return
	}
	http.Redirect(w, r, "/admin/users", http.StatusFound)
}

func (s *Server) Channels(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	loc, tzName := s.adminTimeLocation(r.Context())
	nowRuntime := time.Now()

	nowUTC := time.Now().UTC()
	nowLocal := nowUTC.In(loc)
	todayStartLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	todayStr := todayStartLocal.Format("2006-01-02")

	q := r.URL.Query()
	startStr := strings.TrimSpace(q.Get("start"))
	endStr := strings.TrimSpace(q.Get("end"))
	if startStr == "" {
		startStr = todayStr
	}
	if endStr == "" {
		endStr = startStr
	}
	sinceLocal, err := time.ParseInLocation("2006-01-02", startStr, loc)
	if err != nil {
		http.Error(w, "start 不合法（格式：YYYY-MM-DD）", http.StatusBadRequest)
		return
	}
	endDateLocal, err := time.ParseInLocation("2006-01-02", endStr, loc)
	if err != nil {
		http.Error(w, "end 不合法（格式：YYYY-MM-DD）", http.StatusBadRequest)
		return
	}
	if sinceLocal.After(endDateLocal) {
		http.Error(w, "start 不能晚于 end", http.StatusBadRequest)
		return
	}
	if endDateLocal.After(todayStartLocal) {
		endDateLocal = todayStartLocal
		endStr = todayStr
	}
	untilLocal := endDateLocal.AddDate(0, 0, 1)
	if endStr == todayStr {
		untilLocal = nowLocal
	}

	since := sinceLocal.UTC()
	until := untilLocal.UTC()

	rawUsage, err := s.st.GetUsageStatsByChannelRange(r.Context(), since, until)
	if err != nil {
		http.Error(w, "渠道用量统计失败", http.StatusInternalServerError)
		return
	}
	usageByChannelID := make(map[int64]store.ChannelUsageStats, len(rawUsage))
	for _, row := range rawUsage {
		usageByChannelID[row.ChannelID] = row
	}

	channels, err := s.st.ListUpstreamChannels(r.Context())
	if err != nil {
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}

	ms, err := s.st.ListManagedModels(r.Context())
	if err != nil {
		http.Error(w, "查询模型失败", http.StatusInternalServerError)
		return
	}
	modelViews := make([]managedModelView, 0, len(ms))
	for _, m := range ms {
		modelViews = append(modelViews, toManagedModelView(m, loc))
	}
	channelNameByID := make(map[int64]string, len(channels))
	for _, ch := range channels {
		channelNameByID[ch.ID] = ch.Name
	}

	var cvs []adminChannelView
	for _, ch := range channels {
		ep, err := s.st.GetUpstreamEndpointByChannelID(r.Context(), ch.ID)
		if err != nil && errors.Is(err, sql.ErrNoRows) {
			def := defaultEndpointBaseURL(ch.Type)
			if _, err := security.ValidateBaseURL(def); err == nil {
				ep, _ = s.st.SetUpstreamEndpointBaseURL(r.Context(), ch.ID, def)
			}
		}

		var channelCreds []adminCredentialView
		switch ch.Type {
		case store.UpstreamTypeOpenAICompatible:
			rawCreds, err := s.st.ListOpenAICompatibleCredentialsByEndpoint(r.Context(), ep.ID)
			if err != nil {
				http.Error(w, "查询失败", http.StatusInternalServerError)
				return
			}
			channelCreds = make([]adminCredentialView, 0, len(rawCreds))
			for _, c := range rawCreds {
				maskedKey := "-"
				if c.APIKeyHint != nil && *c.APIKeyHint != "" {
					hint := *c.APIKeyHint
					if len(hint) > 4 {
						maskedKey = "..." + hint[len(hint)-4:]
					} else {
						maskedKey = hint
					}
				}
				channelCreds = append(channelCreds, adminCredentialView{
					ID:         c.ID,
					Name:       c.Name,
					APIKeyHint: c.APIKeyHint,
					MaskedKey:  maskedKey,
					Status:     c.Status,
					Runtime:    adminRuntimeLimitsView{},
				})
			}
		case store.UpstreamTypeAnthropic:
			rawCreds, err := s.st.ListAnthropicCredentialsByEndpoint(r.Context(), ep.ID)
			if err != nil {
				http.Error(w, "查询失败", http.StatusInternalServerError)
				return
			}
			channelCreds = make([]adminCredentialView, 0, len(rawCreds))
			for _, c := range rawCreds {
				maskedKey := "-"
				if c.APIKeyHint != nil && *c.APIKeyHint != "" {
					hint := *c.APIKeyHint
					if len(hint) > 4 {
						maskedKey = "..." + hint[len(hint)-4:]
					} else {
						maskedKey = hint
					}
				}
				channelCreds = append(channelCreds, adminCredentialView{
					ID:         c.ID,
					Name:       c.Name,
					APIKeyHint: c.APIKeyHint,
					MaskedKey:  maskedKey,
					Status:     c.Status,
					Runtime:    adminRuntimeLimitsView{},
				})
			}
		default:
		}

		var channelAccounts []adminCodexAccountView
		if ch.Type == store.UpstreamTypeCodexOAuth {
			accs, err := s.st.ListCodexOAuthAccountsByEndpoint(r.Context(), ep.ID)
			if err != nil {
				http.Error(w, "查询失败", http.StatusInternalServerError)
				return
			}

			now := time.Now()
			channelAccounts = make([]adminCodexAccountView, 0, len(accs))
			for _, a := range accs {
				v := adminCodexAccountView{
					ID:                      a.ID,
					AccountID:               a.AccountID,
					Email:                   a.Email,
					Status:                  a.Status,
					InCooldown:              a.CooldownUntil != nil && now.Before(*a.CooldownUntil),
					ExpiresAt:               formatTimePtrIn(a.ExpiresAt, time.RFC3339, loc),
					LastRefreshAt:           formatTimePtrIn(a.LastRefreshAt, time.RFC3339, loc),
					CooldownUntil:           formatTimePtrIn(a.CooldownUntil, time.RFC3339, loc),
					PlanType:                "-",
					SubscriptionActiveStart: "-",
					SubscriptionActiveUntil: "-",
					SubscriptionDaysLeft:    "-",
					QuotaCredits:            "-",
					QuotaPrimary:            "-",
					QuotaSecondary:          "-",
					QuotaUpdatedAt:          formatTimePtrIn(a.QuotaUpdatedAt, time.RFC3339, loc),
					QuotaPrimaryDetail:      "-",
					QuotaSecondaryDetail:    "-",
				}
				if s.sched != nil {
					rv := adminRuntimeLimitsView{
						Available: true,
					}
					key := fmt.Sprintf("%s:%d", scheduler.CredentialTypeCodex, a.ID)
					stats := s.sched.RuntimeCredentialStats(key)
					rv.RPM = stats.RPM
					rv.TPM = stats.TPM
					rv.Sessions = stats.Sessions
					rv.FailScore = stats.FailScore
					if stats.CoolingUntil != nil {
						rv.CoolingUntil = formatTimeIn(*stats.CoolingUntil, time.RFC3339, loc)
					}
					v.Runtime = rv
				}
				v.QuotaCredits = formatCreditsForView(a.QuotaCreditsHasCredits, a.QuotaCreditsUnlimited, a.QuotaCreditsBalance)
				v.QuotaPrimary = formatRateLimitForView(a.QuotaPrimaryUsedPercent, a.QuotaPrimaryResetAt, loc)
				v.QuotaSecondary = formatRateLimitForView(a.QuotaSecondaryUsedPercent, a.QuotaSecondaryResetAt, loc)
				if a.QuotaPrimaryUsedPercent != nil {
					v.QuotaPrimaryUsed = *a.QuotaPrimaryUsedPercent
					v.QuotaPrimaryShow = true
					v.QuotaPrimaryDetail = formatQuotaWindowDetail(a.QuotaPrimaryUsedPercent, a.QuotaPrimaryResetAt, codexTeamQuotaCapUSD5H, loc)
				}
				if a.QuotaSecondaryUsedPercent != nil {
					v.QuotaSecondaryUsed = *a.QuotaSecondaryUsedPercent
					v.QuotaSecondaryShow = true
					v.QuotaSecondaryDetail = formatQuotaWindowDetail(a.QuotaSecondaryUsedPercent, a.QuotaSecondaryResetAt, codexTeamQuotaCapUSDWeek, loc)
				}
				if a.QuotaError != nil && strings.TrimSpace(*a.QuotaError) != "" {
					v.QuotaError = strings.TrimSpace(*a.QuotaError)
				}
				sec, err := s.st.GetCodexOAuthSecret(r.Context(), a.ID)
				if err == nil && sec.IDToken != nil && *sec.IDToken != "" {
					if claims, err := codexoauth.ParseIDTokenClaims(*sec.IDToken); err == nil && claims != nil {
						if pt := strings.TrimSpace(claims.PlanType); pt != "" {
							v.PlanType = pt
						}
						v.SubscriptionActiveStart = formatClaimTimeIn(claims.SubscriptionActiveStart, loc)
						v.SubscriptionActiveUntil = formatClaimTimeIn(claims.SubscriptionActiveUntil, loc)

						start, startOK := getClaimFloat64(claims.SubscriptionActiveStart)
						until, untilOK := getClaimFloat64(claims.SubscriptionActiveUntil)
						if startOK && untilOK {
							nowTs := float64(time.Now().Unix())
							if nowTs >= start && nowTs < until {
								v.SubscriptionActive = true
								total := until - start
								if total > 0 {
									v.SubscriptionPercent = int(((nowTs - start) / total) * 100)
									if v.SubscriptionPercent < 0 {
										v.SubscriptionPercent = 0
									}
									if v.SubscriptionPercent > 100 {
										v.SubscriptionPercent = 100
									}
								}
								v.SubscriptionDaysLeft = formatDaysLeftSeconds(until - nowTs)
							}
						}
					}
				}
				channelAccounts = append(channelAccounts, v)
			}
		}

		cms, err := s.st.ListChannelModelsByChannelID(r.Context(), ch.ID)
		if err != nil {
			http.Error(w, "查询失败", http.StatusInternalServerError)
			return
		}
		channelModels := make([]channelModelView, 0, len(cms))
		for _, m := range cms {
			channelModels = append(channelModels, toChannelModelView(m, loc))
		}
		channelRuntime := adminChannelRuntimeView{Available: s.sched != nil}
		if s.sched != nil {
			rt := s.sched.RuntimeChannelStats(ch.ID)
			channelRuntime.FailScore = rt.FailScore
			channelRuntime.BanStreak = rt.BanStreak
			if rt.BannedUntil != nil {
				channelRuntime.BannedActive = true
				channelRuntime.BannedUntil = formatTimeIn(*rt.BannedUntil, time.RFC3339, loc)
				channelRuntime.BannedRemaining = formatRemainingUntilZH(*rt.BannedUntil, nowRuntime)
			}
			channelRuntime.PinnedActive = rt.Pointer
		}

		us := usageByChannelID[ch.ID]
		cvs = append(cvs, adminChannelView{
			Channel:  ch,
			Endpoint: ep,
			Usage: adminChannelUsageView{
				CommittedUSD: formatUSDPlain(us.CommittedUSD),
				Tokens:       us.Tokens,
				CacheRatio:   fmt.Sprintf("%.1f%%", us.CacheRatio*100),
			},
			Runtime:       channelRuntime,
			Credentials:   channelCreds,
			Accounts:      channelAccounts,
			ChannelModels: channelModels,
		})
	}

	channelGroups, err := s.st.ListChannelGroups(r.Context())
	if err != nil {
		http.Error(w, "查询渠道分组失败", http.StatusInternalServerError)
		return
	}
	var usedGroups []string
	for _, ch := range channels {
		usedGroups = append(usedGroups, splitGroups(ch.Groups)...)
	}
	channelGroups = mergeChannelGroupsOptions(channelGroups, usedGroups)

	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}
	schedulerRuntime := adminSchedulerRuntimeView{Available: s.sched != nil}
	if s.sched != nil {
		if id, ok := s.sched.PinnedChannel(); ok {
			schedulerRuntime.PinnedActive = true
			schedulerRuntime.PinnedChannelID = id
			if name := strings.TrimSpace(channelNameByID[id]); name != "" {
				schedulerRuntime.PinnedChannel = fmt.Sprintf("%s (#%d)", name, id)
			} else {
				schedulerRuntime.PinnedChannel = fmt.Sprintf("渠道 #%d", id)
			}
		}
		if sel, at, ok := s.sched.LastSuccess(); ok {
			schedulerRuntime.LastSuccessActive = true
			schedulerRuntime.LastSuccessChannelID = sel.ChannelID
			if name := strings.TrimSpace(channelNameByID[sel.ChannelID]); name != "" {
				schedulerRuntime.LastSuccessChannel = fmt.Sprintf("%s (#%d)", name, sel.ChannelID)
			} else if sel.ChannelID > 0 {
				schedulerRuntime.LastSuccessChannel = fmt.Sprintf("渠道 #%d", sel.ChannelID)
			}
			schedulerRuntime.LastSuccessAt = formatTimeIn(at, "2006-01-02 15:04:05", loc)
		}
	}
	s.render(w, "admin_channels", s.withFeatures(r.Context(), templateData{
		Title:                  "上游渠道 - Realms",
		Error:                  errMsg,
		Notice:                 notice,
		User:                   u,
		IsRoot:                 isRoot,
		CSRFToken:              csrf,
		AdminTimeZoneEffective: tzName,
		UsageStart:             startStr,
		UsageEnd:               endStr,
		ChannelGroups:          channelGroups,
		Channels:               channels,
		ChannelViews:           cvs,
		ManagedModels:          modelViews,
		SchedulerRuntime:       schedulerRuntime,
	}))
}

func (s *Server) CreateChannel(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	// 兼容：部分页面/代理可能错误地把“测试渠道”表单提交到 /admin/channels（例如 action 丢失或被重写）。
	// 在这种情况下，按测试逻辑处理，避免误触发“保存分组/创建渠道”。
	if strings.TrimSpace(r.FormValue("_intent")) == "test_channel" {
		s.TestChannel(w, r)
		return
	}

	// 兼容：部分页面/旧版本可能把“修改分组”表单误提交到 /admin/channels（缺少 action 时默认如此）。
	// 当携带 channel_id 时，按“更新渠道分组”处理，而不是走“创建渠道”校验。
	if rawID := strings.TrimSpace(r.FormValue("channel_id")); rawID != "" && isChannelGroupsUpdateForm(r.Form) {
		channelID, err := parseInt64(rawID)
		if err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "channel_id 不合法")
				return
			}
			http.Error(w, "参数错误", http.StatusBadRequest)
			return
		}
		returnTo := safeAdminReturnTo(r.FormValue("return_to"), "/admin/channels")
		ch, err := s.st.GetUpstreamChannelByID(r.Context(), channelID)
		if err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusNotFound, "channel 不存在")
				return
			}
			http.Error(w, "channel 不存在", http.StatusNotFound)
			return
		}
		s.updateChannelGroups(w, r, ch, returnTo)
		return
	}

	typ := strings.TrimSpace(r.FormValue("type"))
	name := strings.TrimSpace(r.FormValue("name"))
	groups, err := normalizeGroupsValues(r.Form["groups"])
	if err != nil {
		http.Error(w, "groups 不合法: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateChannelGroupsSelectable(r.Context(), s.st, groups); err != nil {
		http.Error(w, "groups 不合法: "+err.Error(), http.StatusBadRequest)
		return
	}
	baseURL := strings.TrimSpace(r.FormValue("base_url"))
	priority, _ := parseInt(r.FormValue("priority"))
	promotion := strings.TrimSpace(r.FormValue("promotion")) == "1"
	allowServiceTier := strings.TrimSpace(r.FormValue("allow_service_tier")) == "1"
	disableStore := strings.TrimSpace(r.FormValue("disable_store")) == "1"
	allowSafetyIdentifier := strings.TrimSpace(r.FormValue("allow_safety_identifier")) == "1"
	if typ == "" || name == "" || baseURL == "" {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	switch typ {
	case store.UpstreamTypeOpenAICompatible, store.UpstreamTypeAnthropic:
		// ok
	case store.UpstreamTypeCodexOAuth:
		http.Error(w, "codex_oauth Channel 为内置，不允许创建", http.StatusBadRequest)
		return
	default:
		http.Error(w, "不支持的渠道类型", http.StatusBadRequest)
		return
	}
	if _, err := security.ValidateBaseURL(baseURL); err != nil {
		http.Error(w, "base_url 不合法："+err.Error(), http.StatusBadRequest)
		return
	}

	id, err := s.st.CreateUpstreamChannel(r.Context(), typ, name, groups, priority, promotion, allowServiceTier, disableStore, allowSafetyIdentifier)
	if err != nil {
		http.Error(w, "创建失败", http.StatusInternalServerError)
		return
	}
	if _, err := s.st.SetUpstreamEndpointBaseURL(r.Context(), id, baseURL); err != nil {
		http.Error(w, "创建 Endpoint 失败", http.StatusInternalServerError)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已创建")
		return
	}
	http.Redirect(w, r, "/admin/channels", http.StatusFound)
}

func (s *Server) updateChannelGroups(w http.ResponseWriter, r *http.Request, ch store.UpstreamChannel, returnTo string) {
	groups, err := normalizeGroupsValues(r.Form["groups"])
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "groups 不合法: "+err.Error())
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("groups 不合法: "+err.Error()), http.StatusFound)
		return
	}
	if groups != ch.Groups {
		if err := validateChannelGroupsSelectable(r.Context(), s.st, groups); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "groups 不合法: "+err.Error())
				return
			}
			http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("groups 不合法: "+err.Error()), http.StatusFound)
			return
		}
		if err := s.st.SetUpstreamChannelGroups(r.Context(), ch.ID, groups); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "保存失败")
				return
			}
			http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("保存失败"), http.StatusFound)
			return
		}
	}
	if isAjax(r) {
		ajaxOK(w, "分组已保存")
		return
	}
	http.Redirect(w, r, returnTo+"?msg="+url.QueryEscape("分组已保存"), http.StatusFound)
}

func (s *Server) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "表单解析失败")
			return
		}
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	returnTo := safeAdminReturnTo(r.FormValue("return_to"), "/admin/channels")

	// 兼容：当路由/代理把 /admin/channels/test 错误匹配为 /admin/channels/{channel_id} 时，
	// path 参数会变成 "test" 导致 channel_id 解析失败。此处按表单意图转交测试逻辑。
	if strings.TrimSpace(r.FormValue("_intent")) == "test_channel" {
		s.TestChannel(w, r)
		return
	}

	rawID := strings.TrimSpace(r.PathValue("channel_id"))
	if rawID == "" {
		rawID = strings.TrimSpace(r.FormValue("channel_id"))
	}
	channelID, err := parseInt64(rawID)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "channel_id 不合法")
			return
		}
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	ch, err := s.st.GetUpstreamChannelByID(r.Context(), channelID)
	if err != nil {
		http.Error(w, "channel 不存在", http.StatusNotFound)
		return
	}

	s.updateChannelGroups(w, r, ch, returnTo)
}

func (s *Server) ReorderChannels(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	var ids []int64
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		http.Error(w, "无效的 JSON 数据", http.StatusBadRequest)
		return
	}

	if len(ids) > 0 {
		if err := s.st.ReorderUpstreamChannels(r.Context(), ids); err != nil {
			http.Error(w, "排序保存失败", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	channelID, err := parseInt64(r.PathValue("channel_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	ch, err := s.st.GetUpstreamChannelByID(r.Context(), channelID)
	if err != nil {
		http.Error(w, "channel 不存在", http.StatusNotFound)
		return
	}
	if ch.Type == store.UpstreamTypeCodexOAuth {
		http.Error(w, "codex_oauth Channel 为内置，不允许删除", http.StatusBadRequest)
		return
	}
	if err := s.st.DeleteUpstreamChannel(r.Context(), ch.ID); err != nil {
		http.Error(w, "删除失败", http.StatusInternalServerError)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已删除")
		return
	}
	http.Redirect(w, r, "/admin/channels", http.StatusFound)
}

func (s *Server) Endpoints(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	channelID, err := parseInt64(r.PathValue("channel_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	ch, err := s.st.GetUpstreamChannelByID(r.Context(), channelID)
	if err != nil {
		http.Error(w, "channel 不存在", http.StatusNotFound)
		return
	}

	hash := "#keys"
	if ch.Type == store.UpstreamTypeCodexOAuth {
		hash = "#accounts"
	}

	q := r.URL.Query()
	q.Set("open_channel_settings", strconv.FormatInt(ch.ID, 10))

	target := "/admin/channels"
	if enc := q.Encode(); enc != "" {
		target += "?" + enc
	}
	target += hash

	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) DeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	http.Error(w, "每个 Channel 必须且仅能配置一个 Endpoint，不支持删除 Endpoint", http.StatusBadRequest)
}

func (s *Server) CreateEndpoint(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	channelID, err := parseInt64(r.PathValue("channel_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	ch, err := s.st.GetUpstreamChannelByID(r.Context(), channelID)
	if err != nil {
		http.Error(w, "channel 不存在", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "表单解析失败")
			return
		}
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	// 兼容：部分前端/旧页面可能会把“保存分组”的表单误提交到 /endpoints。
	// 这种情况下应按“更新渠道分组”处理，避免误触发 codex_oauth base_url 限制。
	if isChannelGroupsUpdateForm(r.Form) {
		s.UpdateChannel(w, r)
		return
	}

	if ch.Type == store.UpstreamTypeCodexOAuth {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "codex_oauth 为内置渠道，不允许修改 base_url")
			return
		}
		http.Error(w, "codex_oauth 为内置渠道，不允许修改 base_url", http.StatusBadRequest)
		return
	}
	baseURL := strings.TrimSpace(r.FormValue("base_url"))
	if _, err := security.ValidateBaseURL(baseURL); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "base_url 不合法："+err.Error())
			return
		}
		http.Error(w, "base_url 不合法："+err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.st.SetUpstreamEndpointBaseURL(r.Context(), ch.ID, baseURL); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "保存失败")
			return
		}
		http.Error(w, "保存失败", http.StatusInternalServerError)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "基础地址已保存")
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d", ch.ID), http.StatusFound)
}

func (s *Server) DeleteOpenAICredential(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	credentialID, err := parseInt64(r.PathValue("credential_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	cred, err := s.st.GetOpenAICompatibleCredentialByID(r.Context(), credentialID)
	if err != nil {
		http.Error(w, "credential 不存在", http.StatusNotFound)
		return
	}
	ep, _, err := s.loadEndpointAndChannel(r, cred.EndpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}
	if err := s.st.DeleteOpenAICompatibleCredential(r.Context(), cred.ID); err != nil {
		http.Error(w, "删除失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d#keys", ep.ChannelID), http.StatusFound)
}

func (s *Server) CreateOpenAICredential(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	endpointID, err := parseInt64(r.PathValue("endpoint_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	ep, _, err := s.loadEndpointAndChannel(r, endpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	if apiKey == "" {
		http.Error(w, "api_key 不能为空", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	var namePtr *string
	if name != "" {
		namePtr = &name
	}
	if _, _, err := s.st.CreateOpenAICompatibleCredential(r.Context(), ep.ID, namePtr, apiKey); err != nil {
		http.Error(w, "创建失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d#keys", ep.ChannelID), http.StatusFound)
}

func (s *Server) DeleteAnthropicCredential(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	credentialID, err := parseInt64(r.PathValue("credential_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	cred, err := s.st.GetAnthropicCredentialByID(r.Context(), credentialID)
	if err != nil {
		http.Error(w, "credential 不存在", http.StatusNotFound)
		return
	}
	ep, ch, err := s.loadEndpointAndChannel(r, cred.EndpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}
	if ch.Type != store.UpstreamTypeAnthropic {
		http.Error(w, "credential 不匹配 channel 类型", http.StatusBadRequest)
		return
	}
	if err := s.st.DeleteAnthropicCredential(r.Context(), cred.ID); err != nil {
		http.Error(w, "删除失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d#keys", ep.ChannelID), http.StatusFound)
}

func (s *Server) CreateAnthropicCredential(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	endpointID, err := parseInt64(r.PathValue("endpoint_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	ep, ch, err := s.loadEndpointAndChannel(r, endpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}
	if ch.Type != store.UpstreamTypeAnthropic {
		http.Error(w, "endpoint 不匹配 channel 类型", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	if apiKey == "" {
		http.Error(w, "api_key 不能为空", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	var namePtr *string
	if name != "" {
		namePtr = &name
	}
	if _, _, err := s.st.CreateAnthropicCredential(r.Context(), ep.ID, namePtr, apiKey); err != nil {
		http.Error(w, "创建失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d#keys", ep.ChannelID), http.StatusFound)
}

func (s *Server) CodexAccounts(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	endpointID, err := parseInt64(r.PathValue("endpoint_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	ep, _, err := s.loadEndpointAndChannel(r, endpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}

	q := r.URL.Query()
	q.Set("open_channel_settings", fmt.Sprintf("%d", ep.ChannelID))
	target := "/admin/channels"
	if enc := q.Encode(); enc != "" {
		target += "?" + enc
	}
	target += "#accounts"
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) DeleteCodexAccount(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	accountID, err := parseInt64(r.PathValue("account_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	acc, err := s.st.GetCodexOAuthAccountByID(r.Context(), accountID)
	if err != nil {
		http.Error(w, "account 不存在", http.StatusNotFound)
		return
	}
	ep, _, err := s.loadEndpointAndChannel(r, acc.EndpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}
	if err := s.st.DeleteCodexOAuthAccount(r.Context(), acc.ID); err != nil {
		http.Error(w, "删除失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d#accounts", ep.ChannelID), http.StatusFound)
}

func (s *Server) CreateCodexAccount(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	endpointID, err := parseInt64(r.PathValue("endpoint_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	ep, _, err := s.loadEndpointAndChannel(r, endpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	accountID := strings.TrimSpace(r.FormValue("account_id"))
	accessToken := strings.TrimSpace(r.FormValue("access_token"))
	refreshToken := strings.TrimSpace(r.FormValue("refresh_token"))
	if accountID == "" || accessToken == "" || refreshToken == "" {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	var emailPtr *string
	if email != "" {
		emailPtr = &email
	}
	if _, err := s.st.CreateCodexOAuthAccount(r.Context(), ep.ID, accountID, emailPtr, accessToken, refreshToken, nil, nil); err != nil {
		http.Error(w, "创建失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d#accounts", ep.ChannelID), http.StatusFound)
}

func (s *Server) StartCodexOAuth(w http.ResponseWriter, r *http.Request) {
	u, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if s.codexOAuth == nil {
		http.Error(w, "Codex OAuth 未启用", http.StatusBadRequest)
		return
	}
	endpointID, err := parseInt64(r.PathValue("endpoint_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	ep, _, err := s.loadEndpointAndChannel(r, endpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}
	authURL, err := s.codexOAuth.Start(r.Context(), ep.ID, u.ID)
	if err != nil {
		msg := codexoauth.UserMessage(err)
		http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d&oauth=error&err=%s#accounts", ep.ChannelID, url.QueryEscape(msg)), http.StatusFound)
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) CompleteCodexOAuth(w http.ResponseWriter, r *http.Request) {
	u, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if s.codexOAuth == nil {
		http.Error(w, "Codex OAuth 未启用", http.StatusBadRequest)
		return
	}
	endpointID, err := parseInt64(r.PathValue("endpoint_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	ep, _, err := s.loadEndpointAndChannel(r, endpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	callbackURL := strings.TrimSpace(r.FormValue("callback_url"))
	parsed, err := codexoauth.ParseOAuthCallback(callbackURL)
	if err != nil || parsed == nil {
		msg := "回调 URL 解析失败，请粘贴浏览器地址栏中的完整 URL（包含 code/state）"
		http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d&oauth=error&err=%s#accounts", ep.ChannelID, url.QueryEscape(msg)), http.StatusFound)
		return
	}
	if strings.TrimSpace(parsed.Error) != "" {
		msg := "OAuth 回调失败：上游返回错误"
		if desc := strings.TrimSpace(parsed.ErrorDescription); desc != "" {
			msg += " - " + desc
		} else {
			msg += " - " + strings.TrimSpace(parsed.Error)
		}
		http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d&oauth=error&err=%s#accounts", ep.ChannelID, url.QueryEscape(msg)), http.StatusFound)
		return
	}
	if err := s.codexOAuth.Complete(r.Context(), ep.ID, u.ID, parsed.State, parsed.Code); err != nil {
		msg := codexoauth.UserMessage(err)
		http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d&oauth=error&err=%s#accounts", ep.ChannelID, url.QueryEscape(msg)), http.StatusFound)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d#accounts", ep.ChannelID), http.StatusFound)
}

func (s *Server) OAuthApps(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !isRoot {
		http.Error(w, "权限不足", http.StatusForbidden)
		return
	}

	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}

	apps, err := s.st.ListOAuthApps(r.Context())
	if err != nil {
		http.Error(w, "查询 OAuth Apps 失败", http.StatusInternalServerError)
		return
	}
	views := make([]oauthAppView, 0, len(apps))
	for _, a := range apps {
		uris, _ := s.st.ListOAuthAppRedirectURIs(r.Context(), a.ID)
		statusLabel := "停用"
		if a.Status == store.OAuthAppStatusEnabled {
			statusLabel = "启用"
		}
		views = append(views, oauthAppView{
			ID:               a.ID,
			ClientID:         a.ClientID,
			Name:             a.Name,
			Status:           a.Status,
			StatusLabel:      statusLabel,
			HasSecret:        len(a.ClientSecretHash) > 0,
			RedirectURIs:     uris,
			RedirectURIsText: strings.Join(uris, "\n"),
		})
	}

	s.render(w, "admin_oauth_apps", s.withFeatures(r.Context(), templateData{
		Title:                "OAuth Apps - Realms",
		Error:                errMsg,
		Notice:               notice,
		User:                 u,
		IsRoot:               isRoot,
		CSRFToken:            csrf,
		OAuthApps:            views,
		SiteBaseURLEffective: s.baseURLFromRequest(r),
	}))
}

func (s *Server) CreateOAuthApp(w http.ResponseWriter, r *http.Request) {
	u, _, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !isRoot {
		http.Error(w, "权限不足", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/admin/oauth-apps?err="+url.QueryEscape("应用名称不能为空"), http.StatusFound)
		return
	}
	status := store.OAuthAppStatusEnabled
	if strings.TrimSpace(r.FormValue("status")) == "0" {
		status = store.OAuthAppStatusDisabled
	}

	redirectURIs, err := parseRedirectURILines(r.FormValue("redirect_uris"))
	if err != nil {
		http.Redirect(w, r, "/admin/oauth-apps?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}

	clientID, err := auth.NewRandomToken("oa_", 16)
	if err != nil {
		http.Error(w, "生成 client_id 失败", http.StatusInternalServerError)
		return
	}
	clientSecret, err := auth.NewRandomToken("oas_", 32)
	if err != nil {
		http.Error(w, "生成 client_secret 失败", http.StatusInternalServerError)
		return
	}
	secretHash, err := auth.HashPassword(clientSecret)
	if err != nil {
		http.Error(w, "生成 client_secret 失败", http.StatusInternalServerError)
		return
	}

	appID, err := s.st.CreateOAuthApp(r.Context(), clientID, name, secretHash, status)
	if err != nil {
		http.Redirect(w, r, "/admin/oauth-apps?err="+url.QueryEscape("创建失败"), http.StatusFound)
		return
	}
	if err := s.st.ReplaceOAuthAppRedirectURIs(r.Context(), appID, redirectURIs); err != nil {
		http.Redirect(w, r, "/admin/oauth-apps?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}

	a, err := s.st.GetOAuthAppByID(r.Context(), appID)
	if err != nil {
		http.Error(w, "查询应用失败", http.StatusInternalServerError)
		return
	}
	uris, _ := s.st.ListOAuthAppRedirectURIs(r.Context(), a.ID)
	statusLabel := "停用"
	if a.Status == store.OAuthAppStatusEnabled {
		statusLabel = "启用"
	}
	view := oauthAppView{
		ID:               a.ID,
		ClientID:         a.ClientID,
		Name:             a.Name,
		Status:           a.Status,
		StatusLabel:      statusLabel,
		HasSecret:        len(a.ClientSecretHash) > 0,
		RedirectURIs:     uris,
		RedirectURIsText: strings.Join(uris, "\n"),
	}

	s.render(w, "admin_oauth_app_secret", s.withFeatures(r.Context(), templateData{
		Title:                "Client Secret 已生成 - Realms",
		User:                 u,
		IsRoot:               isRoot,
		OAuthApp:             &view,
		OAuthAppSecret:       clientSecret,
		SiteBaseURLEffective: s.baseURLFromRequest(r),
	}))
}

func (s *Server) OAuthApp(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !isRoot {
		http.Error(w, "权限不足", http.StatusForbidden)
		return
	}

	appID, err := parseInt64(r.PathValue("app_id"))
	if err != nil || appID == 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	a, err := s.st.GetOAuthAppByID(r.Context(), appID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "查询应用失败", http.StatusInternalServerError)
		return
	}
	uris, _ := s.st.ListOAuthAppRedirectURIs(r.Context(), a.ID)

	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}

	statusLabel := "停用"
	if a.Status == store.OAuthAppStatusEnabled {
		statusLabel = "启用"
	}
	view := oauthAppView{
		ID:               a.ID,
		ClientID:         a.ClientID,
		Name:             a.Name,
		Status:           a.Status,
		StatusLabel:      statusLabel,
		HasSecret:        len(a.ClientSecretHash) > 0,
		RedirectURIs:     uris,
		RedirectURIsText: strings.Join(uris, "\n"),
	}

	s.render(w, "admin_oauth_app", s.withFeatures(r.Context(), templateData{
		Title:     "OAuth App - Realms",
		Error:     errMsg,
		Notice:    notice,
		User:      u,
		IsRoot:    isRoot,
		CSRFToken: csrf,
		OAuthApp:  &view,
	}))
}

func (s *Server) UpdateOAuthApp(w http.ResponseWriter, r *http.Request) {
	_, _, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !isRoot {
		http.Error(w, "权限不足", http.StatusForbidden)
		return
	}
	appID, err := parseInt64(r.PathValue("app_id"))
	if err != nil || appID == 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, fmt.Sprintf("/admin/oauth-apps/%d?err=%s", appID, url.QueryEscape("应用名称不能为空")), http.StatusFound)
		return
	}
	status := store.OAuthAppStatusEnabled
	if strings.TrimSpace(r.FormValue("status")) == "0" {
		status = store.OAuthAppStatusDisabled
	}
	redirectURIs, err := parseRedirectURILines(r.FormValue("redirect_uris"))
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/oauth-apps/%d?err=%s", appID, url.QueryEscape(err.Error())), http.StatusFound)
		return
	}
	if err := s.st.UpdateOAuthApp(r.Context(), appID, name, status); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/oauth-apps/%d?err=%s", appID, url.QueryEscape("保存失败")), http.StatusFound)
		return
	}
	if err := s.st.ReplaceOAuthAppRedirectURIs(r.Context(), appID, redirectURIs); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/oauth-apps/%d?err=%s", appID, url.QueryEscape(err.Error())), http.StatusFound)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/oauth-apps/%d?msg=%s", appID, url.QueryEscape("已保存")), http.StatusFound)
}

func (s *Server) RotateOAuthAppSecret(w http.ResponseWriter, r *http.Request) {
	u, _, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !isRoot {
		http.Error(w, "权限不足", http.StatusForbidden)
		return
	}
	appID, err := parseInt64(r.PathValue("app_id"))
	if err != nil || appID == 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	a, err := s.st.GetOAuthAppByID(r.Context(), appID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "查询应用失败", http.StatusInternalServerError)
		return
	}

	clientSecret, err := auth.NewRandomToken("oas_", 32)
	if err != nil {
		http.Error(w, "生成 client_secret 失败", http.StatusInternalServerError)
		return
	}
	secretHash, err := auth.HashPassword(clientSecret)
	if err != nil {
		http.Error(w, "生成 client_secret 失败", http.StatusInternalServerError)
		return
	}
	if err := s.st.UpdateOAuthAppSecretHash(r.Context(), appID, secretHash); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/oauth-apps/%d?err=%s", appID, url.QueryEscape("轮换失败")), http.StatusFound)
		return
	}

	uris, _ := s.st.ListOAuthAppRedirectURIs(r.Context(), a.ID)
	statusLabel := "停用"
	if a.Status == store.OAuthAppStatusEnabled {
		statusLabel = "启用"
	}
	view := oauthAppView{
		ID:               a.ID,
		ClientID:         a.ClientID,
		Name:             a.Name,
		Status:           a.Status,
		StatusLabel:      statusLabel,
		HasSecret:        true,
		RedirectURIs:     uris,
		RedirectURIsText: strings.Join(uris, "\n"),
	}

	s.render(w, "admin_oauth_app_secret", s.withFeatures(r.Context(), templateData{
		Title:                "Client Secret 已生成 - Realms",
		User:                 u,
		IsRoot:               isRoot,
		OAuthApp:             &view,
		OAuthAppSecret:       clientSecret,
		SiteBaseURLEffective: s.baseURLFromRequest(r),
	}))
}

func parseRedirectURILines(raw string) ([]string, error) {
	text := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		norm, err := store.NormalizeOAuthRedirectURI(line)
		if err != nil {
			return nil, err
		}
		out = append(out, norm)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("redirect_uri 不能为空")
	}
	return out, nil
}

func (s *Server) currentUser(r *http.Request) (userView, string, bool, error) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeSession || p.CSRFToken == nil {
		return userView{}, "", false, fmt.Errorf("未登录")
	}
	u, err := s.st.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		return userView{}, "", false, err
	}
	isRoot := u.Role == store.UserRoleRoot
	return userView{ID: u.ID, Email: u.Email, Role: u.Role}, *p.CSRFToken, isRoot, nil
}

func (s *Server) loadEndpointAndChannel(r *http.Request, endpointID int64) (store.UpstreamEndpoint, store.UpstreamChannel, error) {
	ep, err := s.st.GetUpstreamEndpointByID(r.Context(), endpointID)
	if err != nil {
		return store.UpstreamEndpoint{}, store.UpstreamChannel{}, err
	}
	ch, err := s.st.GetUpstreamChannelByID(r.Context(), ep.ChannelID)
	if err != nil {
		return store.UpstreamEndpoint{}, store.UpstreamChannel{}, err
	}
	return ep, ch, nil
}

func defaultEndpointBaseURL(channelType string) string {
	switch channelType {
	case store.UpstreamTypeCodexOAuth:
		return "https://chatgpt.com/backend-api/codex"
	case store.UpstreamTypeAnthropic:
		return "https://api.anthropic.com"
	default:
		return "https://api.openai.com"
	}
}

func parseInt64(s string) (int64, error) {
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("非数字")
		}
		n = n*10 + int64(ch-'0')
	}
	return n, nil
}

func parseInt(s string) (int, error) {
	n64, err := parseInt64(strings.TrimSpace(s))
	return int(n64), err
}

func isValidUserRole(role string) bool {
	switch role {
	case store.UserRoleUser, store.UserRoleRoot:
		return true
	default:
		return false
	}
}

func parseJWTClaims(raw string) (map[string]any, error) {
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid jwt")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func formatClaimTimeIn(v any, loc *time.Location) string {
	switch vv := v.(type) {
	case float64:
		if vv <= 0 {
			return "-"
		}
		return formatTimeIn(time.Unix(int64(vv), 0), time.RFC3339, loc)
	case int64:
		if vv <= 0 {
			return "-"
		}
		return formatTimeIn(time.Unix(vv, 0), time.RFC3339, loc)
	case string:
		s := strings.TrimSpace(vv)
		if s == "" {
			return "-"
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return formatTimeIn(t, time.RFC3339, loc)
		}
		return s
	default:
		return "-"
	}
}

func formatCreditsForView(hasCredits *bool, unlimited *bool, balance *string) string {
	if hasCredits != nil && !*hasCredits {
		return "-"
	}
	if unlimited != nil && *unlimited {
		return "Unlimited"
	}
	if balance == nil {
		return "-"
	}
	raw := strings.TrimSpace(*balance)
	if raw == "" {
		return "-"
	}
	if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return fmt.Sprintf("%d credits", v)
	}
	if v, err := strconv.ParseFloat(raw, 64); err == nil {
		return fmt.Sprintf("%d credits", int64(math.Round(v)))
	}
	return raw
}

func formatRateLimitForView(usedPercent *int, resetAt *time.Time, loc *time.Location) string {
	if usedPercent == nil {
		return "-"
	}
	left := 100 - *usedPercent
	if left < 0 {
		left = 0
	}
	if left > 100 {
		left = 100
	}
	if resetAt == nil {
		return fmt.Sprintf("%d%% left", left)
	}
	return fmt.Sprintf("%d%% left (reset %s)", left, formatTimePtrIn(resetAt, time.RFC3339, loc))
}

func formatDaysLeftSeconds(remainingSeconds float64) string {
	if remainingSeconds <= 0 {
		return "0"
	}
	if remainingSeconds < 24*60*60 {
		return "<1"
	}
	days := math.Floor(remainingSeconds / (24 * 60 * 60))
	if days < 1 {
		return "<1"
	}
	return strconv.Itoa(int(days))
}

const (
	codexTeamQuotaCapUSD5H   = 6.0
	codexTeamQuotaCapUSDWeek = 20.0
)

func formatQuotaWindowDetail(usedPercent *int, resetAt *time.Time, capUSD float64, loc *time.Location) string {
	if usedPercent == nil || capUSD <= 0 {
		return "-"
	}

	used := *usedPercent
	if used < 0 {
		used = 0
	}
	if used > 100 {
		used = 100
	}

	leftPercent := 100 - used
	if leftPercent < 0 {
		leftPercent = 0
	}
	if leftPercent > 100 {
		leftPercent = 100
	}

	remaining := capUSD * (float64(leftPercent) / 100.0)
	remaining = math.Round(remaining*100) / 100

	if resetAt == nil {
		return fmt.Sprintf("剩余 $%.2f/$%.2f", remaining, capUSD)
	}
	return fmt.Sprintf("剩余 $%.2f/$%.2f · 重置 %s", remaining, capUSD, formatTimePtrIn(resetAt, time.RFC3339, loc))
}

func getClaimFloat64(v any) (float64, bool) {
	switch vv := v.(type) {
	case float64:
		return vv, true
	case int64:
		return float64(vv), true
	case string:
		if t, err := time.Parse(time.RFC3339, vv); err == nil {
			return float64(t.Unix()), true
		}
		if f, err := strconv.ParseFloat(vv, 64); err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func (s *Server) baseURLFromRequest(r *http.Request) string {
	if r != nil {
		if v, ok, err := s.st.GetStringAppSetting(r.Context(), store.SettingSiteBaseURL); err == nil && ok {
			if normalized, err := config.NormalizeHTTPBaseURL(v, "site_base_url"); err == nil && normalized != "" {
				return normalized
			}
		}
	}
	if strings.TrimSpace(s.publicBaseURL) != "" {
		return s.publicBaseURL
	}
	return security.DeriveBaseURLFromRequest(r, s.trustProxyHeaders, s.trustedProxies)
}
