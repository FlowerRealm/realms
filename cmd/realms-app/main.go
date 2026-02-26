// realms-app 提供一个“可双击启动”的启动器形态：
// - 默认启用自用模式（REALMS_SELF_MODE_ENABLE=true）
// - 默认监听 :8080（多人访问）
// - 默认启用 CORS（REALMS_CORS_ALLOW_ORIGINS=*）
// - 启动后等待 /healthz 就绪并打开默认浏览器到 /login
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
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

	if err := applyAppDefaults(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
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
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := waitForHealthz(ctx, localBaseURL+"/healthz"); err != nil {
			slog.Warn("等待后端就绪超时", "err", err)
			return
		}
		if err := openBrowser(localBaseURL + "/login"); err != nil {
			slog.Warn("打开浏览器失败", "err", err)
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
	setDefaultEnv("REALMS_SELF_MODE_ENABLE", "true")
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

func waitForHealthz(ctx context.Context, url string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			err = fmt.Errorf("healthz not ok: %d", resp.StatusCode)
		}
		lastErr = err

		select {
		case <-ctx.Done():
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			return lastErr
		case <-ticker.C:
		}
	}
}

func openBrowser(url string) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("url 为空")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
