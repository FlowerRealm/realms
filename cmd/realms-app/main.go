// realms-app 提供一个“可双击启动”的启动器形态：
// - 默认启用 personal 模式（REALMS_MODE=personal）
// - 默认监听 :8080（多人访问）
// - 默认启用 CORS（REALMS_CORS_ALLOW_ORIGINS=*）
// - 启动后打印 /login 访问地址（不自动打开浏览器）
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	"realms/internal/config"
	"realms/internal/obs"
	"realms/internal/server"
	"realms/internal/store"
	"realms/internal/version"
)

func main() {
	_ = godotenv.Load()

	var (
		doSet bool
		key   string
		quiet bool
	)
	flag.BoolVar(&doSet, "set", false, "set/overwrite personal-mode management key and exit")
	flag.StringVar(&key, "key", "", "new management key (will not be printed)")
	flag.BoolVar(&quiet, "quiet", false, "suppress success output")
	flag.Parse()

	if err := applyAppDefaults(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if doSet {
		key = strings.TrimSpace(key)
		if key == "" {
			fmt.Fprintln(os.Stderr, "--key 不能为空")
			os.Exit(2)
		}
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		slog.Error("加载配置失败", "err", err)
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
	case store.DialectMySQL:
		if err := store.ApplyMigrations(context.Background(), db, store.MigrationOptions{
			LockName:    cfg.DB.MigrationLockName,
			LockTimeout: time.Duration(cfg.DB.MigrationLockTimeoutSeconds) * time.Second,
		}); err != nil {
			slog.Error("执行数据库迁移失败", "err", err)
			os.Exit(1)
		}
	case store.DialectSQLite:
		if err := store.EnsureSQLiteSchema(db); err != nil {
			slog.Error("初始化 SQLite schema 失败", "err", err)
			os.Exit(1)
		}
	default:
		slog.Error("未知数据库方言", "dialect", dialect)
		os.Exit(1)
	}

	if doSet {
		st := store.New(db)
		st.SetDialect(dialect)
		if err := st.SetPersonalModeKey(context.Background(), key); err != nil {
			slog.Error("重设管理 Key 失败", "err", err)
			os.Exit(1)
		}
		if !quiet {
			slog.Info("管理 Key 已更新（旧 Key 已失效）")
		}
		return
	}

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
		slog.Info("服务启动", "addr", ln.Addr().String(), "version", version.Info().Version)
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	localBaseURL := localURLFromListener(ln)
	slog.Info("访问控制台", "url", localBaseURL+"/login")

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
	slog.Info("服务已退出")
}

func setDefaultEnv(key string, value string) {
	if strings.TrimSpace(os.Getenv(key)) != "" {
		return
	}
	_ = os.Setenv(key, value)
}

func applyAppDefaults() error {
	setDefaultEnv("REALMS_ENV", "app")
	setDefaultEnv("REALMS_MODE", "personal")
	setDefaultEnv("REALMS_ADDR", ":8080")
	setDefaultEnv("REALMS_DB_DRIVER", "sqlite")
	setDefaultEnv("REALMS_DISABLE_SECURE_COOKIES", "true")
	setDefaultEnv("FRONTEND_BASE_URL", "")
	setDefaultEnv("REALMS_CORS_ALLOW_ORIGINS", "*")

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("获取用户配置目录失败: %w", err)
	}
	dataDir := filepath.Join(cfgDir, "Realms")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}
	setDefaultEnv("REALMS_SQLITE_PATH", filepath.Join(dataDir, "realms.db")+"?_busy_timeout=30000")
	setDefaultEnv("REALMS_TICKETS_ATTACHMENTS_DIR", filepath.Join(dataDir, "tickets"))
	return nil
}

func localURLFromListener(ln net.Listener) string {
	addr := ln.Addr().String()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// 兜底：尝试去掉 IPv6 bracket 等，最终回退到固定端口。
		return "http://127.0.0.1:8080"
	}
	_ = host
	if strings.TrimSpace(port) == "" {
		return "http://127.0.0.1"
	}
	return "http://127.0.0.1:" + port
}
