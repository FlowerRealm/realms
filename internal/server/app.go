// Package server 组装 HTTP 路由、依赖与中间件，使 main 保持简单可读。
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"realms/internal/admin"
	openaiapi "realms/internal/api/openai"
	"realms/internal/assets"
	"realms/internal/codexoauth"
	"realms/internal/config"
	"realms/internal/limits"
	"realms/internal/middleware"
	"realms/internal/proxylog"
	"realms/internal/quota"
	"realms/internal/scheduler"
	"realms/internal/store"
	"realms/internal/tickets"
	"realms/internal/upstream"
	"realms/internal/version"
	"realms/internal/web"
)

type AppOptions struct {
	Config  config.Config
	DB      *sql.DB
	Version version.BuildInfo
}

type App struct {
	cfg           config.Config
	db            *sql.DB
	store         *store.Store
	web           *web.Server
	admin         *admin.Server
	codexOAuth    *codexoauth.Flow
	codexClient   *codexoauth.Client
	openai        *openaiapi.Handler
	sched         *scheduler.Scheduler
	tokenLimits   *limits.TokenLimits
	version       version.BuildInfo
	ticketStorage *tickets.Storage
	mux           *http.ServeMux
}

func NewApp(opts AppOptions) (*App, error) {
	st := store.New(opts.DB)
	st.SetDialect(store.Dialect(opts.Config.DB.Driver))
	st.SetAppSettingsDefaults(opts.Config.AppSettingsDefaults)

	sched := scheduler.New(st)
	exec := upstream.NewExecutor(st, opts.Config)

	publicBaseURL := opts.Config.Server.PublicBaseURL
	if strings.TrimSpace(opts.Config.AppSettingsDefaults.SiteBaseURL) != "" {
		publicBaseURL = strings.TrimRight(strings.TrimSpace(opts.Config.AppSettingsDefaults.SiteBaseURL), "/")
	}

	var oauthFlow *codexoauth.Flow
	var codexClient *codexoauth.Client
	if opts.Config.CodexOAuth.Enable {
		codexClient = codexoauth.NewClient(opts.Config.CodexOAuth)
		oauthFlow = codexoauth.NewFlow(st, codexClient, web.SessionCookieName, localBaseURL(opts.Config))
	}

	ticketStorage := tickets.NewStorage(opts.Config.Tickets.AttachmentsDir)

	webServer, err := web.NewServer(
		st,
		sched,
		exec,
		opts.Config.SelfMode.Enable,
		opts.Config.Security.AllowOpenRegistration,
		opts.Config.Security.DisableSecureCookies,
		opts.Config.Billing,
		opts.Config.Payment,
		opts.Config.SMTP,
		opts.Config.EmailVerif.Enable,
		publicBaseURL,
		opts.Config.Security.TrustProxyHeaders,
		opts.Config.Security.TrustedProxyCIDRs,
		opts.Config.Tickets,
		ticketStorage,
	)
	if err != nil {
		return nil, err
	}
	adminServer, err := admin.NewServer(
		st,
		oauthFlow,
		exec,
		opts.Config.SelfMode.Enable,
		opts.Config.EmailVerif.Enable,
		opts.Config.SMTP,
		opts.Config.Billing,
		opts.Config.Payment,
		publicBaseURL,
		opts.Config.AppSettingsDefaults.AdminTimeZone,
		opts.Config.Security.TrustProxyHeaders,
		opts.Config.Security.TrustedProxyCIDRs,
		opts.Config.Tickets,
		ticketStorage,
		sched,
	)
	if err != nil {
		return nil, err
	}
	tokenLimits := limits.NewTokenLimits(opts.Config.Limits.MaxInflightPerToken, opts.Config.Limits.MaxSSEConnectionsPerToken)
	credLimits := limits.NewCredentialLimits(opts.Config.Limits.MaxInflightPerCredential)
	proxyLog := proxylog.New(proxylog.Config{
		Enable:   opts.Config.Env == "dev" && opts.Config.Debug.ProxyLog.Enable,
		Dir:      opts.Config.Debug.ProxyLog.Dir,
		MaxBytes: opts.Config.Debug.ProxyLog.MaxBytes,
		MaxFiles: opts.Config.Debug.ProxyLog.MaxFiles,
	})
	qp := quotaProviderForConfig(st, opts.Config)
	sseMaxEventBytes := int(opts.Config.Limits.SSEMaxEventBytes)
	if int64(sseMaxEventBytes) != opts.Config.Limits.SSEMaxEventBytes || sseMaxEventBytes < 0 {
		sseMaxEventBytes = 0
	}
	openaiHandler := openaiapi.NewHandler(st, st, sched, exec, tokenLimits, credLimits, proxyLog, st, opts.Config.SelfMode.Enable, qp, st, st, opts.Config.Limits.DefaultMaxOutputTokens, upstream.SSEPumpOptions{
		MaxLineBytes:     sseMaxEventBytes,
		InitialLineBytes: 64 << 10,
		PingInterval:     opts.Config.Limits.SSEPingInterval,
		IdleTimeout:      opts.Config.Limits.StreamIdleTimeout,
	})

	app := &App{
		cfg:           opts.Config,
		db:            opts.DB,
		store:         st,
		web:           webServer,
		admin:         adminServer,
		codexOAuth:    oauthFlow,
		codexClient:   codexClient,
		openai:        openaiHandler,
		sched:         sched,
		tokenLimits:   tokenLimits,
		version:       opts.Version,
		ticketStorage: ticketStorage,
		mux:           http.NewServeMux(),
	}
	if err := app.bootstrap(); err != nil {
		return nil, err
	}
	app.routes()
	return app, nil
}

