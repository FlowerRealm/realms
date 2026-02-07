package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteManagedModelGroupNameColumn(db *sql.DB) error {
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

	if _, ok := cols["group_name"]; !ok {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE managed_models ADD COLUMN group_name TEXT NOT NULL DEFAULT 'default'`); err != nil {
			return fmt.Errorf("添加 managed_models 列 group_name 失败: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE managed_models SET group_name='default' WHERE TRIM(IFNULL(group_name, ''))=''`); err != nil {
		return fmt.Errorf("回填 managed_models.group_name 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_managed_models_status_group ON managed_models(status, group_name)`); err != nil {
		return fmt.Errorf("创建 managed_models(status, group_name) 索引失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}
