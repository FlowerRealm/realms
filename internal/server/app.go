// Package server 组装 HTTP 路由、依赖与中间件，使 main 保持简单可读。
package server

import (
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	root "realms"
	openaiapi "realms/internal/api/openai"
	"realms/internal/assets"
	"realms/internal/codexoauth"
	"realms/internal/config"
	"realms/internal/obs"
	"realms/internal/proxylog"
	"realms/internal/quota"
	"realms/internal/scheduler"
	"realms/internal/store"
	"realms/internal/tickets"
	"realms/internal/upstream"
	"realms/internal/version"
	"realms/router"
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
	codexOAuth    *codexoauth.Flow
	exec          *upstream.Executor
	openai        *openaiapi.Handler
	sched         *scheduler.Scheduler
	version       version.BuildInfo
	ticketStorage *tickets.Storage
	engine        *gin.Engine
}

func NewApp(opts AppOptions) (*App, error) {
	st := store.New(opts.DB)
	st.SetDialect(store.Dialect(opts.Config.DB.Driver))
	st.SetAppSettingsDefaults(opts.Config.AppSettingsDefaults)

	// Upstream 配置快照缓存：减少数据面每请求读取 channels/endpoints/credentials 的 DB QPS。
	// - 0/未设置：禁用（保持旧行为）
	// - 建议：1000–5000ms（运维侧可按“配置变更频率 vs DB 压力”调整）
	upstreamSnapshotTTL := 0 * time.Millisecond
	if raw := strings.TrimSpace(os.Getenv("REALMS_UPSTREAM_SNAPSHOT_TTL_MILLIS")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			upstreamSnapshotTTL = time.Duration(n) * time.Millisecond
		}
	}
	upstreamStore := scheduler.UpstreamStore(st)
	if upstreamSnapshotTTL > 0 {
		upstreamStore = scheduler.NewCachedUpstreamStore(st, upstreamSnapshotTTL)
	}

	sched := scheduler.New(upstreamStore)
	sched.SetBindingStore(st)
	sched.SetGroupPointerStore(st)
	exec := upstream.NewExecutor(st, opts.Config)

	publicBaseURL := opts.Config.Server.PublicBaseURL
	if strings.TrimSpace(opts.Config.AppSettingsDefaults.SiteBaseURL) != "" {
		publicBaseURL = strings.TrimRight(strings.TrimSpace(opts.Config.AppSettingsDefaults.SiteBaseURL), "/")
	}
	sessionCookieName := SessionCookieNameForSelfMode(opts.Config.SelfMode.Enable)
	sessionSecret := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	if sessionSecret == "" {
		sessionSecret = randomSecret(32)
	}

	oauthFlow := codexoauth.NewFlow(st, sessionCookieName, sessionSecret, localBaseURL(opts.Config), codexOAuthRedirectURI(opts.Config.Server.Addr))

	ticketStorage := tickets.NewStorage(opts.Config.Tickets.AttachmentsDir)
	proxyLog := proxylog.New(proxylog.Config{
		Enable: opts.Config.Env == "dev" && opts.Config.Debug.ProxyLog.Enable,
		Dir:    opts.Config.Debug.ProxyLog.Dir,
	})
	qp := quotaProvider(st, opts.Config)
	maxSSELineBytes := 1 << 20 // 1MiB: bound per-connection memory on malformed upstream SSE
	if raw := strings.TrimSpace(os.Getenv("REALMS_SSE_MAX_LINE_BYTES")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			if n <= 0 {
				maxSSELineBytes = 0 // allow disabling the limit explicitly
			} else {
				maxSSELineBytes = n
			}
		}
	}
	openaiHandler := openaiapi.NewHandler(st, st, sched, exec, proxyLog, st, opts.Config.SelfMode.Enable, qp, st, st, st, upstream.SSEPumpOptions{
		InitialLineBytes: 64 << 10,
		MaxLineBytes:     maxSSELineBytes,
	})

	app := &App{
		cfg:           opts.Config,
		db:            opts.DB,
		store:         st,
		codexOAuth:    oauthFlow,
		exec:          exec,
		openai:        openaiHandler,
		sched:         sched,
		version:       opts.Version,
		ticketStorage: ticketStorage,
	}
	if err := app.bootstrap(); err != nil {
		return nil, err
	}

	if opts.Config.Env != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	engine.Use(gin.Recovery())
	sessionStore := cookie.NewStore([]byte(sessionSecret))
	sessionStore.Options(sessions.Options{
		Path:     "/",
		MaxAge:   2592000, // 30 days
		HttpOnly: true,
		Secure:   opts.Config.Env != "dev" && !opts.Config.Security.DisableSecureCookies,
		SameSite: http.SameSiteStrictMode,
	})
	engine.Use(sessions.Sessions(sessionCookieName, sessionStore))

	frontendBaseURL := strings.TrimSpace(os.Getenv("FRONTEND_BASE_URL"))
	frontendDistDir := strings.TrimSpace(os.Getenv("FRONTEND_DIST_DIR"))
	if frontendDistDir == "" {
		frontendDistDir = "./web/dist"
	}
	var frontendFS fs.FS
	frontendIndexPage := loadEmbeddedIndexHTML()
	if len(frontendIndexPage) > 0 {
		frontendFS = root.WebDistFS
	}

	router.SetRouter(engine, router.Options{
		Store:                           st,
		SelfMode:                        opts.Config.SelfMode.Enable,
		AllowOpenRegistration:           opts.Config.Security.AllowOpenRegistration,
		EmailVerificationEnabledDefault: opts.Config.EmailVerif.Enable,
		PublicBaseURLDefault:            publicBaseURL,
		AdminTimeZoneDefault:            opts.Config.AppSettingsDefaults.AdminTimeZone,
		BillingDefault:                  opts.Config.Billing,
		SMTPDefault:                     opts.Config.SMTP,
		TicketStorage:                   ticketStorage,
		PublicMaxBodyBytes:              opts.Config.Server.PublicMaxBodyBytes,
		OpenAIMaxBodyBytes:              opts.Config.Server.OpenAIMaxBodyBytes,
		FrontendBaseURL:                 frontendBaseURL,
		FrontendDistDir:                 frontendDistDir,
		FrontendIndexPage:               frontendIndexPage,
		FrontendFS:                      frontendFS,
		OpenAI:                          openaiHandler,
		Sched:                           sched,

		CodexOAuthHandler: func() http.Handler {
			return oauthFlow.Handler()
		}(),

		Healthz:       app.handleHealthz,
		RealmsIconSVG: app.handleRealmsIconSVG,
		FaviconICO:    app.handleFaviconICO,

		SubscriptionOrderPaidWebhook:  app.handleSubscriptionOrderPaidWebhook,
		StripeWebhookByPaymentChannel: app.handleStripeWebhookByPaymentChannel,
		EPayNotifyByPaymentChannel:    app.handleEPayNotifyByPaymentChannel,

		RefreshCodexQuotasByEndpoint: app.RefreshCodexQuotasByEndpoint,
		RefreshCodexQuota:            app.RefreshCodexQuota,
		StartCodexOAuth: func(ctx context.Context, endpointID int64, actorUserID int64) (string, error) {
			if app.codexOAuth == nil {
				return "", errors.New("Codex OAuth 未启用")
			}
			return app.codexOAuth.Start(ctx, endpointID, actorUserID)
		},
		CompleteCodexOAuth: func(ctx context.Context, endpointID int64, actorUserID int64, state string, code string) error {
			if app.codexOAuth == nil {
				return errors.New("Codex OAuth 未启用")
			}
			return app.codexOAuth.Complete(ctx, endpointID, actorUserID, state, code)
		},
		RefreshCodexQuotasByEndpointID: func(ctx context.Context, endpointID int64) error {
			accs, err := app.store.ListCodexOAuthAccountsByEndpoint(ctx, endpointID)
			if err != nil {
				return err
			}
			for _, acc := range accs {
				perCtx, perCancel := context.WithTimeout(ctx, 15*time.Second)
				app.refreshCodexBalance(perCtx, acc.ID)
				perCancel()
			}
			return nil
		},
		RefreshCodexQuotaByAccountID: func(ctx context.Context, accountID int64) error {
			app.refreshCodexBalance(ctx, accountID)
			return nil
		},
	})
	app.engine = engine
	return app, nil
}

