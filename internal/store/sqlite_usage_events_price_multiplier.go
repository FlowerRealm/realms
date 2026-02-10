package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteUsageEventsPriceMultiplierColumns(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite schema 修补事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(usage_events)`)
	if err != nil {
		return fmt.Errorf("查询 usage_events 列信息失败: %w", err)
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
			return fmt.Errorf("扫描 usage_events 列信息失败: %w", err)
		}
		if name != "" {
			cols[name] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 usage_events 列信息失败: %w", err)
	}

	addCol := func(name string, ddl string) error {
		if _, ok := cols[name]; ok {
			return nil
		}
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return err
		}
		return nil
	}

	if err := addCol("price_multiplier", `ALTER TABLE usage_events ADD COLUMN price_multiplier DECIMAL(20,6) NOT NULL DEFAULT 1.000000`); err != nil {
		return fmt.Errorf("添加 usage_events 列 price_multiplier 失败: %w", err)
	}
	if err := addCol("price_multiplier_group", `ALTER TABLE usage_events ADD COLUMN price_multiplier_group DECIMAL(20,6) NOT NULL DEFAULT 1.000000`); err != nil {
		return fmt.Errorf("添加 usage_events 列 price_multiplier_group 失败: %w", err)
	}
	if err := addCol("price_multiplier_payment", `ALTER TABLE usage_events ADD COLUMN price_multiplier_payment DECIMAL(20,6) NOT NULL DEFAULT 1.000000`); err != nil {
		return fmt.Errorf("添加 usage_events 列 price_multiplier_payment 失败: %w", err)
	}
	if err := addCol("price_multiplier_group_name", `ALTER TABLE usage_events ADD COLUMN price_multiplier_group_name TEXT NULL`); err != nil {
		return fmt.Errorf("添加 usage_events 列 price_multiplier_group_name 失败: %w", err)
	}

	// backfill
	if _, err := tx.ExecContext(ctx, `UPDATE usage_events SET price_multiplier=1.000000 WHERE price_multiplier IS NULL OR price_multiplier<=0`); err != nil {
		return fmt.Errorf("回填 usage_events.price_multiplier 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE usage_events SET price_multiplier_group=1.000000 WHERE price_multiplier_group IS NULL OR price_multiplier_group<=0`); err != nil {
		return fmt.Errorf("回填 usage_events.price_multiplier_group 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE usage_events SET price_multiplier_payment=1.000000 WHERE price_multiplier_payment IS NULL OR price_multiplier_payment<=0`); err != nil {
		return fmt.Errorf("回填 usage_events.price_multiplier_payment 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}
