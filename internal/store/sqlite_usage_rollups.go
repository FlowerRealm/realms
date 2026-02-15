package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteUsageRollups(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite schema 修补事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1) usage_events.rollup_applied_at
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(usage_events)`)
	if err != nil {
		return fmt.Errorf("查询 usage_events 列信息失败: %w", err)
	}
	cols := make(map[string]struct{})
	for rows.Next() {
		var (
			cid        int
			name       string
			typ        string
			notNull    int
			dfltValue  sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dfltValue, &primaryKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("扫描 usage_events 列信息失败: %w", err)
		}
		if name != "" {
			cols[name] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("遍历 usage_events 列信息失败: %w", err)
	}
	_ = rows.Close()

	if _, ok := cols["rollup_applied_at"]; !ok {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE usage_events ADD COLUMN rollup_applied_at DATETIME NULL"); err != nil {
			return fmt.Errorf("添加 usage_events 列 rollup_applied_at 失败: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_usage_events_rollup_applied_at ON usage_events(rollup_applied_at)"); err != nil {
		return fmt.Errorf("创建 usage_events 索引 idx_usage_events_rollup_applied_at 失败: %w", err)
	}

	// 2) rollup tables
	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS usage_rollup_global_hour (
  bucket_start DATETIME NOT NULL,
  requests_total INTEGER NOT NULL DEFAULT 0,
  committed_usd DECIMAL(20,6) NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_output_tokens INTEGER NOT NULL DEFAULT 0,
  first_token_latency_ms_sum INTEGER NOT NULL DEFAULT 0,
  first_token_samples INTEGER NOT NULL DEFAULT 0,
  decode_latency_ms_sum INTEGER NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (bucket_start)
)
`); err != nil {
		return fmt.Errorf("创建 usage_rollup_global_hour 失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS usage_rollup_channel_hour (
  bucket_start DATETIME NOT NULL,
  upstream_channel_id INTEGER NOT NULL,
  requests_total INTEGER NOT NULL DEFAULT 0,
  committed_usd DECIMAL(20,6) NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_output_tokens INTEGER NOT NULL DEFAULT 0,
  first_token_latency_ms_sum INTEGER NOT NULL DEFAULT 0,
  first_token_samples INTEGER NOT NULL DEFAULT 0,
  decode_latency_ms_sum INTEGER NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (bucket_start, upstream_channel_id)
)
`); err != nil {
		return fmt.Errorf("创建 usage_rollup_channel_hour 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_usage_rollup_channel_hour_channel ON usage_rollup_channel_hour(upstream_channel_id, bucket_start)"); err != nil {
		return fmt.Errorf("创建 usage_rollup_channel_hour 索引失败: %w", err)
	}

	// 2.1) sharded rollup tables（global/channel hour）
	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS usage_rollup_global_hour_sharded (
  bucket_start DATETIME NOT NULL,
  shard INTEGER NOT NULL,
  requests_total INTEGER NOT NULL DEFAULT 0,
  committed_usd DECIMAL(20,6) NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_output_tokens INTEGER NOT NULL DEFAULT 0,
  first_token_latency_ms_sum INTEGER NOT NULL DEFAULT 0,
  first_token_samples INTEGER NOT NULL DEFAULT 0,
  decode_latency_ms_sum INTEGER NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (bucket_start, shard)
)
`); err != nil {
		return fmt.Errorf("创建 usage_rollup_global_hour_sharded 失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS usage_rollup_channel_hour_sharded (
  bucket_start DATETIME NOT NULL,
  upstream_channel_id INTEGER NOT NULL,
  shard INTEGER NOT NULL,
  requests_total INTEGER NOT NULL DEFAULT 0,
  committed_usd DECIMAL(20,6) NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_output_tokens INTEGER NOT NULL DEFAULT 0,
  first_token_latency_ms_sum INTEGER NOT NULL DEFAULT 0,
  first_token_samples INTEGER NOT NULL DEFAULT 0,
  decode_latency_ms_sum INTEGER NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (bucket_start, upstream_channel_id, shard)
)
`); err != nil {
		return fmt.Errorf("创建 usage_rollup_channel_hour_sharded 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_usage_rollup_channel_hour_sharded_channel ON usage_rollup_channel_hour_sharded(upstream_channel_id, bucket_start)"); err != nil {
		return fmt.Errorf("创建 usage_rollup_channel_hour_sharded 索引失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS usage_rollup_user_day (
  day DATETIME NOT NULL,
  user_id INTEGER NOT NULL,
  requests_total INTEGER NOT NULL DEFAULT 0,
  committed_usd DECIMAL(20,6) NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_output_tokens INTEGER NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (day, user_id)
)
`); err != nil {
		return fmt.Errorf("创建 usage_rollup_user_day 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_usage_rollup_user_day_user ON usage_rollup_user_day(user_id, day)"); err != nil {
		return fmt.Errorf("创建 usage_rollup_user_day 索引失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS usage_rollup_model_day (
  day DATETIME NOT NULL,
  model TEXT NOT NULL,
  requests_total INTEGER NOT NULL DEFAULT 0,
  committed_usd DECIMAL(20,6) NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_output_tokens INTEGER NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (day, model)
)
`); err != nil {
		return fmt.Errorf("创建 usage_rollup_model_day 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_usage_rollup_model_day_model ON usage_rollup_model_day(model, day)"); err != nil {
		return fmt.Errorf("创建 usage_rollup_model_day 索引失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}
