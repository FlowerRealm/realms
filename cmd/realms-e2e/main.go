// realms-e2e 是 Playwright Web E2E 专用的启动器：
// - 启动时自动创建临时 SQLite 并 seed 最小数据集（root 用户/公告/工单/OAuth App/充值订单）
// - 仅用于 CI/本地 E2E，不用于生产或发布构建
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/obs"
	"realms/internal/server"
	"realms/internal/store"
	"realms/internal/version"
)

const (
	defaultAddr = "127.0.0.1:18181"

	e2eRootEmail    = "root@example.com"
	e2eRootUsername = "root"
	e2eRootPassword = "rootpass123"

	e2eOAuthClientID     = "oa_playwright_e2e"
	e2eOAuthClientSecret = "oas_playwright_e2e_secret"
	e2eOAuthAppName      = "Playwright E2E App"
	e2eOAuthRedirectURI  = "https://example.com/callback"

	e2eAnnouncementTitle = "Playwright E2E Announcement"
	e2eAnnouncementBody  = "This is a seeded announcement for Playwright E2E."

	e2eTicketOpenSubject   = "Playwright E2E Ticket (Open)"
	e2eTicketClosedSubject = "Playwright E2E Ticket (Closed)"

	// Billing E2E seed（与 web/e2e/seed.ts 保持一致）
	e2eModelPublicID = "gpt-4.1-mini"

	e2eUserEmail           = "e2e-user@example.com"
	e2eUserUsername        = "e2e-user"
	e2eUserTokenPlain      = "sk_playwright_e2e_user_token"
	e2eUserInitialBalance  = "1"
	e2ePoorUserEmail       = "e2e-poor@example.com"
	e2ePoorUserUsername    = "e2e-poor"
	e2ePoorUserTokenPlain  = "sk_playwright_e2e_poor_token"
	e2ePoorUserInitialBal  = "0.0005" // < 0.001 USD default reserve
	e2eUpstreamAPIKeyPlain = "sk_upstream_playwright_e2e"
)

