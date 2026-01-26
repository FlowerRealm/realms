package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

var ErrInsufficientBalance = errors.New("余额不足")

func (s *Store) GetUserBalanceUSD(ctx context.Context, userID int64) (decimal.Decimal, error) {
	var v decimal.Decimal
	err := s.db.QueryRowContext(ctx, `SELECT usd FROM user_balances WHERE user_id=?`, userID).Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return decimal.Zero, nil
		}
		return decimal.Zero, fmt.Errorf("查询 user_balances 失败: %w", err)
	}
	return v, nil
}

func (s *Store) GetUserBalancesUSD(ctx context.Context, userIDs []int64) (map[int64]decimal.Decimal, error) {
	out := make(map[int64]decimal.Decimal, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}

	var b strings.Builder
	b.WriteString("SELECT user_id, usd FROM user_balances WHERE user_id IN (")
	args := make([]any, 0, len(userIDs))
	for i, id := range userIDs {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("?")
		args = append(args, id)
		out[id] = decimal.Zero
	}
	b.WriteString(")")

	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("查询 user_balances 失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID int64
		var usd decimal.Decimal
		if err := rows.Scan(&userID, &usd); err != nil {
			return nil, fmt.Errorf("扫描 user_balances 失败: %w", err)
		}
		out[userID] = usd.Truncate(USDScale)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 user_balances 失败: %w", err)
	}
	return out, nil
}

func (s *Store) AddUserBalanceUSD(ctx context.Context, userID int64, deltaUSD decimal.Decimal) (decimal.Decimal, error) {
	if userID <= 0 {
		return decimal.Zero, errors.New("user_id 不能为空")
	}
	deltaUSD = deltaUSD.Truncate(USDScale)
	if deltaUSD.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, errors.New("delta_usd 不合法")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return decimal.Zero, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmtInitBalance := fmt.Sprintf(`
%s INTO user_balances(user_id, usd, created_at, updated_at)
VALUES(?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, insertIgnoreVerb(s.dialect))
	if _, err := tx.ExecContext(ctx, stmtInitBalance, userID); err != nil {
		return decimal.Zero, fmt.Errorf("初始化余额失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, userBalancesAddSQL(s.dialect), deltaUSD, userID); err != nil {
		return decimal.Zero, fmt.Errorf("入账失败: %w", err)
	}

	var newBal decimal.Decimal
	if err := tx.QueryRowContext(ctx, `SELECT usd FROM user_balances WHERE user_id=?`, userID).Scan(&newBal); err != nil {
		return decimal.Zero, fmt.Errorf("查询余额失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return decimal.Zero, fmt.Errorf("提交事务失败: %w", err)
	}
	return newBal.Truncate(USDScale), nil
}

func userBalancesAddSQL(d Dialect) string {
	if d == DialectSQLite {
		return `
UPDATE user_balances
SET usd=ROUND(usd+?, 6), updated_at=CURRENT_TIMESTAMP
WHERE user_id=?
`
	}
	return `
UPDATE user_balances
SET usd=usd+?, updated_at=CURRENT_TIMESTAMP
WHERE user_id=?
`
}

func userBalancesSubSQL(d Dialect) string {
	if d == DialectSQLite {
		return `
UPDATE user_balances
SET usd=ROUND(usd-?, 6), updated_at=CURRENT_TIMESTAMP
WHERE user_id=?
`
	}
	return `
UPDATE user_balances
SET usd=usd-?, updated_at=CURRENT_TIMESTAMP
WHERE user_id=?
`
}
