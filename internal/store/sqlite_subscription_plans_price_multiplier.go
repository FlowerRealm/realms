package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteSubscriptionPlansPriceMultiplierColumn(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite schema 修补事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(subscription_plans)`)
	if err != nil {
		return fmt.Errorf("查询 subscription_plans 列信息失败: %w", err)
	}
	defer rows.Close()

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
			return fmt.Errorf("扫描 subscription_plans 列信息失败: %w", err)
		}
		if name != "" {
			cols[name] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 subscription_plans 列信息失败: %w", err)
	}

	if _, ok := cols["price_multiplier"]; !ok {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE subscription_plans ADD COLUMN price_multiplier DECIMAL(20,6) NOT NULL DEFAULT 1.000000`); err != nil {
			return fmt.Errorf("添加 subscription_plans 列 price_multiplier 失败: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE subscription_plans SET price_multiplier=1.000000 WHERE price_multiplier IS NULL OR price_multiplier<=0`); err != nil {
		return fmt.Errorf("回填 subscription_plans.price_multiplier 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}