func quotaProviderForConfig(st *store.Store, cfg config.Config) quota.Provider {
	reserveTTL := cfg.Limits.MaxRequestDuration + 30*time.Second
	normal := quota.NewHybridProvider(st, reserveTTL, cfg.Billing.EnablePayAsYouGo)
	free := quota.NewFreeProvider(st, reserveTTL)
	return quota.NewFeatureProvider(st, cfg.SelfMode.Enable, normal, free)
}

func (a *App) Handler() http.Handler {
	return a.mux
}

func (a *App) CodexOAuthCallbackHandler() http.Handler {
	if a.codexOAuth == nil {
		return nil
	}
	return a.codexOAuth.Handler()
}

func localBaseURL(cfg config.Config) string {
	if strings.TrimSpace(cfg.AppSettingsDefaults.SiteBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(cfg.AppSettingsDefaults.SiteBaseURL), "/")
	}
	if strings.TrimSpace(cfg.Server.PublicBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(cfg.Server.PublicBaseURL), "/")
	}
	scheme := "http"
	host := "localhost"
	port := ""
	if h, p, err := net.SplitHostPort(cfg.Server.Addr); err == nil {
		port = p
		if h != "" && h != "0.0.0.0" && h != "::" {
			host = h
		}
	} else {
		port = strings.TrimPrefix(cfg.Server.Addr, ":")
	}
	if port == "" {
		return scheme + "://" + host
	}
	return scheme + "://" + host + ":" + port
}

