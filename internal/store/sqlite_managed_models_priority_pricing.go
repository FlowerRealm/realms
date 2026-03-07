package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteManagedModelPriorityPricingColumns(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite schema 修补事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(managed_models)`)
	if err != nil {
		return fmt.Errorf("查询 managed_models 列信息失败: %w", err)
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
			return fmt.Errorf("扫描 managed_models 列信息失败: %w", err)
		}
		if name != "" {
			cols[name] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 managed_models 列信息失败: %w", err)
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

	if err := addCol("priority_pricing_enabled", `ALTER TABLE managed_models ADD COLUMN priority_pricing_enabled INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("添加 managed_models 列 priority_pricing_enabled 失败: %w", err)
	}
	if err := addCol("priority_input_usd_per_1m", `ALTER TABLE managed_models ADD COLUMN priority_input_usd_per_1m DECIMAL(20,6) NULL`); err != nil {
		return fmt.Errorf("添加 managed_models 列 priority_input_usd_per_1m 失败: %w", err)
	}
	if err := addCol("priority_output_usd_per_1m", `ALTER TABLE managed_models ADD COLUMN priority_output_usd_per_1m DECIMAL(20,6) NULL`); err != nil {
		return fmt.Errorf("添加 managed_models 列 priority_output_usd_per_1m 失败: %w", err)
	}
	if err := addCol("priority_cache_input_usd_per_1m", `ALTER TABLE managed_models ADD COLUMN priority_cache_input_usd_per_1m DECIMAL(20,6) NULL`); err != nil {
		return fmt.Errorf("添加 managed_models 列 priority_cache_input_usd_per_1m 失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE managed_models SET priority_pricing_enabled=0 WHERE priority_pricing_enabled IS NULL`); err != nil {
		return fmt.Errorf("回填 managed_models.priority_pricing_enabled 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}
