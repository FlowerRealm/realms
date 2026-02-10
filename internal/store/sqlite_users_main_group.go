package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteUsersMainGroupColumn(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite schema 修补事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(users)`)
	if err != nil {
		return fmt.Errorf("查询 users 列信息失败: %w", err)
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
			return fmt.Errorf("扫描 users 列信息失败: %w", err)
		}
		if name != "" {
			cols[name] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 users 列信息失败: %w", err)
	}

	if _, ok := cols["main_group"]; !ok {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE users ADD COLUMN main_group TEXT NOT NULL DEFAULT 'default'`); err != nil {
			return fmt.Errorf("添加 users 列 main_group 失败: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET main_group='default' WHERE TRIM(IFNULL(main_group, ''))=''`); err != nil {
		return fmt.Errorf("回填 users.main_group 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}