func (a *App) routes() {
	selfMode := a.cfg.SelfMode.Enable

	a.mux.HandleFunc("GET /healthz", a.handleHealthz)
	a.mux.HandleFunc("GET /api/version", a.handleVersion)
	a.mux.HandleFunc("GET /assets/realms_icon.svg", a.handleRealmsIconSVG)
	a.mux.HandleFunc("HEAD /assets/realms_icon.svg", a.handleRealmsIconSVG)
	a.mux.HandleFunc("GET /favicon.ico", a.handleFaviconICO)
	a.mux.HandleFunc("HEAD /favicon.ico", a.handleFaviconICO)

	a.mux.Handle("GET /{$}", http.HandlerFunc(a.web.Index))
	a.mux.Handle("GET /login", http.HandlerFunc(a.web.LoginPage))
	a.mux.Handle("POST /login", http.HandlerFunc(a.web.Login))
	a.mux.Handle("GET /register", http.HandlerFunc(a.web.RegisterPage))
	a.mux.Handle("POST /register", http.HandlerFunc(a.web.Register))

	publicChain := func(h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.RequestTimeout(a.cfg.Limits.MaxRequestDuration),
			middleware.BodyCache(a.cfg.Limits.MaxBodyBytes),
		)
	}
	a.mux.Handle("POST /oauth/token", publicChain(http.HandlerFunc(a.web.OAuthToken)))
	publicFeatureChain := func(featureKey string, h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.RequestTimeout(a.cfg.Limits.MaxRequestDuration),
			middleware.FeatureGateEffective(a.store, selfMode, featureKey),
			middleware.BodyCache(a.cfg.Limits.MaxBodyBytes),
		)
	}

	if a.codexOAuth != nil {
		a.mux.Handle("GET /auth/callback", publicChain(a.codexOAuth.Handler()))
	}

	a.mux.Handle("POST /api/email/verification/send", publicChain(http.HandlerFunc(a.web.APIEmailVerificationSend)))
	if !selfMode {
		a.mux.Handle("POST /api/webhooks/subscription-orders/{order_id}/paid", publicFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.handleSubscriptionOrderPaidWebhook)))
		a.mux.Handle("POST /api/pay/stripe/webhook", publicFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.handleStripeWebhook)))
		a.mux.Handle("POST /api/pay/stripe/webhook/{payment_channel_id}", publicFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.handleStripeWebhookByPaymentChannel)))
		a.mux.Handle("GET /api/pay/epay/notify", publicFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.handleEPayNotify)))
		a.mux.Handle("GET /api/pay/epay/notify/{payment_channel_id}", publicFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.handleEPayNotifyByPaymentChannel)))
	}

	apiChain := func(h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.TokenAuth(a.store),
			middleware.TokenInflightLimiter(a.tokenLimits),
			middleware.BodyCache(a.cfg.Limits.MaxBodyBytes),
			middleware.StreamAwareRequestTimeout(a.cfg.Limits.MaxRequestDuration, a.cfg.Limits.MaxStreamDuration),
		)
	}
	apiFeatureChain := func(featureKey string, h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(a.store, selfMode, featureKey),
			middleware.TokenAuth(a.store),
			middleware.TokenInflightLimiter(a.tokenLimits),
			middleware.BodyCache(a.cfg.Limits.MaxBodyBytes),
			middleware.StreamAwareRequestTimeout(a.cfg.Limits.MaxRequestDuration, a.cfg.Limits.MaxStreamDuration),
		)
	}
	a.mux.Handle("POST /v1/responses", apiChain(http.HandlerFunc(a.openai.Responses)))
	a.mux.Handle("POST /v1/messages", apiChain(http.HandlerFunc(a.openai.Messages)))
	a.mux.Handle("GET /v1/models", apiFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(a.openai.Models)))
	a.mux.Handle("GET /api/usage/windows", apiFeatureChain(store.SettingFeatureDisableWebUsage, http.HandlerFunc(a.web.APIUsageWindows)))
	a.mux.Handle("GET /api/usage/events", apiFeatureChain(store.SettingFeatureDisableWebUsage, http.HandlerFunc(a.web.APIUsageEvents)))

	webChain := func(h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.SessionAuth(a.store, web.SessionCookieName),
		)
	}
	webFeatureChain := func(featureKey string, h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(a.store, selfMode, featureKey),
			middleware.SessionAuth(a.store, web.SessionCookieName),
		)
	}
	webCSRFChain := func(h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.SessionAuth(a.store, web.SessionCookieName),
			middleware.CSRF(),
		)
	}
	webCSRFFeatureChain := func(featureKey string, h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(a.store, selfMode, featureKey),
			middleware.SessionAuth(a.store, web.SessionCookieName),
			middleware.CSRF(),
		)
	}
	ticketUploadLimit := a.cfg.Tickets.MaxUploadBytes + (4 << 20) // 预留少量 multipart 开销
	webUploadFeatureChain := func(featureKey string, h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(a.store, selfMode, featureKey),
			middleware.SessionAuth(a.store, web.SessionCookieName),
			middleware.MaxBytes(ticketUploadLimit),
			middleware.CSRF(),
		)
	}
	adminChain := func(h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.SessionAuth(a.store, web.SessionCookieName),
			middleware.RequireRoles(store.UserRoleRoot),
			middleware.CSRF(),
		)
	}
	adminFeatureChain := func(featureKey string, h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(a.store, selfMode, featureKey),
			middleware.SessionAuth(a.store, web.SessionCookieName),
			middleware.RequireRoles(store.UserRoleRoot),
			middleware.CSRF(),
		)
	}
	adminFeatureChain2 := func(featureKey1, featureKey2 string, h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(a.store, selfMode, featureKey1),
			middleware.FeatureGateEffective(a.store, selfMode, featureKey2),
			middleware.SessionAuth(a.store, web.SessionCookieName),
			middleware.RequireRoles(store.UserRoleRoot),
			middleware.CSRF(),
		)
	}
	adminUploadFeatureChain := func(featureKey string, h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(a.store, selfMode, featureKey),
			middleware.SessionAuth(a.store, web.SessionCookieName),
			middleware.RequireRoles(store.UserRoleRoot),
			middleware.MaxBytes(ticketUploadLimit),
			middleware.CSRF(),
		)
	}

	a.mux.Handle("GET /admin", adminChain(http.HandlerFunc(a.admin.Home)))
	a.mux.Handle("GET /admin/channels", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.Channels)))
	a.mux.Handle("POST /admin/channels", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.CreateChannel)))
	a.mux.Handle("POST /admin/channels/{channel_id}", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.UpdateChannel)))
	a.mux.Handle("POST /admin/channels/{channel_id}/limits", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.UpdateChannelLimits)))
	a.mux.Handle("GET /admin/channel-groups", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(a.admin.ChannelGroups)))
	a.mux.Handle("POST /admin/channel-groups", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(a.admin.CreateChannelGroup)))
	a.mux.Handle("POST /admin/channel-groups/{group_id}", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(a.admin.UpdateChannelGroup)))
	a.mux.Handle("GET /admin/channel-groups/{group_id}", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(a.admin.ChannelGroupDetail)))
	a.mux.Handle("POST /admin/channel-groups/{group_id}/children/groups", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(a.admin.CreateChildChannelGroup)))
	a.mux.Handle("POST /admin/channel-groups/{group_id}/children/channels", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(a.admin.AddChannelGroupChannelMember)))
	a.mux.Handle("POST /admin/channel-groups/{group_id}/children/groups/{child_group_id}/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(a.admin.DeleteChannelGroupGroupMember)))
	a.mux.Handle("POST /admin/channel-groups/{group_id}/children/channels/{channel_id}/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(a.admin.DeleteChannelGroupChannelMember)))
	a.mux.Handle("POST /admin/channel-groups/{group_id}/children/reorder", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(a.admin.ReorderChannelGroupMembers)))
	a.mux.Handle("POST /admin/channel-groups/{group_id}/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(a.admin.DeleteChannelGroup)))
	a.mux.Handle("POST /admin/channels/reorder", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.ReorderChannels)))
	// 兼容：部分前端/代理可能无法正确透传 path 参数，允许从表单 channel_id 读取。
	a.mux.Handle("POST /admin/channels/test", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.TestChannel)))
	a.mux.Handle("POST /admin/channels/{channel_id}/test", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.TestChannel)))
	a.mux.Handle("POST /admin/channels/{channel_id}/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.DeleteChannel)))
	a.mux.Handle("GET /admin/channels/{channel_id}/models", adminFeatureChain2(store.SettingFeatureDisableAdminChannels, store.SettingFeatureDisableModels, http.HandlerFunc(a.admin.ChannelModels)))
	a.mux.Handle("POST /admin/channels/{channel_id}/models", adminFeatureChain2(store.SettingFeatureDisableAdminChannels, store.SettingFeatureDisableModels, http.HandlerFunc(a.admin.CreateChannelModel)))
	a.mux.Handle("GET /admin/channels/{channel_id}/models/{binding_id}", adminFeatureChain2(store.SettingFeatureDisableAdminChannels, store.SettingFeatureDisableModels, http.HandlerFunc(a.admin.ChannelModel)))
	a.mux.Handle("POST /admin/channels/{channel_id}/models/{binding_id}", adminFeatureChain2(store.SettingFeatureDisableAdminChannels, store.SettingFeatureDisableModels, http.HandlerFunc(a.admin.UpdateChannelModel)))
	a.mux.Handle("POST /admin/channels/{channel_id}/models/{binding_id}/delete", adminFeatureChain2(store.SettingFeatureDisableAdminChannels, store.SettingFeatureDisableModels, http.HandlerFunc(a.admin.DeleteChannelModel)))
	a.mux.Handle("GET /admin/channels/{channel_id}/endpoints", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.Endpoints)))
	a.mux.Handle("POST /admin/channels/{channel_id}/endpoints", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.CreateEndpoint)))
	a.mux.Handle("POST /admin/endpoints/{endpoint_id}/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.DeleteEndpoint)))
	a.mux.Handle("POST /admin/endpoints/{endpoint_id}/openai-credentials", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.CreateOpenAICredential)))
	a.mux.Handle("POST /admin/endpoints/{endpoint_id}/anthropic-credentials", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.CreateAnthropicCredential)))
	a.mux.Handle("POST /admin/openai-credentials/{credential_id}/limits", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.UpdateOpenAICredentialLimits)))
	a.mux.Handle("POST /admin/openai-credentials/{credential_id}/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.DeleteOpenAICredential)))
	a.mux.Handle("POST /admin/anthropic-credentials/{credential_id}/limits", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.UpdateAnthropicCredentialLimits)))
	a.mux.Handle("POST /admin/anthropic-credentials/{credential_id}/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.DeleteAnthropicCredential)))
	a.mux.Handle("GET /admin/endpoints/{endpoint_id}/codex-accounts", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.CodexAccounts)))
	a.mux.Handle("POST /admin/endpoints/{endpoint_id}/codex-accounts", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.CreateCodexAccount)))
	a.mux.Handle("POST /admin/endpoints/{endpoint_id}/codex-accounts/refresh", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.RefreshCodexQuotasByEndpoint)))
	a.mux.Handle("POST /admin/endpoints/{endpoint_id}/codex-oauth/start", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.StartCodexOAuth)))
	a.mux.Handle("POST /admin/endpoints/{endpoint_id}/codex-oauth/complete", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.CompleteCodexOAuth)))
	a.mux.Handle("POST /admin/codex-accounts/{account_id}/limits", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.UpdateCodexAccountLimits)))
	a.mux.Handle("POST /admin/codex-accounts/{account_id}/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.DeleteCodexAccount)))
	a.mux.Handle("POST /admin/codex-accounts/{account_id}/refresh", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.RefreshCodexQuota)))

	// 批量注册API
	a.mux.Handle("POST /admin/batch-register", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.StartBatchRegister)))
	a.mux.Handle("GET /admin/batch-register-progress/{task_id}", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.StreamBatchRegisterProgress)))
	a.mux.Handle("POST /admin/batch-register/{task_id}/cancel", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(a.admin.CancelBatchRegister)))

	a.mux.Handle("GET /admin/users", adminFeatureChain(store.SettingFeatureDisableAdminUsers, http.HandlerFunc(a.admin.Users)))
	a.mux.Handle("POST /admin/users", adminFeatureChain(store.SettingFeatureDisableAdminUsers, http.HandlerFunc(a.admin.CreateUser)))
	a.mux.Handle("POST /admin/users/{user_id}", adminFeatureChain(store.SettingFeatureDisableAdminUsers, http.HandlerFunc(a.admin.UpdateUser)))
	a.mux.Handle("POST /admin/users/{user_id}/profile", adminFeatureChain(store.SettingFeatureDisableAdminUsers, http.HandlerFunc(a.admin.UpdateUserProfile)))
	a.mux.Handle("POST /admin/users/{user_id}/password", adminFeatureChain(store.SettingFeatureDisableAdminUsers, http.HandlerFunc(a.admin.UpdateUserPassword)))
	a.mux.Handle("POST /admin/users/{user_id}/delete", adminFeatureChain(store.SettingFeatureDisableAdminUsers, http.HandlerFunc(a.admin.DeleteUser)))
	if !selfMode {
		a.mux.Handle("GET /admin/subscriptions", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.Subscriptions)))
		a.mux.Handle("POST /admin/subscriptions", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.CreateSubscriptionPlan)))
		a.mux.Handle("GET /admin/subscriptions/{plan_id}", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.SubscriptionPlan)))
		a.mux.Handle("POST /admin/subscriptions/{plan_id}", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.UpdateSubscriptionPlan)))
		a.mux.Handle("POST /admin/subscriptions/{plan_id}/delete", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.DeleteSubscriptionPlan)))
		a.mux.Handle("GET /admin/orders", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.Orders)))
		a.mux.Handle("POST /admin/orders/{order_id}/approve", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.ApproveSubscriptionOrder)))
		a.mux.Handle("POST /admin/orders/{order_id}/reject", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.RejectSubscriptionOrder)))
		a.mux.Handle("GET /admin/settings/payment-channels", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.PaymentChannels)))
		a.mux.Handle("POST /admin/settings/payment-channels", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.CreatePaymentChannel)))
		a.mux.Handle("GET /admin/settings/payment-channels/{payment_channel_id}", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.PaymentChannel)))
		a.mux.Handle("POST /admin/settings/payment-channels/{payment_channel_id}", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.UpdatePaymentChannel)))
		a.mux.Handle("POST /admin/settings/payment-channels/{payment_channel_id}/delete", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.DeletePaymentChannel)))

		a.mux.Handle("GET /admin/payment-channels", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := "/admin/settings/payment-channels"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusFound)
		})))
		a.mux.Handle("POST /admin/payment-channels", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.CreatePaymentChannel)))
		a.mux.Handle("GET /admin/payment-channels/{payment_channel_id}", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := "/admin/settings/payment-channels?edit=" + strings.TrimSpace(r.PathValue("payment_channel_id"))
			if r.URL.RawQuery != "" {
				target += "&" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusFound)
		})))
		a.mux.Handle("POST /admin/payment-channels/{payment_channel_id}", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.UpdatePaymentChannel)))
		a.mux.Handle("POST /admin/payment-channels/{payment_channel_id}/delete", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.admin.DeletePaymentChannel)))
	}
	a.mux.Handle("GET /admin/usage", adminFeatureChain(store.SettingFeatureDisableAdminUsage, http.HandlerFunc(a.admin.Usage)))
	a.mux.Handle("GET /admin/settings", adminChain(http.HandlerFunc(a.admin.Settings)))
	a.mux.Handle("POST /admin/settings", adminChain(http.HandlerFunc(a.admin.UpdateSettings)))
	a.mux.Handle("GET /admin/backup", adminChain(http.HandlerFunc(a.admin.Backup)))
	a.mux.Handle("GET /admin/export", adminChain(http.HandlerFunc(a.admin.Export)))
	a.mux.Handle("POST /admin/import", adminChain(http.HandlerFunc(a.admin.Import)))
	a.mux.Handle("GET /admin/oauth-apps", adminChain(http.HandlerFunc(a.admin.OAuthApps)))
	a.mux.Handle("POST /admin/oauth-apps", adminChain(http.HandlerFunc(a.admin.CreateOAuthApp)))
	a.mux.Handle("GET /admin/oauth-apps/{app_id}", adminChain(http.HandlerFunc(a.admin.OAuthApp)))
	a.mux.Handle("POST /admin/oauth-apps/{app_id}", adminChain(http.HandlerFunc(a.admin.UpdateOAuthApp)))
	a.mux.Handle("POST /admin/oauth-apps/{app_id}/rotate-secret", adminChain(http.HandlerFunc(a.admin.RotateOAuthAppSecret)))
	a.mux.Handle("GET /admin/models", adminFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(a.admin.Models)))
	a.mux.Handle("POST /admin/models", adminFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(a.admin.CreateModel)))
	a.mux.Handle("POST /admin/models/library-lookup", adminFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(a.admin.ModelLibraryLookup)))
	a.mux.Handle("POST /admin/models/import-pricing", adminUploadFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(a.admin.ImportModelPricing)))
	a.mux.Handle("GET /admin/models/{model_id}", adminFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(a.admin.Model)))
	a.mux.Handle("POST /admin/models/{model_id}", adminFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(a.admin.UpdateModel)))
	a.mux.Handle("POST /admin/models/{model_id}/delete", adminFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(a.admin.DeleteModel)))
	if !selfMode {
		a.mux.Handle("GET /admin/tickets", adminFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(a.admin.Tickets)))
		a.mux.Handle("GET /admin/tickets/{ticket_id}", adminFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(a.admin.Ticket)))
		a.mux.Handle("POST /admin/tickets/{ticket_id}/reply", adminUploadFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(a.admin.ReplyTicket)))
		a.mux.Handle("POST /admin/tickets/{ticket_id}/close", adminFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(a.admin.CloseTicket)))
		a.mux.Handle("POST /admin/tickets/{ticket_id}/reopen", adminFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(a.admin.ReopenTicket)))
		a.mux.Handle("GET /admin/tickets/{ticket_id}/attachments/{attachment_id}", adminFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(a.admin.TicketAttachmentDownload)))
	}
	a.mux.Handle("GET /admin/announcements", adminFeatureChain(store.SettingFeatureDisableAdminAnnouncements, http.HandlerFunc(a.admin.Announcements)))
	a.mux.Handle("POST /admin/announcements", adminFeatureChain(store.SettingFeatureDisableAdminAnnouncements, http.HandlerFunc(a.admin.CreateAnnouncement)))
	a.mux.Handle("POST /admin/announcements/{announcement_id}", adminFeatureChain(store.SettingFeatureDisableAdminAnnouncements, http.HandlerFunc(a.admin.UpdateAnnouncementStatus)))
	a.mux.Handle("POST /admin/announcements/{announcement_id}/delete", adminFeatureChain(store.SettingFeatureDisableAdminAnnouncements, http.HandlerFunc(a.admin.DeleteAnnouncement)))

	a.mux.Handle("GET /oauth/authorize", webChain(http.HandlerFunc(a.web.OAuthAuthorizePage)))
	a.mux.Handle("POST /oauth/authorize", webCSRFChain(http.HandlerFunc(a.web.OAuthAuthorize)))

	a.mux.Handle("GET /dashboard", webChain(http.HandlerFunc(a.web.Dashboard)))
	a.mux.Handle("GET /announcements", webFeatureChain(store.SettingFeatureDisableWebAnnouncements, http.HandlerFunc(a.web.AnnouncementsPage)))
	a.mux.Handle("GET /announcements/{announcement_id}", webFeatureChain(store.SettingFeatureDisableWebAnnouncements, http.HandlerFunc(a.web.AnnouncementDetailPage)))
	a.mux.Handle("GET /account", webChain(http.HandlerFunc(a.web.AccountPage)))
	a.mux.Handle("GET /tokens", webFeatureChain(store.SettingFeatureDisableWebTokens, http.HandlerFunc(a.web.TokensPage)))
	a.mux.Handle("GET /models", webFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(a.web.ModelsPage)))
	if !selfMode {
		a.mux.Handle("GET /subscription", webFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.web.SubscriptionPage)))
		a.mux.Handle("GET /topup", webFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.web.TopupPage)))
	}
	a.mux.Handle("GET /usage", webFeatureChain(store.SettingFeatureDisableWebUsage, http.HandlerFunc(a.web.UsagePage)))
	if !selfMode {
		a.mux.Handle("GET /tickets", webFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(a.web.TicketsPage)))
		a.mux.Handle("GET /tickets/new", webFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(a.web.TicketNewPage)))
		a.mux.Handle("POST /tickets/new", webUploadFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(a.web.CreateTicket)))
		a.mux.Handle("GET /tickets/{ticket_id}", webFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(a.web.TicketDetailPage)))
		a.mux.Handle("POST /tickets/{ticket_id}/reply", webUploadFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(a.web.ReplyTicket)))
		a.mux.Handle("GET /tickets/{ticket_id}/attachments/{attachment_id}", webFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(a.web.TicketAttachmentDownload)))
	}
	a.mux.Handle("POST /account/username", webCSRFChain(http.HandlerFunc(a.web.AccountUpdateUsername)))
	a.mux.Handle("POST /account/email", webCSRFChain(http.HandlerFunc(a.web.AccountUpdateEmail)))
	a.mux.Handle("POST /account/password", webCSRFChain(http.HandlerFunc(a.web.AccountUpdatePassword)))
	if !selfMode {
		a.mux.Handle("POST /subscription/purchase", webCSRFFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.web.PurchaseSubscription)))
		a.mux.Handle("POST /topup/create", webCSRFFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.web.CreateTopupOrder)))
		a.mux.Handle("GET /pay/{kind}/{order_id}", webFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.web.PayPage)))
		a.mux.Handle("POST /pay/{kind}/{order_id}/start", webCSRFFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.web.StartPayment)))
		a.mux.Handle("POST /pay/{kind}/{order_id}/cancel", webCSRFFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(a.web.CancelPayOrder)))
	}
	a.mux.Handle("POST /tokens/new", webCSRFFeatureChain(store.SettingFeatureDisableWebTokens, http.HandlerFunc(a.web.CreateToken)))
	a.mux.Handle("POST /tokens/rotate", webCSRFFeatureChain(store.SettingFeatureDisableWebTokens, http.HandlerFunc(a.web.RotateToken)))
	a.mux.Handle("POST /tokens/revoke", webCSRFFeatureChain(store.SettingFeatureDisableWebTokens, http.HandlerFunc(a.web.RevokeToken)))
	a.mux.Handle("POST /tokens/delete", webCSRFFeatureChain(store.SettingFeatureDisableWebTokens, http.HandlerFunc(a.web.DeleteToken)))
	a.mux.Handle("POST /announcements/{announcement_id}/read", webCSRFFeatureChain(store.SettingFeatureDisableWebAnnouncements, http.HandlerFunc(a.web.AnnouncementMarkRead)))
	a.mux.Handle("POST /logout", webCSRFChain(http.HandlerFunc(a.web.Logout)))
}