func envOr(key string, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func ensureSQLiteQuery(path string) string {
	if strings.Contains(path, "?") || path == ":memory:" || strings.HasPrefix(path, "file::memory:") {
		return path
	}
	return path + "?_busy_timeout=30000"
}

func main() {
	addr := envOr("REALMS_E2E_ADDR", defaultAddr)
	workDir := strings.TrimSpace(os.Getenv("REALMS_E2E_WORKDIR"))
	dbPath := strings.TrimSpace(os.Getenv("REALMS_E2E_DB_PATH"))
	frontendDistDir := strings.TrimSpace(os.Getenv("REALMS_E2E_FRONTEND_DIST_DIR"))

	if workDir == "" {
		dir, err := os.MkdirTemp("", "realms-e2e-*")
		if err != nil {
			fmt.Fprintln(os.Stderr, "创建临时目录失败:", err)
			os.Exit(1)
		}
		workDir = dir
	}
	if dbPath == "" {
		dbPath = filepath.Join(workDir, "realms.sqlite")
	}

	os.Setenv("REALMS_ENV", "dev")
	os.Setenv("REALMS_ADDR", addr)
	os.Setenv("REALMS_DB_DRIVER", "sqlite")
	os.Setenv("REALMS_SQLITE_PATH", ensureSQLiteQuery(dbPath))
	os.Setenv("REALMS_TICKETS_ATTACHMENTS_DIR", filepath.Join(workDir, "tickets"))

	if frontendDistDir != "" {
		os.Setenv("FRONTEND_DIST_DIR", frontendDistDir)
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "加载配置失败:", err)
		os.Exit(1)
	}

	logger := obs.NewLogger(cfg.Env)
	slog.SetDefault(logger)

	db, dialect, err := store.OpenDB(cfg.Env, cfg.DB.Driver, cfg.DB.DSN, cfg.DB.SQLitePath)
	if err != nil {
		slog.Error("连接数据库失败", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	switch dialect {
	case store.DialectSQLite:
		if err := store.EnsureSQLiteSchema(db); err != nil {
			slog.Error("初始化 SQLite schema 失败", "err", err)
			os.Exit(1)
		}
	default:
		slog.Error("realms-e2e 仅支持 SQLite", "dialect", dialect)
		os.Exit(1)
	}

	st := store.New(db)
	st.SetDialect(dialect)
	st.SetAppSettingsDefaults(cfg.AppSettingsDefaults)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		_ = r.Body.Close()

		resp := map[string]any{
			"id":     "resp_pw_e2e_1",
			"object": "response",
			"model":  e2eModelPublicID,
			"output": []any{
				map[string]any{
					"type": "message",
					"role": "assistant",
					"content": []any{
						map[string]any{"type": "output_text", "text": "OK"},
					},
				},
			},
			"usage": map[string]any{
				"input_tokens":  1000,
				"output_tokens": 1,
			},
			"status": "completed",
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	upstreamBaseURL := strings.TrimRight(strings.TrimSpace(upstream.URL), "/") + "/v1"

	seed, err := seedE2EData(context.Background(), st, cfg, upstreamBaseURL)
	if err != nil {
		slog.Error("seed 失败", "err", err)
		os.Exit(1)
	}
	slog.Info("e2e seed 完成",
		"user_id", seed.RootUserID,
		"e2e_user_id", seed.E2EUserID,
		"poor_user_id", seed.PoorUserID,
		"announcement_id", seed.AnnouncementID,
		"ticket_open_id", seed.TicketOpenID,
		"ticket_closed_id", seed.TicketClosedID,
		"oauth_app_id", seed.OAuthAppID,
		"topup_order_id", seed.TopupOrderID,
		"upstream_base_url", upstreamBaseURL,
	)

	app, err := server.NewApp(server.AppOptions{
		Config:  cfg,
		DB:      db,
		Version: version.Info(),
	})
	if err != nil {
		slog.Error("初始化服务失败", "err", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:    cfg.Server.Addr,
		Handler: app.Handler(),
	}

	serverErr := make(chan error, 1)
	ln, err := net.Listen("tcp", cfg.Server.Addr)
	if err != nil {
		slog.Error("HTTP 服务监听启动失败", "addr", cfg.Server.Addr, "err", err)
		os.Exit(1)
	}
	go func() {
		slog.Info("E2E 服务启动", "addr", ln.Addr().String())
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-stop:
	case err := <-serverErr:
		slog.Error("HTTP 服务异常退出", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("优雅停机失败", "err", err)
		_ = httpServer.Close()
	}
	slog.Info("E2E 服务已退出")
}

type e2eSeedResult struct {
	RootUserID     int64
	E2EUserID      int64
	PoorUserID     int64
	AnnouncementID int64
	TicketOpenID   int64
	TicketClosedID int64
	OAuthAppID     int64
	TopupOrderID   int64
}

func seedE2EData(ctx context.Context, st *store.Store, cfg config.Config, upstreamBaseURL string) (e2eSeedResult, error) {
	if st == nil {
		return e2eSeedResult{}, errors.New("store 为空")
	}

	u, err := st.GetUserByUsername(ctx, e2eRootUsername)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return e2eSeedResult{}, fmt.Errorf("查询 root 用户失败: %w", err)
	}
	userID := u.ID
	if userID == 0 {
		hash, err := auth.HashPassword(e2eRootPassword)
		if err != nil {
			return e2eSeedResult{}, fmt.Errorf("生成 root 密码失败: %w", err)
		}
		id, err := st.CreateUser(ctx, e2eRootEmail, e2eRootUsername, hash, store.UserRoleRoot)
		if err != nil {
			return e2eSeedResult{}, fmt.Errorf("创建 root 用户失败: %w", err)
		}
		userID = id
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "pw-e2e-upstream", store.DefaultGroupName, 0, false, false, false, false)
	if err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建 upstream_channel 失败: %w", err)
	}
	epID, err := st.CreateUpstreamEndpoint(ctx, channelID, strings.TrimRight(strings.TrimSpace(upstreamBaseURL), "/"), 0)
	if err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建 upstream_endpoint 失败: %w", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, epID, strPtr("pw-e2e"), e2eUpstreamAPIKeyPlain); err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建 upstream_credential 失败: %w", err)
	}

	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            e2eModelPublicID,
		OwnedBy:             strPtr("upstream"),
		InputUSDPer1M:       decimal.RequireFromString("10"),
		OutputUSDPer1M:      decimal.Zero,
		CacheInputUSDPer1M:  decimal.Zero,
		CacheOutputUSDPer1M: decimal.Zero,
		Status:              1,
	}); err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建 managed_model 失败: %w", err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     channelID,
		PublicID:      e2eModelPublicID,
		UpstreamModel: e2eModelPublicID,
		Status:        1,
	}); err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建 channel_model 失败: %w", err)
	}

	userHash, err := auth.HashPassword("pw-e2e-user-123")
	if err != nil {
		return e2eSeedResult{}, fmt.Errorf("生成 e2e 用户密码失败: %w", err)
	}
	e2eUserID, err := st.CreateUser(ctx, e2eUserEmail, e2eUserUsername, userHash, store.UserRoleUser)
	if err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建 e2e 用户失败: %w", err)
	}
	if _, err := st.AddUserBalanceUSD(ctx, e2eUserID, decimal.RequireFromString(e2eUserInitialBalance)); err != nil {
		return e2eSeedResult{}, fmt.Errorf("写入 e2e 用户余额失败: %w", err)
	}
	if _, _, err := st.CreateUserToken(ctx, e2eUserID, strPtr("pw-e2e-user"), e2eUserTokenPlain); err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建 e2e 用户 token 失败: %w", err)
	}

	poorUserID, err := st.CreateUser(ctx, e2ePoorUserEmail, e2ePoorUserUsername, userHash, store.UserRoleUser)
	if err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建 poor 用户失败: %w", err)
	}
	if _, err := st.AddUserBalanceUSD(ctx, poorUserID, decimal.RequireFromString(e2ePoorUserInitialBal)); err != nil {
		return e2eSeedResult{}, fmt.Errorf("写入 poor 用户余额失败: %w", err)
	}
	if _, _, err := st.CreateUserToken(ctx, poorUserID, strPtr("pw-e2e-poor"), e2ePoorUserTokenPlain); err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建 poor 用户 token 失败: %w", err)
	}

	announcementID, err := st.CreateAnnouncement(ctx, e2eAnnouncementTitle, e2eAnnouncementBody, store.AnnouncementStatusPublished)
	if err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建公告失败: %w", err)
	}

	ticketOpenID, _, err := st.CreateTicketWithMessageAndAttachments(ctx, userID, e2eTicketOpenSubject, "hello", nil)
	if err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建 open 工单失败: %w", err)
	}

	ticketClosedID, _, err := st.CreateTicketWithMessageAndAttachments(ctx, userID, e2eTicketClosedSubject, "close me", nil)
	if err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建 closed 工单失败: %w", err)
	}
	_ = st.CloseTicket(ctx, ticketClosedID)

	secretHash, err := auth.HashPassword(e2eOAuthClientSecret)
	if err != nil {
		return e2eSeedResult{}, fmt.Errorf("生成 oauth app secret 失败: %w", err)
	}
	oauthAppID, err := st.CreateOAuthApp(ctx, e2eOAuthClientID, e2eOAuthAppName, secretHash, store.OAuthAppStatusEnabled)
	if err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建 oauth app 失败: %w", err)
	}
	if err := st.ReplaceOAuthAppRedirectURIs(ctx, oauthAppID, []string{e2eOAuthRedirectURI}); err != nil {
		return e2eSeedResult{}, fmt.Errorf("写入 oauth redirect_uri 失败: %w", err)
	}

	amountCNY := decimal.NewFromInt(10)
	creditUSD := amountCNY.Mul(cfg.Billing.CreditUSDPerCNY).Truncate(store.USDScale)
	if creditUSD.LessThanOrEqual(decimal.Zero) {
		creditUSD = decimal.NewFromFloat(1.4)
	}
	order, err := st.CreateTopupOrder(ctx, userID, amountCNY, creditUSD, time.Now())
	if err != nil {
		return e2eSeedResult{}, fmt.Errorf("创建充值订单失败: %w", err)
	}

	// 输出常用 seed 常量，便于 Playwright 侧复用（仅日志输出，不写入数据库）。
	slog.Info("e2e seed constants",
		"root_username", e2eRootUsername,
		"root_password_len", strconv.Itoa(len(e2eRootPassword)),
		"oauth_client_id", e2eOAuthClientID,
		"oauth_redirect_uri", e2eOAuthRedirectURI,
		"model_public_id", e2eModelPublicID,
		"e2e_user_username", e2eUserUsername,
		"e2e_user_token_prefix", "sk_",
		"poor_user_username", e2ePoorUserUsername,
		"poor_user_token_prefix", "sk_",
	)

	return e2eSeedResult{
		RootUserID:     userID,
		E2EUserID:      e2eUserID,
		PoorUserID:     poorUserID,
		AnnouncementID: announcementID,
		TicketOpenID:   ticketOpenID,
		TicketClosedID: ticketClosedID,
		OAuthAppID:     oauthAppID,
		TopupOrderID:   order.ID,
	}, nil
}

func strPtr(s string) *string {
	v := strings.TrimSpace(s)
	if v == "" {
		return nil
	}
	return &v
}