func randomSecret(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	if _, err := cryptorand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func loadEmbeddedIndexHTML() []byte {
	b, err := fs.ReadFile(root.WebDistFS, "web/dist/index.html")
	if err != nil || len(b) == 0 {
		return nil
	}
	return b
}

func quotaProvider(st *store.Store, cfg config.Config) quota.Provider {
	reserveTTL := 2*time.Minute + 30*time.Second
	normal := quota.NewHybridProvider(st, reserveTTL, cfg.Billing.EnablePayAsYouGo)
	free := quota.NewFreeProvider(st, reserveTTL)
	return quota.NewFeatureProvider(st, cfg.SelfMode.Enable, normal, free)
}

func (a *App) Handler() http.Handler {
	return a.engine
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

func codexOAuthRedirectURI(addr string) string {
	if v := strings.TrimSpace(os.Getenv("REALMS_CODEX_OAUTH_REDIRECT_URI")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("CODEX_OAUTH_REDIRECT_URI")); v != "" {
		return v
	}
	_ = addr
	return codexoauth.DefaultRedirectURI
}

func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	type resp struct {
		OK      bool   `json:"ok"`
		Env     string `json:"env"`
		Version string `json:"version"`
		Date    string `json:"date"`

		DBOK bool `json:"db_ok"`

		AllowOpenRegistration    bool `json:"allow_open_registration"`
		EmailVerificationEnabled bool `json:"email_verification_enabled"`
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	dbOK := a.db.PingContext(ctx) == nil

	emailVerifEnabled := a.cfg.EmailVerif.Enable
	if a.store != nil {
		if v, ok, err := a.store.GetBoolAppSetting(ctx, store.SettingEmailVerificationEnable); err == nil && ok {
			emailVerifEnabled = v
		}
	}

	out := resp{
		OK:                       true,
		Env:                      a.cfg.Env,
		Version:                  a.version.Version,
		Date:                     a.version.Date,
		DBOK:                     dbOK,
		AllowOpenRegistration:    a.cfg.Security.AllowOpenRegistration,
		EmailVerificationEnabled: emailVerifEnabled,
	}

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

func (a *App) bootstrap() error {
	go a.usageCleanupLoop()
	go a.usageRetentionLoop()
	go a.sessionBindingsCleanupLoop()
	go a.codexBalanceRefreshLoop()
	go a.schedulerStateSweepLoop()
	go a.cacheInvalidationPollerLoop()
	if !a.cfg.SelfMode.Enable {
		go a.ticketAttachmentsCleanupLoop()
	}
	return nil
}

func (a *App) cacheInvalidationPollerLoop() {
	if a.store == nil {
		return
	}

	interval := 1 * time.Second
	if raw := strings.TrimSpace(os.Getenv("REALMS_CACHE_INVALIDATION_POLL_MILLIS")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			if n <= 0 {
				return
			}
			interval = time.Duration(n) * time.Millisecond
		}
	}
	if interval <= 0 {
		return
	}

	var (
		lastUpstreamSnapshotVersion int64
		hasUpstreamSnapshotVersion  bool
		lastTokenAuthVersion        int64
		hasTokenAuthVersion         bool
	)

	runOnce := func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		vers, ok, err := a.store.GetCacheInvalidationVersions(ctx, []string{
			store.CacheInvalidationKeyUpstreamSnapshot,
			store.CacheInvalidationKeyTokenAuth,
		})
		if err != nil {
			obs.RecordCacheInvalidationPollError()
			obs.RecordCacheInvalidationPollTick(false)
			return false
		}
		if !ok {
			// Older DB without cache_invalidation table.
			obs.RecordCacheInvalidationPollTick(true)
			return true
		}

		if a.sched != nil {
			if v, ok := vers[store.CacheInvalidationKeyUpstreamSnapshot]; ok {
				prev := lastUpstreamSnapshotVersion
				if !hasUpstreamSnapshotVersion {
					prev = 0
				}
				if v != prev {
					a.sched.InvalidateUpstreamSnapshot()
				}
				lastUpstreamSnapshotVersion = v
				hasUpstreamSnapshotVersion = true
				obs.SetCacheInvalidationVersion(store.CacheInvalidationKeyUpstreamSnapshot, v)
			}
		}

		if v, ok := vers[store.CacheInvalidationKeyTokenAuth]; ok {
			prev := lastTokenAuthVersion
			if !hasTokenAuthVersion {
				prev = 0
			}
			if v != prev {
				a.store.PurgeTokenAuthCacheAll()
			}
			lastTokenAuthVersion = v
			hasTokenAuthVersion = true
			obs.SetCacheInvalidationVersion(store.CacheInvalidationKeyTokenAuth, v)
		}
		obs.RecordCacheInvalidationPollTick(true)
		return true
	}

	runOnce()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	jitterMax := interval / 5 // 0-20%
	for {
		delay := interval
		if jitterMax > 0 {
			delta := time.Duration(rng.Int63n(int64(jitterMax)*2+1)) - jitterMax
			delay += delta
		}
		if delay < 0 {
			delay = 0
		}
		time.Sleep(delay)
		runOnce()
	}
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

func (a *App) usageRetentionLoop() {
	if a.store == nil {
		return
	}
	raw := strings.TrimSpace(os.Getenv("REALMS_USAGE_EVENTS_RETENTION_DAYS"))
	if raw == "" {
		return
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days <= 0 {
		return
	}

	const (
		interval      = 10 * time.Minute
		deleteBatch   = 5000
		backfillBatch = 200
		maxBackfill   = 10 // per tick
	)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		cutoff := time.Now().UTC().AddDate(0, 0, -days)

		// 先补算，避免删除未 rollup 的明细导致统计缺失。
		for i := 0; i < maxBackfill; i++ {
			n, err := a.store.BackfillUsageRollupsBefore(ctx, cutoff, backfillBatch)
			if err != nil || n < backfillBatch {
				break
			}
		}

		for {
			n, _ := a.store.DeleteUsageEventsBefore(ctx, cutoff, deleteBatch)
			if n < deleteBatch {
				break
			}
		}
		cancel()
	}
}

func (a *App) sessionBindingsCleanupLoop() {
	if a.store == nil {
		return
	}
	const (
		interval  = 3 * time.Minute
		batchSize = 2000
	)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		now := time.Now()
		for {
			n, _ := a.store.DeleteExpiredSessionBindings(ctx, now, batchSize)
			if n < batchSize {
				break
			}
		}
		cancel()
	}
}

func (a *App) schedulerStateSweepLoop() {
	if a.sched == nil {
		return
	}

	// Run once on startup, then periodically.
	a.sched.Sweep(time.Now())

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		a.sched.Sweep(time.Now())
	}
}