func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	type resp struct {
		OK      bool   `json:"ok"`
		Env     string `json:"env"`
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Date    string `json:"date"`

		DBOK bool `json:"db_ok"`

		AllowOpenRegistration bool `json:"allow_open_registration"`

		Limits struct {
			MaxBodyBytes              int64  `json:"max_body_bytes"`
			MaxRequestDuration        string `json:"max_request_duration"`
			DefaultMaxOutputTokens    int    `json:"default_max_output_tokens"`
			MaxInflightPerToken       int    `json:"max_inflight_per_token"`
			MaxSSEConnectionsPerToken int    `json:"max_sse_connections_per_token"`
			MaxInflightPerCredential  int    `json:"max_inflight_per_credential"`
		} `json:"limits"`
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	dbOK := a.db.PingContext(ctx) == nil

	out := resp{
		OK:                    true,
		Env:                   a.cfg.Env,
		Version:               a.version.Version,
		Commit:                a.version.Commit,
		Date:                  a.version.Date,
		DBOK:                  dbOK,
		AllowOpenRegistration: a.cfg.Security.AllowOpenRegistration,
	}
	out.Limits.MaxBodyBytes = a.cfg.Limits.MaxBodyBytes
	out.Limits.MaxRequestDuration = a.cfg.Limits.MaxRequestDuration.String()
	out.Limits.DefaultMaxOutputTokens = a.cfg.Limits.DefaultMaxOutputTokens
	out.Limits.MaxInflightPerToken = a.cfg.Limits.MaxInflightPerToken
	out.Limits.MaxSSEConnectionsPerToken = a.cfg.Limits.MaxSSEConnectionsPerToken
	out.Limits.MaxInflightPerCredential = a.cfg.Limits.MaxInflightPerCredential

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}

