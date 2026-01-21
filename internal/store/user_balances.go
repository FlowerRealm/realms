package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
