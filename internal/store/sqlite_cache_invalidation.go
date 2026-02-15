package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteCacheInvalidationTable(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite schema 修补事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS cache_invalidation (
  cache_key TEXT PRIMARY KEY,
  version INTEGER NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)
`); err != nil {
		return fmt.Errorf("创建 cache_invalidation 失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}

