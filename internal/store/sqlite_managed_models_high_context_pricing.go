package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteManagedModelHighContextPricingColumn(db *sql.DB) error {
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

	hasColumn := false
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
		if name == "high_context_pricing_json" {
			hasColumn = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 managed_models 列信息失败: %w", err)
	}
	if !hasColumn {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE managed_models ADD COLUMN high_context_pricing_json TEXT NULL`); err != nil {
			return fmt.Errorf("添加 managed_models 列 high_context_pricing_json 失败: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}
