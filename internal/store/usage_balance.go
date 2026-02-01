package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

func (s *Store) ReserveUsageAndDebitBalance(ctx context.Context, in ReserveUsageInput) (int64, error) {
	if in.UserID <= 0 {
		return 0, errors.New("user_id 不能为空")
	}
	if in.TokenID <= 0 {
		return 0, errors.New("token_id 不能为空")
	}
	if in.SubscriptionID != nil {
		return 0, errors.New("按量计费预留不支持 subscription_id")
	}
	if in.ReservedUSD.LessThanOrEqual(decimal.Zero) {
		return 0, errors.New("预留金额不合法")
	}
	if in.ReserveExpiresAt.IsZero() {
		return 0, errors.New("reserve_expires_at 不能为空")
	}

	reservedUSD := in.ReservedUSD.Truncate(USDScale)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmtInitBalance := fmt.Sprintf(`
%s INTO user_balances(user_id, usd, created_at, updated_at)
VALUES(?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, insertIgnoreVerb(s.dialect))
	if _, err := tx.ExecContext(ctx, stmtInitBalance, in.UserID); err != nil {
		return 0, fmt.Errorf("初始化余额失败: %w", err)
	}

	var bal decimal.Decimal
	qBalance := "SELECT usd FROM user_balances WHERE user_id=?" + forUpdateClause(s.dialect)
	if err := tx.QueryRowContext(ctx, qBalance, in.UserID).Scan(&bal); err != nil {
		return 0, fmt.Errorf("查询余额失败: %w", err)
	}
	if bal.LessThan(reservedUSD) {
		return 0, ErrInsufficientBalance
	}
	if _, err := tx.ExecContext(ctx, userBalancesSubSQL(s.dialect), reservedUSD, in.UserID); err != nil {
		return 0, fmt.Errorf("扣减余额失败: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
INSERT INTO usage_events(
  time, request_id, user_id, subscription_id, token_id, state, model,
  reserved_usd, committed_usd, reserve_expires_at, created_at, updated_at
) VALUES(
  CURRENT_TIMESTAMP, ?, ?, NULL, ?, ?, ?,
  ?, 0, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
)
`, in.RequestID, in.UserID, in.TokenID, UsageStateReserved, in.Model, reservedUSD, in.ReserveExpiresAt)
	if err != nil {
		return 0, fmt.Errorf("写入 usage_events(reserved) 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取 usage_event id 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交事务失败: %w", err)
	}
	return id, nil
}

