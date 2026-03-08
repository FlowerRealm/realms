package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteChannelModelsReferenceManagedModels(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite channel_models 清理事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tables := make(map[string]struct{}, 2)
	rows, err := tx.QueryContext(ctx, `
SELECT name
FROM sqlite_master
WHERE type='table' AND name IN ('managed_models', 'channel_models')
`)
	if err != nil {
		return fmt.Errorf("查询 SQLite 表信息失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("扫描 SQLite 表信息失败: %w", err)
		}
		if name != "" {
			tables[name] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 SQLite 表信息失败: %w", err)
	}
	if _, ok := tables["managed_models"]; !ok {
		return nil
	}
	if _, ok := tables["channel_models"]; !ok {
		return nil
	}

	if _, err := tx.ExecContext(ctx, `
DELETE FROM channel_models
WHERE NOT EXISTS (
  SELECT 1 FROM managed_models m WHERE m.public_id = channel_models.public_id
)
`); err != nil {
		return fmt.Errorf("清理 SQLite 脏 channel_models 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite channel_models 清理事务失败: %w", err)
	}
	return nil
}
