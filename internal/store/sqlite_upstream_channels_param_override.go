package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteUpstreamChannelParamOverrideColumn(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite schema 修补事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(upstream_channels)`)
	if err != nil {
		return fmt.Errorf("查询 upstream_channels 列信息失败: %w", err)
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
			return fmt.Errorf("扫描 upstream_channels 列信息失败: %w", err)
		}
		if name != "" {
			cols[name] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 upstream_channels 列信息失败: %w", err)
	}

	type addCol struct {
		name string
		ddl  string
	}
	need := []addCol{
		{name: "param_override", ddl: `ALTER TABLE upstream_channels ADD COLUMN param_override TEXT NULL`},
		{name: "header_override", ddl: `ALTER TABLE upstream_channels ADD COLUMN header_override TEXT NULL`},
		{name: "status_code_mapping", ddl: `ALTER TABLE upstream_channels ADD COLUMN status_code_mapping TEXT NULL`},
	}
	for _, c := range need {
		if _, ok := cols[c.name]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, c.ddl); err != nil {
			return fmt.Errorf("添加 upstream_channels 列 %s 失败: %w", c.name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}
