package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/shopspring/decimal"
)

func shouldUseUsageRollups(since, until time.Time) bool {
	if since.IsZero() || until.IsZero() {
		return false
	}
	if !until.After(since) {
		return false
	}
	// 避免 1-minute window（RPM/TPM）等“短窗口”被 hour rollup 放大。
	return until.Sub(since) >= time.Hour
}

func isMissingTableErr(err error) bool {
	if err == nil {
		return false
	}
	// SQLite
	if strings.Contains(err.Error(), "no such table") {
		return true
	}
	// MySQL: ER_NO_SUCH_TABLE = 1146
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		return myErr.Number == 1146
	}
	return false
}

func (s *Store) applyUsageRollupsIfAvailable(ctx context.Context, usageEventID int64) error {
	if s == nil || s.db == nil || usageEventID <= 0 {
		return nil
	}
	// 用一次轻量探测来避免在未迁移的库上反复报错。
	var v int
	shardingEnabled := s.usageRollupShards > 0 && !s.usageRollupShardsCutoverAt.IsZero()
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM usage_rollup_global_hour LIMIT 1`).Scan(&v)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		if isMissingTableErr(err) {
			return nil
		}
		// 其他错误（权限/连接）交给调用方处理。
		return fmt.Errorf("探测 usage rollup 表失败: %w", err)
	}
	if shardingEnabled {
		err := s.db.QueryRowContext(ctx, `SELECT 1 FROM usage_rollup_global_hour_sharded LIMIT 1`).Scan(&v)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			if isMissingTableErr(err) {
				return nil
			}
			return fmt.Errorf("探测 usage rollup sharded 表失败: %w", err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 rollup 事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 幂等：每个 usage_event 只 rollup 一次。
	res, err := tx.ExecContext(ctx, `
UPDATE usage_events
SET rollup_applied_at=CURRENT_TIMESTAMP
WHERE id=? AND rollup_applied_at IS NULL
`, usageEventID)
	if err != nil {
		if isMissingTableErr(err) {
			return nil
		}
		return fmt.Errorf("标记 usage_event rollup_applied_at 失败: %w", err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return nil
	}

	useSharded := false
	if shardingEnabled {
		var t time.Time
		if err := tx.QueryRowContext(ctx, `SELECT time FROM usage_events WHERE id=?`, usageEventID).Scan(&t); err == nil && !t.IsZero() {
			useSharded = !t.Before(s.usageRollupShardsCutoverAt)
		}
	}

	if useSharded {
		shard := int(usageEventID % int64(s.usageRollupShards))
		if err := upsertUsageRollupGlobalHourSharded(ctx, tx, s.dialect, usageEventID, shard); err != nil {
			return err
		}
		if err := upsertUsageRollupChannelHourSharded(ctx, tx, s.dialect, usageEventID, shard); err != nil {
			return err
		}
	} else {
		if err := upsertUsageRollupGlobalHour(ctx, tx, s.dialect, usageEventID); err != nil {
			return err
		}
		if err := upsertUsageRollupChannelHour(ctx, tx, s.dialect, usageEventID); err != nil {
			return err
		}
	}
	if err := upsertUsageRollupUserDay(ctx, tx, s.dialect, usageEventID); err != nil {
		return err
	}
	if err := upsertUsageRollupModelDay(ctx, tx, s.dialect, usageEventID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 rollup 事务失败: %w", err)
	}
	return nil
}

func upsertUsageRollupGlobalHourSharded(ctx context.Context, tx *sql.Tx, d Dialect, usageEventID int64, shard int) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	query := `
INSERT INTO usage_rollup_global_hour_sharded(
  bucket_start, shard, requests_total, committed_usd,
  input_tokens, cached_input_tokens, output_tokens, cached_output_tokens,
  first_token_latency_ms_sum, first_token_samples, decode_latency_ms_sum, updated_at
)
SELECT
  DATE_FORMAT(time, '%Y-%m-%d %H:00:00'),
  ?,
  1,
  CASE WHEN state='` + UsageStateCommitted + `' THEN committed_usd ELSE 0 END,
  COALESCE(input_tokens, 0),
  COALESCE(cached_input_tokens, 0),
  COALESCE(output_tokens, 0),
  COALESCE(cached_output_tokens, 0),
  CASE WHEN first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END,
  CASE WHEN first_token_latency_ms > 0 THEN 1 ELSE 0 END,
  CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END,
  CURRENT_TIMESTAMP
FROM usage_events
WHERE id=?
ON DUPLICATE KEY UPDATE
  requests_total = requests_total + VALUES(requests_total),
  committed_usd = committed_usd + VALUES(committed_usd),
  input_tokens = input_tokens + VALUES(input_tokens),
  cached_input_tokens = cached_input_tokens + VALUES(cached_input_tokens),
  output_tokens = output_tokens + VALUES(output_tokens),
  cached_output_tokens = cached_output_tokens + VALUES(cached_output_tokens),
  first_token_latency_ms_sum = first_token_latency_ms_sum + VALUES(first_token_latency_ms_sum),
  first_token_samples = first_token_samples + VALUES(first_token_samples),
  decode_latency_ms_sum = decode_latency_ms_sum + VALUES(decode_latency_ms_sum),
  updated_at = CURRENT_TIMESTAMP
`
	if d == DialectSQLite {
		query = `
INSERT INTO usage_rollup_global_hour_sharded(
  bucket_start, shard, requests_total, committed_usd,
  input_tokens, cached_input_tokens, output_tokens, cached_output_tokens,
  first_token_latency_ms_sum, first_token_samples, decode_latency_ms_sum, updated_at
)
SELECT
  STRFTIME('%Y-%m-%d %H:00:00', time),
  ?,
  1,
  CASE WHEN state='` + UsageStateCommitted + `' THEN committed_usd ELSE 0 END,
  COALESCE(input_tokens, 0),
  COALESCE(cached_input_tokens, 0),
  COALESCE(output_tokens, 0),
  COALESCE(cached_output_tokens, 0),
  CASE WHEN first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END,
  CASE WHEN first_token_latency_ms > 0 THEN 1 ELSE 0 END,
  CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END,
  CURRENT_TIMESTAMP
FROM usage_events
WHERE id=?
ON CONFLICT(bucket_start, shard) DO UPDATE SET
  requests_total = requests_total + excluded.requests_total,
  committed_usd = committed_usd + excluded.committed_usd,
  input_tokens = input_tokens + excluded.input_tokens,
  cached_input_tokens = cached_input_tokens + excluded.cached_input_tokens,
  output_tokens = output_tokens + excluded.output_tokens,
  cached_output_tokens = cached_output_tokens + excluded.cached_output_tokens,
  first_token_latency_ms_sum = first_token_latency_ms_sum + excluded.first_token_latency_ms_sum,
  first_token_samples = first_token_samples + excluded.first_token_samples,
  decode_latency_ms_sum = decode_latency_ms_sum + excluded.decode_latency_ms_sum,
  updated_at = CURRENT_TIMESTAMP
`
	}

	if _, err := tx.ExecContext(ctx, query, shard, usageEventID); err != nil {
		if isMissingTableErr(err) {
			return nil
		}
		return fmt.Errorf("写入 usage_rollup_global_hour_sharded 失败: %w", err)
	}
	return nil
}

func upsertUsageRollupChannelHourSharded(ctx context.Context, tx *sql.Tx, d Dialect, usageEventID int64, shard int) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	query := `
INSERT INTO usage_rollup_channel_hour_sharded(
  bucket_start, upstream_channel_id, shard, requests_total, committed_usd,
  input_tokens, cached_input_tokens, output_tokens, cached_output_tokens,
  first_token_latency_ms_sum, first_token_samples, decode_latency_ms_sum, updated_at
)
SELECT
  DATE_FORMAT(time, '%Y-%m-%d %H:00:00'),
  upstream_channel_id,
  ?,
  1,
  CASE WHEN state='` + UsageStateCommitted + `' THEN committed_usd ELSE 0 END,
  COALESCE(input_tokens, 0),
  COALESCE(cached_input_tokens, 0),
  COALESCE(output_tokens, 0),
  COALESCE(cached_output_tokens, 0),
  CASE WHEN first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END,
  CASE WHEN first_token_latency_ms > 0 THEN 1 ELSE 0 END,
  CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END,
  CURRENT_TIMESTAMP
FROM usage_events
WHERE id=? AND upstream_channel_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  requests_total = requests_total + VALUES(requests_total),
  committed_usd = committed_usd + VALUES(committed_usd),
  input_tokens = input_tokens + VALUES(input_tokens),
  cached_input_tokens = cached_input_tokens + VALUES(cached_input_tokens),
  output_tokens = output_tokens + VALUES(output_tokens),
  cached_output_tokens = cached_output_tokens + VALUES(cached_output_tokens),
  first_token_latency_ms_sum = first_token_latency_ms_sum + VALUES(first_token_latency_ms_sum),
  first_token_samples = first_token_samples + VALUES(first_token_samples),
  decode_latency_ms_sum = decode_latency_ms_sum + VALUES(decode_latency_ms_sum),
  updated_at = CURRENT_TIMESTAMP
`
	if d == DialectSQLite {
		query = `
INSERT INTO usage_rollup_channel_hour_sharded(
  bucket_start, upstream_channel_id, shard, requests_total, committed_usd,
  input_tokens, cached_input_tokens, output_tokens, cached_output_tokens,
  first_token_latency_ms_sum, first_token_samples, decode_latency_ms_sum, updated_at
)
SELECT
  STRFTIME('%Y-%m-%d %H:00:00', time),
  upstream_channel_id,
  ?,
  1,
  CASE WHEN state='` + UsageStateCommitted + `' THEN committed_usd ELSE 0 END,
  COALESCE(input_tokens, 0),
  COALESCE(cached_input_tokens, 0),
  COALESCE(output_tokens, 0),
  COALESCE(cached_output_tokens, 0),
  CASE WHEN first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END,
  CASE WHEN first_token_latency_ms > 0 THEN 1 ELSE 0 END,
  CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END,
  CURRENT_TIMESTAMP
FROM usage_events
WHERE id=? AND upstream_channel_id IS NOT NULL
ON CONFLICT(bucket_start, upstream_channel_id, shard) DO UPDATE SET
  requests_total = requests_total + excluded.requests_total,
  committed_usd = committed_usd + excluded.committed_usd,
  input_tokens = input_tokens + excluded.input_tokens,
  cached_input_tokens = cached_input_tokens + excluded.cached_input_tokens,
  output_tokens = output_tokens + excluded.output_tokens,
  cached_output_tokens = cached_output_tokens + excluded.cached_output_tokens,
  first_token_latency_ms_sum = first_token_latency_ms_sum + excluded.first_token_latency_ms_sum,
  first_token_samples = first_token_samples + excluded.first_token_samples,
  decode_latency_ms_sum = decode_latency_ms_sum + excluded.decode_latency_ms_sum,
  updated_at = CURRENT_TIMESTAMP
`
	}

	if _, err := tx.ExecContext(ctx, query, shard, usageEventID); err != nil {
		if isMissingTableErr(err) {
			return nil
		}
		return fmt.Errorf("写入 usage_rollup_channel_hour_sharded 失败: %w", err)
	}
	return nil
}

func upsertUsageRollupGlobalHour(ctx context.Context, tx *sql.Tx, d Dialect, usageEventID int64) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	query := `
INSERT INTO usage_rollup_global_hour(
  bucket_start, requests_total, committed_usd,
  input_tokens, cached_input_tokens, output_tokens, cached_output_tokens,
  first_token_latency_ms_sum, first_token_samples, decode_latency_ms_sum, updated_at
)
SELECT
  DATE_FORMAT(time, '%Y-%m-%d %H:00:00'),
  1,
  CASE WHEN state='` + UsageStateCommitted + `' THEN committed_usd ELSE 0 END,
  COALESCE(input_tokens, 0),
  COALESCE(cached_input_tokens, 0),
  COALESCE(output_tokens, 0),
  COALESCE(cached_output_tokens, 0),
  CASE WHEN first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END,
  CASE WHEN first_token_latency_ms > 0 THEN 1 ELSE 0 END,
  CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END,
  CURRENT_TIMESTAMP
FROM usage_events
WHERE id=?
ON DUPLICATE KEY UPDATE
  requests_total = requests_total + VALUES(requests_total),
  committed_usd = committed_usd + VALUES(committed_usd),
  input_tokens = input_tokens + VALUES(input_tokens),
  cached_input_tokens = cached_input_tokens + VALUES(cached_input_tokens),
  output_tokens = output_tokens + VALUES(output_tokens),
  cached_output_tokens = cached_output_tokens + VALUES(cached_output_tokens),
  first_token_latency_ms_sum = first_token_latency_ms_sum + VALUES(first_token_latency_ms_sum),
  first_token_samples = first_token_samples + VALUES(first_token_samples),
  decode_latency_ms_sum = decode_latency_ms_sum + VALUES(decode_latency_ms_sum),
  updated_at = CURRENT_TIMESTAMP
`
	if d == DialectSQLite {
		query = `
INSERT INTO usage_rollup_global_hour(
  bucket_start, requests_total, committed_usd,
  input_tokens, cached_input_tokens, output_tokens, cached_output_tokens,
  first_token_latency_ms_sum, first_token_samples, decode_latency_ms_sum, updated_at
)
SELECT
  STRFTIME('%Y-%m-%d %H:00:00', time),
  1,
  CASE WHEN state='` + UsageStateCommitted + `' THEN committed_usd ELSE 0 END,
  COALESCE(input_tokens, 0),
  COALESCE(cached_input_tokens, 0),
  COALESCE(output_tokens, 0),
  COALESCE(cached_output_tokens, 0),
  CASE WHEN first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END,
  CASE WHEN first_token_latency_ms > 0 THEN 1 ELSE 0 END,
  CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END,
  CURRENT_TIMESTAMP
FROM usage_events
WHERE id=?
ON CONFLICT(bucket_start) DO UPDATE SET
  requests_total = requests_total + excluded.requests_total,
  committed_usd = committed_usd + excluded.committed_usd,
  input_tokens = input_tokens + excluded.input_tokens,
  cached_input_tokens = cached_input_tokens + excluded.cached_input_tokens,
  output_tokens = output_tokens + excluded.output_tokens,
  cached_output_tokens = cached_output_tokens + excluded.cached_output_tokens,
  first_token_latency_ms_sum = first_token_latency_ms_sum + excluded.first_token_latency_ms_sum,
  first_token_samples = first_token_samples + excluded.first_token_samples,
  decode_latency_ms_sum = decode_latency_ms_sum + excluded.decode_latency_ms_sum,
  updated_at = CURRENT_TIMESTAMP
`
	}

	if _, err := tx.ExecContext(ctx, query, usageEventID); err != nil {
		if isMissingTableErr(err) {
			return nil
		}
		return fmt.Errorf("写入 usage_rollup_global_hour 失败: %w", err)
	}
	return nil
}

func upsertUsageRollupChannelHour(ctx context.Context, tx *sql.Tx, d Dialect, usageEventID int64) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	query := `
INSERT INTO usage_rollup_channel_hour(
  bucket_start, upstream_channel_id, requests_total, committed_usd,
  input_tokens, cached_input_tokens, output_tokens, cached_output_tokens,
  first_token_latency_ms_sum, first_token_samples, decode_latency_ms_sum, updated_at
)
SELECT
  DATE_FORMAT(time, '%Y-%m-%d %H:00:00'),
  upstream_channel_id,
  1,
  CASE WHEN state='` + UsageStateCommitted + `' THEN committed_usd ELSE 0 END,
  COALESCE(input_tokens, 0),
  COALESCE(cached_input_tokens, 0),
  COALESCE(output_tokens, 0),
  COALESCE(cached_output_tokens, 0),
  CASE WHEN first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END,
  CASE WHEN first_token_latency_ms > 0 THEN 1 ELSE 0 END,
  CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END,
  CURRENT_TIMESTAMP
FROM usage_events
WHERE id=? AND upstream_channel_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  requests_total = requests_total + VALUES(requests_total),
  committed_usd = committed_usd + VALUES(committed_usd),
  input_tokens = input_tokens + VALUES(input_tokens),
  cached_input_tokens = cached_input_tokens + VALUES(cached_input_tokens),
  output_tokens = output_tokens + VALUES(output_tokens),
  cached_output_tokens = cached_output_tokens + VALUES(cached_output_tokens),
  first_token_latency_ms_sum = first_token_latency_ms_sum + VALUES(first_token_latency_ms_sum),
  first_token_samples = first_token_samples + VALUES(first_token_samples),
  decode_latency_ms_sum = decode_latency_ms_sum + VALUES(decode_latency_ms_sum),
  updated_at = CURRENT_TIMESTAMP
`
	if d == DialectSQLite {
		query = `
INSERT INTO usage_rollup_channel_hour(
  bucket_start, upstream_channel_id, requests_total, committed_usd,
  input_tokens, cached_input_tokens, output_tokens, cached_output_tokens,
  first_token_latency_ms_sum, first_token_samples, decode_latency_ms_sum, updated_at
)
SELECT
  STRFTIME('%Y-%m-%d %H:00:00', time),
  upstream_channel_id,
  1,
  CASE WHEN state='` + UsageStateCommitted + `' THEN committed_usd ELSE 0 END,
  COALESCE(input_tokens, 0),
  COALESCE(cached_input_tokens, 0),
  COALESCE(output_tokens, 0),
  COALESCE(cached_output_tokens, 0),
  CASE WHEN first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END,
  CASE WHEN first_token_latency_ms > 0 THEN 1 ELSE 0 END,
  CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END,
  CURRENT_TIMESTAMP
FROM usage_events
WHERE id=? AND upstream_channel_id IS NOT NULL
ON CONFLICT(bucket_start, upstream_channel_id) DO UPDATE SET
  requests_total = requests_total + excluded.requests_total,
  committed_usd = committed_usd + excluded.committed_usd,
  input_tokens = input_tokens + excluded.input_tokens,
  cached_input_tokens = cached_input_tokens + excluded.cached_input_tokens,
  output_tokens = output_tokens + excluded.output_tokens,
  cached_output_tokens = cached_output_tokens + excluded.cached_output_tokens,
  first_token_latency_ms_sum = first_token_latency_ms_sum + excluded.first_token_latency_ms_sum,
  first_token_samples = first_token_samples + excluded.first_token_samples,
  decode_latency_ms_sum = decode_latency_ms_sum + excluded.decode_latency_ms_sum,
  updated_at = CURRENT_TIMESTAMP
`
	}

	if _, err := tx.ExecContext(ctx, query, usageEventID); err != nil {
		if isMissingTableErr(err) {
			return nil
		}
		return fmt.Errorf("写入 usage_rollup_channel_hour 失败: %w", err)
	}
	return nil
}

func upsertUsageRollupUserDay(ctx context.Context, tx *sql.Tx, d Dialect, usageEventID int64) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	query := `
INSERT INTO usage_rollup_user_day(
  day, user_id, requests_total, committed_usd,
  input_tokens, cached_input_tokens, output_tokens, cached_output_tokens, updated_at
)
SELECT
  DATE(time),
  user_id,
  1,
  CASE WHEN state='` + UsageStateCommitted + `' THEN committed_usd ELSE 0 END,
  COALESCE(input_tokens, 0),
  COALESCE(cached_input_tokens, 0),
  COALESCE(output_tokens, 0),
  COALESCE(cached_output_tokens, 0),
  CURRENT_TIMESTAMP
FROM usage_events
WHERE id=?
ON DUPLICATE KEY UPDATE
  requests_total = requests_total + VALUES(requests_total),
  committed_usd = committed_usd + VALUES(committed_usd),
  input_tokens = input_tokens + VALUES(input_tokens),
  cached_input_tokens = cached_input_tokens + VALUES(cached_input_tokens),
  output_tokens = output_tokens + VALUES(output_tokens),
  cached_output_tokens = cached_output_tokens + VALUES(cached_output_tokens),
  updated_at = CURRENT_TIMESTAMP
`
	if d == DialectSQLite {
		query = `
INSERT INTO usage_rollup_user_day(
  day, user_id, requests_total, committed_usd,
  input_tokens, cached_input_tokens, output_tokens, cached_output_tokens, updated_at
)
SELECT
  DATE(time),
  user_id,
  1,
  CASE WHEN state='` + UsageStateCommitted + `' THEN committed_usd ELSE 0 END,
  COALESCE(input_tokens, 0),
  COALESCE(cached_input_tokens, 0),
  COALESCE(output_tokens, 0),
  COALESCE(cached_output_tokens, 0),
  CURRENT_TIMESTAMP
FROM usage_events
WHERE id=?
ON CONFLICT(day, user_id) DO UPDATE SET
  requests_total = requests_total + excluded.requests_total,
  committed_usd = committed_usd + excluded.committed_usd,
  input_tokens = input_tokens + excluded.input_tokens,
  cached_input_tokens = cached_input_tokens + excluded.cached_input_tokens,
  output_tokens = output_tokens + excluded.output_tokens,
  cached_output_tokens = cached_output_tokens + excluded.cached_output_tokens,
  updated_at = CURRENT_TIMESTAMP
`
	}

	if _, err := tx.ExecContext(ctx, query, usageEventID); err != nil {
		if isMissingTableErr(err) {
			return nil
		}
		return fmt.Errorf("写入 usage_rollup_user_day 失败: %w", err)
	}
	return nil
}

func upsertUsageRollupModelDay(ctx context.Context, tx *sql.Tx, d Dialect, usageEventID int64) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	query := `
INSERT INTO usage_rollup_model_day(
  day, model, requests_total, committed_usd,
  input_tokens, cached_input_tokens, output_tokens, cached_output_tokens, updated_at
)
SELECT
  DATE(time),
  COALESCE(model, ''),
  1,
  CASE WHEN state='` + UsageStateCommitted + `' THEN committed_usd ELSE 0 END,
  COALESCE(input_tokens, 0),
  COALESCE(cached_input_tokens, 0),
  COALESCE(output_tokens, 0),
  COALESCE(cached_output_tokens, 0),
  CURRENT_TIMESTAMP
FROM usage_events
WHERE id=?
ON DUPLICATE KEY UPDATE
  requests_total = requests_total + VALUES(requests_total),
  committed_usd = committed_usd + VALUES(committed_usd),
  input_tokens = input_tokens + VALUES(input_tokens),
  cached_input_tokens = cached_input_tokens + VALUES(cached_input_tokens),
  output_tokens = output_tokens + VALUES(output_tokens),
  cached_output_tokens = cached_output_tokens + VALUES(cached_output_tokens),
  updated_at = CURRENT_TIMESTAMP
`
	if d == DialectSQLite {
		query = `
INSERT INTO usage_rollup_model_day(
  day, model, requests_total, committed_usd,
  input_tokens, cached_input_tokens, output_tokens, cached_output_tokens, updated_at
)
SELECT
  DATE(time),
  COALESCE(model, ''),
  1,
  CASE WHEN state='` + UsageStateCommitted + `' THEN committed_usd ELSE 0 END,
  COALESCE(input_tokens, 0),
  COALESCE(cached_input_tokens, 0),
  COALESCE(output_tokens, 0),
  COALESCE(cached_output_tokens, 0),
  CURRENT_TIMESTAMP
FROM usage_events
WHERE id=?
ON CONFLICT(day, model) DO UPDATE SET
  requests_total = requests_total + excluded.requests_total,
  committed_usd = committed_usd + excluded.committed_usd,
  input_tokens = input_tokens + excluded.input_tokens,
  cached_input_tokens = cached_input_tokens + excluded.cached_input_tokens,
  output_tokens = output_tokens + excluded.output_tokens,
  cached_output_tokens = cached_output_tokens + excluded.cached_output_tokens,
  updated_at = CURRENT_TIMESTAMP
`
	}

	if _, err := tx.ExecContext(ctx, query, usageEventID); err != nil {
		if isMissingTableErr(err) {
			return nil
		}
		return fmt.Errorf("写入 usage_rollup_model_day 失败: %w", err)
	}
	return nil
}

type usageRollupStatsRow struct {
	RequestsTotal        int64
	InputTokens          int64
	CachedInputTokens    int64
	OutputTokens         int64
	CachedOutputTokens   int64
	FirstTokenLatencySum int64
	FirstTokenSamples    int64
	DecodeLatencySum     int64
	CommittedUSD         decimal.Decimal
}

func scanUsageRollupStats(row *sql.Row, out *usageRollupStatsRow) error {
	if row == nil || out == nil {
		return errors.New("scan args 为空")
	}
	var (
		reqs                 sql.NullInt64
		inTok                sql.NullInt64
		cachedInTok          sql.NullInt64
		outTok               sql.NullInt64
		cachedOutTok         sql.NullInt64
		firstTokenLatencySum sql.NullInt64
		firstTokenSamples    sql.NullInt64
		decodeLatencySum     sql.NullInt64
		committedUSDNull     decimal.NullDecimal
	)
	if err := row.Scan(&reqs, &inTok, &cachedInTok, &outTok, &cachedOutTok, &firstTokenLatencySum, &firstTokenSamples, &decodeLatencySum, &committedUSDNull); err != nil {
		return err
	}
	if reqs.Valid {
		out.RequestsTotal = reqs.Int64
	}
	if inTok.Valid {
		out.InputTokens = inTok.Int64
	}
	if cachedInTok.Valid {
		out.CachedInputTokens = cachedInTok.Int64
	}
	if outTok.Valid {
		out.OutputTokens = outTok.Int64
	}
	if cachedOutTok.Valid {
		out.CachedOutputTokens = cachedOutTok.Int64
	}
	if firstTokenLatencySum.Valid {
		out.FirstTokenLatencySum = firstTokenLatencySum.Int64
	}
	if firstTokenSamples.Valid {
		out.FirstTokenSamples = firstTokenSamples.Int64
	}
	if decodeLatencySum.Valid {
		out.DecodeLatencySum = decodeLatencySum.Int64
	}
	if committedUSDNull.Valid {
		out.CommittedUSD = committedUSDNull.Decimal.Truncate(USDScale)
	}
	return nil
}
