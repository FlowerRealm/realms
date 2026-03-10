package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteUsageEventsModelCheckColumns(db *sql.DB) error {
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

	if _, ok := cols["forwarded_model"]; !ok {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE usage_events ADD COLUMN forwarded_model TEXT NULL`); err != nil {
			return fmt.Errorf("添加 usage_events 列 forwarded_model 失败: %w", err)
		}
	}
	if _, ok := cols["upstream_response_model"]; !ok {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE usage_events ADD COLUMN upstream_response_model TEXT NULL`); err != nil {
			return fmt.Errorf("添加 usage_events 列 upstream_response_model 失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}