func (a *App) handleRealmsIconSVG(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(assets.RealmsIconSVG())
}

func (a *App) handleFaviconICO(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/assets/realms_icon.svg", http.StatusPermanentRedirect)
}

func (a *App) handleVersion(w http.ResponseWriter, r *http.Request) {
	type resp struct {
		OK      bool   `json:"ok"`
		Env     string `json:"env"`
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Date    string `json:"date"`
	}
	out := resp{
		OK:      true,
		Env:     a.cfg.Env,
		Version: a.version.Version,
		Commit:  a.version.Commit,
		Date:    a.version.Date,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}

func (a *App) bootstrap() error {
	go a.usageCleanupLoop()
	go a.codexBalanceRefreshLoop()
	if !a.cfg.SelfMode.Enable {
		go a.ticketAttachmentsCleanupLoop()
	}
	return nil
}

func (a *App) usageCleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, _ = a.store.ExpireReservedUsage(ctx, time.Now())
		cancel()
	}
}

func (a *App) codexBalanceRefreshLoop() {
	if a.codexClient == nil {
		return
	}

	refreshOnce := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		a.refreshAllCodexBalances(ctx)
	}

	// 启动后先跑一轮，尽快让后台可见到余额信息。
	refreshOnce()

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		refreshOnce()
	}
}

