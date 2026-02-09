package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteUsageEventDetailsRemoved(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite schema 修补事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DROP TABLE IF EXISTS usage_event_details"); err != nil {
		return fmt.Errorf("删除 usage_event_details 表失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}