func (a *App) codexBalanceRefreshLoop() {
	if a.store == nil {
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
	client := codexoauth.NewClient(codexoauth.DefaultConfig(""))

	accessToken := sec.AccessToken
	refreshToken := sec.RefreshToken
	idTokenPtr := sec.IDToken
	expiresAt := sec.ExpiresAt

	maybeRefresh := func() bool {
		refreshed, err := client.Refresh(ctx, refreshToken)
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

	quota, err := client.FetchQuota(ctx, ep.BaseURL, accessToken, sec.AccountID)
	if err != nil {
		var se *codexoauth.HTTPStatusError
		if errors.As(err, &se) && (se.StatusCode == http.StatusUnauthorized || se.StatusCode == http.StatusForbidden) {
			if maybeRefresh() {
				quota, err = client.FetchQuota(ctx, ep.BaseURL, accessToken, sec.AccountID)
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

func parseLastIntPathSegment(path string) (int64, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		id, err := strconv.ParseInt(strings.TrimSpace(parts[i]), 10, 64)
		if err == nil && id > 0 {
			return id, true
		}
	}
	return 0, false
}

func (a *App) RefreshCodexQuotasByEndpoint(w http.ResponseWriter, r *http.Request) {
	endpointID, ok := parseLastIntPathSegment(r.URL.Path)
	if !ok {
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
	http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d#accounts", ch.ID), http.StatusFound)
}

func (a *App) RefreshCodexQuota(w http.ResponseWriter, r *http.Request) {
	accountID, ok := parseLastIntPathSegment(r.URL.Path)
	if !ok {
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
	http.Redirect(w, r, fmt.Sprintf("/admin/channels?open_channel_settings=%d#accounts", ch.ID), http.StatusFound)
}