func (a *App) ticketAttachmentsCleanupLoop() {
	if a.ticketStorage == nil {
		return
	}

	cleanupOnce := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		a.cleanupExpiredTicketAttachments(ctx)
	}

	// 启动后先跑一轮，尽快释放磁盘与记录。
	cleanupOnce()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		cleanupOnce()
	}
}

func (a *App) cleanupExpiredTicketAttachments(ctx context.Context) {
	const batchSize = 200
	for {
		atts, err := a.store.ListExpiredTicketAttachments(ctx, batchSize)
		if err != nil || len(atts) == 0 {
			return
		}

		ids := make([]int64, 0, len(atts))
		for _, att := range atts {
			ids = append(ids, att.ID)
			full, err := a.ticketStorage.Resolve(att.StorageRelPath)
			if err != nil {
				continue
			}
			_ = os.Remove(full)
		}
		_, _ = a.store.DeleteTicketAttachmentsByIDs(ctx, ids)

		if len(atts) < batchSize {
			return
		}
	}
}

func (a *App) refreshAllCodexBalances(ctx context.Context) {
	accs, err := a.store.ListCodexOAuthAccountRefs(ctx)
	if err != nil {
		return
	}

	for _, acc := range accs {
		perCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		a.refreshCodexBalance(perCtx, acc.ID)
		cancel()
	}
}

