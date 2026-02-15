// realms-rollup-backfill 用于一次性补算 usage_events 的 rollup（上线后迁移历史数据）。
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	"realms/internal/config"
	"realms/internal/obs"
	"realms/internal/store"
)

func main() {
	_ = godotenv.Load()

	var (
		batch = flag.Int("batch", 500, "backfill batch size (max 5000)")
		maxN  = flag.Int("max", 0, "max events to backfill (0 = unlimited)")
		sleep = flag.Duration("sleep", 0, "sleep between batches (e.g. 200ms)")
	)
	flag.Parse()

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

	st := store.New(db)
	st.SetDialect(dialect)
	st.SetAppSettingsDefaults(cfg.AppSettingsDefaults)

	before := time.Now().UTC().AddDate(100, 0, 0)
	total := 0
	t0 := time.Now()
	for {
		if *maxN > 0 && total >= *maxN {
			break
		}
		limit := *batch
		if *maxN > 0 && total+limit > *maxN {
			limit = *maxN - total
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		n, err := st.BackfillUsageRollupsBefore(ctx, before, limit)
		cancel()
		if err != nil {
			slog.Error("backfill 失败", "err", err)
			os.Exit(1)
		}
		total += n

		elapsed := time.Since(t0)
		rate := float64(total) / elapsed.Seconds()
		slog.Info("backfill progress", "dialect", dialect, "batch", n, "total", total, "rate_per_sec", fmt.Sprintf("%.1f", rate))

		if n < limit {
			break
		}
		if *sleep > 0 {
			time.Sleep(*sleep)
		}
	}

	slog.Info("backfill complete", "total", total, "elapsed", time.Since(t0).String())
}

