// realms 是一个单体的 OpenAI 风格 API 中转服务入口，提供数据面代理与管理面控制台。
package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
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
		if err := store.ApplyMigrations(db); err != nil {
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

	var oauthServer *http.Server
	if cfg.CodexOAuth.Enable {
		h := app.CodexOAuthCallbackHandler()
		if h == nil {
			slog.Error("Codex OAuth 已启用但回调 handler 为空")
			os.Exit(1)
		}

		callbackAddr := strings.TrimSpace(cfg.CodexOAuth.CallbackListenAddr)
		if callbackAddr != "" && !sameTCPPort(cfg.Server.Addr, callbackAddr) {
			oauthServer = &http.Server{
				Addr:    callbackAddr,
				Handler: h,
			}
			ln, err := net.Listen("tcp", oauthServer.Addr)
			if err != nil {
				slog.Error("Codex OAuth 回调监听启动失败", "addr", oauthServer.Addr, "err", err)
				os.Exit(1)
			}
			go func() {
				slog.Info("Codex OAuth 回调监听启动", "addr", ln.Addr().String())
				if err := oauthServer.Serve(ln); err != nil && err != http.ErrServerClosed {
					slog.Error("Codex OAuth 回调监听异常退出", "err", err)
				}
			}()
			httpServer.RegisterOnShutdown(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = oauthServer.Shutdown(ctx)
			})
		} else {
			slog.Info("Codex OAuth 未启动独立回调监听", "callback_listen_addr", callbackAddr)
		}
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

func sameTCPPort(a string, b string) bool {
	pa := tcpPort(a)
	pb := tcpPort(b)
	return pa != "" && pa == pb
}

func tcpPort(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if _, port, err := net.SplitHostPort(addr); err == nil {
		return port
	}
	if !strings.Contains(addr, ":") {
		if _, port, err := net.SplitHostPort(":" + addr); err == nil {
			return port
		}
	}
	return ""
}