func (a *App) refreshCodexBalance(ctx context.Context, accountID int64) {
	now := time.Now()
	sec, err := a.store.GetCodexOAuthSecret(ctx, accountID)
	if err != nil {
		msg := err.Error()
		_ = a.store.UpdateCodexOAuthAccountQuota(ctx, accountID, store.CodexOAuthQuota{}, now, &msg)
		return
	}

	ep, err := a.store.GetUpstreamEndpointByID(ctx, sec.EndpointID)
	if err != nil {
		msg := err.Error()
		_ = a.store.UpdateCodexOAuthAccountQuota(ctx, accountID, store.CodexOAuthQuota{}, now, &msg)
		return
	}

	accessToken := sec.AccessToken
	refreshToken := sec.RefreshToken
	idTokenPtr := sec.IDToken
	expiresAt := sec.ExpiresAt

	maybeRefresh := func() bool {
		if a.codexClient == nil {
			return false
		}
		refreshed, err := a.codexClient.Refresh(ctx, refreshToken)
		if err != nil {
			return false
		}
		rt := refreshed.RefreshToken
		if strings.TrimSpace(rt) == "" {
			rt = refreshToken
		}
		if strings.TrimSpace(refreshed.IDToken) != "" {
			t := refreshed.IDToken
			idTokenPtr = &t
		}
		if refreshed.ExpiresAt != nil {
			expiresAt = refreshed.ExpiresAt
		}
		_ = a.store.UpdateCodexOAuthAccountTokens(ctx, accountID, refreshed.AccessToken, rt, idTokenPtr, expiresAt)
		accessToken = refreshed.AccessToken
		refreshToken = rt
		return true
	}

	if expiresAt != nil && time.Until(*expiresAt) < 5*time.Minute {
		_ = maybeRefresh()
	}

	quota, err := a.codexClient.FetchQuota(ctx, ep.BaseURL, accessToken, sec.AccountID)
	if err != nil {
		var se *codexoauth.HTTPStatusError
		if errors.As(err, &se) && (se.StatusCode == http.StatusUnauthorized || se.StatusCode == http.StatusForbidden) {
			if maybeRefresh() {
				quota, err = a.codexClient.FetchQuota(ctx, ep.BaseURL, accessToken, sec.AccountID)
			}
		}
	}
	if err != nil {
		msg := err.Error()
		_ = a.store.UpdateCodexOAuthAccountQuota(ctx, accountID, store.CodexOAuthQuota{}, now, &msg)
		return
	}

	_ = a.store.UpdateCodexOAuthAccountQuota(ctx, accountID, store.CodexOAuthQuota{
		CreditsHasCredits:    quota.CreditsHasCredits,
		CreditsUnlimited:     quota.CreditsUnlimited,
		CreditsBalance:       quota.CreditsBalance,
		PrimaryUsedPercent:   quota.PrimaryUsedPercent,
		PrimaryResetAt:       quota.PrimaryResetAt,
		SecondaryUsedPercent: quota.SecondaryUsedPercent,
		SecondaryResetAt:     quota.SecondaryResetAt,
	}, now, nil)
}

