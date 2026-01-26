package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteUsageEventDetailsTable(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite schema 修补事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS usage_event_details (
  usage_event_id INTEGER PRIMARY KEY,
  downstream_request_body TEXT NULL,
  upstream_request_body TEXT NULL,
  upstream_response_body TEXT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_usage_event_details_updated_at ON usage_event_details (updated_at);
`)
	if err != nil {
		return fmt.Errorf("创建 usage_event_details 表失败: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(usage_event_details)`)
	if err != nil {
		return fmt.Errorf("查询 usage_event_details 列信息失败: %w", err)
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
			return fmt.Errorf("扫描 usage_event_details 列信息失败: %w", err)
		}
		if name != "" {
			cols[name] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 usage_event_details 列信息失败: %w", err)
	}

	if _, ok := cols["downstream_request_body"]; !ok {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE usage_event_details ADD COLUMN downstream_request_body TEXT NULL"); err != nil {
			return fmt.Errorf("添加 usage_event_details 列 downstream_request_body 失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}
