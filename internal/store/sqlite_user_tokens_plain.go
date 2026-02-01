package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteUserTokensPlainColumn(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite schema 修补事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(user_tokens)`)
	if err != nil {
		return fmt.Errorf("查询 user_tokens 列信息失败: %w", err)
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
			return fmt.Errorf("扫描 user_tokens 列信息失败: %w", err)
		}
		if name != "" {
			cols[name] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 user_tokens 列信息失败: %w", err)
	}

	if _, ok := cols["token_plain"]; !ok {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE user_tokens ADD COLUMN token_plain TEXT NULL`); err != nil {
			return fmt.Errorf("添加 user_tokens 列 token_plain 失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}