func (a *App) RefreshCodexQuotasByEndpoint(w http.ResponseWriter, r *http.Request) {
	if a.codexClient == nil {
		http.Error(w, "Codex OAuth 未启用", http.StatusBadRequest)
		return
	}

	endpointID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("endpoint_id")), 10, 64)
	if err != nil || endpointID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	ep, err := a.store.GetUpstreamEndpointByID(ctx, endpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}
	ch, err := a.store.GetUpstreamChannelByID(ctx, ep.ChannelID)
	if err != nil {
		http.Error(w, "channel 不存在", http.StatusNotFound)
		return
	}

	accs, err := a.store.ListCodexOAuthAccountsByEndpoint(ctx, endpointID)
	if err != nil {
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}
	for _, acc := range accs {
		perCtx, perCancel := context.WithTimeout(ctx, 15*time.Second)
		a.refreshCodexBalance(perCtx, acc.ID)
		perCancel()
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/channels/%d/endpoints#accounts", ch.ID), http.StatusFound)
}

func (a *App) RefreshCodexQuota(w http.ResponseWriter, r *http.Request) {
	if a.codexClient == nil {
		http.Error(w, "Codex OAuth 未启用", http.StatusBadRequest)
		return
	}

	accountID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("account_id")), 10, 64)
	if err != nil || accountID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	acc, err := a.store.GetCodexOAuthAccountByID(ctx, accountID)
	if err != nil {
		http.Error(w, "account 不存在", http.StatusNotFound)
		return
	}
	ep, err := a.store.GetUpstreamEndpointByID(ctx, acc.EndpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}
	ch, err := a.store.GetUpstreamChannelByID(ctx, ep.ChannelID)
	if err != nil {
		http.Error(w, "channel 不存在", http.StatusNotFound)
		return
	}

	a.refreshCodexBalance(ctx, acc.ID)
	http.Redirect(w, r, fmt.Sprintf("/admin/channels/%d/endpoints#accounts", ch.ID), http.StatusFound)
}