func (s *Store) CommitUsageAndRefundBalance(ctx context.Context, in CommitUsageInput) error {
	if in.UsageEventID <= 0 {
		return nil
	}
	if in.CommittedUSD.IsNegative() {
		return errors.New("committed_usd 不合法")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var userID int64
	var state string
	var reserved decimal.Decimal
	var subID sql.NullInt64
	qUsage := `
SELECT user_id, subscription_id, state, reserved_usd
FROM usage_events
WHERE id=?
` + forUpdateClause(s.dialect)
	if err := tx.QueryRowContext(ctx, qUsage, in.UsageEventID).Scan(&userID, &subID, &state, &reserved); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("查询 usage_event 失败: %w", err)
	}
	if subID.Valid {
		return errors.New("该 usage_event 不属于按量计费")
	}
	if state != UsageStateReserved {
		return nil
	}

	committed := in.CommittedUSD.Truncate(USDScale)
	if committed.Equal(decimal.Zero) {
		committed = reserved
	}
	committedEffective := committed
	refund := decimal.Zero
	if committed.LessThan(reserved) {
		refund = reserved.Sub(committed)
	} else if committed.GreaterThan(reserved) {
		// 预留不足：尝试补扣差额（余额不足时最多扣到 0）。
		extra := committed.Sub(reserved)

		stmtInitBalance := fmt.Sprintf(`
%s INTO user_balances(user_id, usd, created_at, updated_at)
VALUES(?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, insertIgnoreVerb(s.dialect))
		if _, err := tx.ExecContext(ctx, stmtInitBalance, userID); err != nil {
			return fmt.Errorf("初始化余额失败: %w", err)
		}

		var bal decimal.Decimal
		qBalance := "SELECT usd FROM user_balances WHERE user_id=?" + forUpdateClause(s.dialect)
		if err := tx.QueryRowContext(ctx, qBalance, userID).Scan(&bal); err != nil {
			return fmt.Errorf("查询余额失败: %w", err)
		}

		debit := extra
		if bal.LessThan(debit) {
			debit = bal
		}
		debit = debit.Truncate(USDScale)
		if debit.GreaterThan(decimal.Zero) {
			if _, err := tx.ExecContext(ctx, userBalancesSubSQL(s.dialect), debit, userID); err != nil {
				return fmt.Errorf("补扣余额失败: %w", err)
			}
		}

		if debit.LessThan(extra) {
			committedEffective = reserved.Add(debit).Truncate(USDScale)
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE usage_events
SET state=?, upstream_channel_id=?, input_tokens=?, cached_input_tokens=?, output_tokens=?, cached_output_tokens=?, committed_usd=?, updated_at=CURRENT_TIMESTAMP
WHERE id=? AND state=?
`, UsageStateCommitted, in.UpstreamChannelID, in.InputTokens, in.CachedInputTokens, in.OutputTokens, in.CachedOutputTokens, committedEffective, in.UsageEventID, UsageStateReserved); err != nil {
		return fmt.Errorf("结算 usage_event 失败: %w", err)
	}

	if refund.GreaterThan(decimal.Zero) {
		if _, err := tx.ExecContext(ctx, userBalancesAddSQL(s.dialect), refund, userID); err != nil {
			return fmt.Errorf("返还余额失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) VoidUsageAndRefundBalance(ctx context.Context, usageEventID int64) error {
	if usageEventID <= 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var userID int64
	var state string
	var reserved decimal.Decimal
	var subID sql.NullInt64
	qUsage := `
SELECT user_id, subscription_id, state, reserved_usd
FROM usage_events
WHERE id=?
` + forUpdateClause(s.dialect)
	if err := tx.QueryRowContext(ctx, qUsage, usageEventID).Scan(&userID, &subID, &state, &reserved); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("查询 usage_event 失败: %w", err)
	}
	if subID.Valid {
		// 非按量计费：沿用旧逻辑（仅作废用量事件）
		if _, err := tx.ExecContext(ctx, `
UPDATE usage_events
SET state=?, committed_usd=0, updated_at=CURRENT_TIMESTAMP
WHERE id=? AND state=?
`, UsageStateVoid, usageEventID, UsageStateReserved); err != nil {
			return fmt.Errorf("作废 usage_event 失败: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交事务失败: %w", err)
		}
		return nil
	}
	if state != UsageStateReserved {
		return nil
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE usage_events
SET state=?, committed_usd=0, updated_at=CURRENT_TIMESTAMP
WHERE id=? AND state=?
	`, UsageStateVoid, usageEventID, UsageStateReserved); err != nil {
		return fmt.Errorf("作废 usage_event 失败: %w", err)
	}
	if reserved.GreaterThan(decimal.Zero) {
		if _, err := tx.ExecContext(ctx, userBalancesAddSQL(s.dialect), reserved, userID); err != nil {
			return fmt.Errorf("返还余额失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) expireReservedUsageRefundBalances(ctx context.Context, tx *sql.Tx, now time.Time) (int64, error) {
	// 仅处理按量计费（subscription_id IS NULL）订单的预留返还。
	type row struct {
		id     int64
		userID int64
		amt    decimal.Decimal
	}

	q := `
SELECT id, user_id, reserved_usd
FROM usage_events
WHERE state=? AND reserve_expires_at < ? AND subscription_id IS NULL
` + forUpdateClause(s.dialect)
	rows, err := tx.QueryContext(ctx, q, UsageStateReserved, now)
	if err != nil {
		return 0, fmt.Errorf("查询过期 usage_events 失败: %w", err)
	}
	defer rows.Close()

	var ids []int64
	refundByUser := make(map[int64]decimal.Decimal)
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.userID, &r.amt); err != nil {
			return 0, fmt.Errorf("扫描过期 usage_events 失败: %w", err)
		}
		ids = append(ids, r.id)
		amt := r.amt.Truncate(USDScale)
		prev, ok := refundByUser[r.userID]
		if !ok {
			prev = decimal.Zero
		}
		refundByUser[r.userID] = prev.Add(amt)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("遍历过期 usage_events 失败: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}

	// 标记为 expired
	var b strings.Builder
	b.WriteString("UPDATE usage_events SET state=?, committed_usd=0, updated_at=CURRENT_TIMESTAMP WHERE id IN (")
	args := make([]any, 0, len(ids)+1)
	args = append(args, UsageStateExpired)
	for i, id := range ids {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("?")
		args = append(args, id)
	}
	b.WriteString(")")
	if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
		return 0, fmt.Errorf("过期清理 usage_events 失败: %w", err)
	}

	// 返还余额（逐用户聚合写入）
	for userID, refundUSD := range refundByUser {
		refundUSD = refundUSD.Truncate(USDScale)
		if refundUSD.LessThanOrEqual(decimal.Zero) {
			continue
		}
		stmtInitBalance := fmt.Sprintf(`
%s INTO user_balances(user_id, usd, created_at, updated_at)
VALUES(?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, insertIgnoreVerb(s.dialect))
		if _, err := tx.ExecContext(ctx, stmtInitBalance, userID); err != nil {
			return 0, fmt.Errorf("初始化余额失败: %w", err)
		}
		if _, err := tx.ExecContext(ctx, userBalancesAddSQL(s.dialect), refundUSD, userID); err != nil {
			return 0, fmt.Errorf("返还余额失败: %w", err)
		}
	}
	return int64(len(ids)), nil
}
