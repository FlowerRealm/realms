package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
)

//go:embed schema_sqlite.sql
var sqliteSchemaFS embed.FS

func EnsureSQLiteSchema(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}
	var v int
	err := db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name='users' LIMIT 1`).Scan(&v)
	if err == nil && v == 1 {
		if err := ensureSQLiteUsersMainGroupColumn(db); err != nil {
			return err
		}
		if err := ensureSQLiteMainGroupsTables(db); err != nil {
			return err
		}
		if err := ensureSQLiteTokenGroupsTable(db); err != nil {
			return err
		}
		if err := ensureSQLiteUpstreamChannelRequestPolicyColumns(db); err != nil {
			return err
		}
		if err := ensureSQLiteUpstreamChannelNewAPISettingsColumns(db); err != nil {
			return err
		}
		if err := ensureSQLiteUpstreamChannelParamOverrideColumn(db); err != nil {
			return err
		}
		if err := ensureSQLiteUpstreamChannelBodyFilterColumns(db); err != nil {
			return err
		}
		if err := ensureSQLiteUsageEventDetailsRemoved(db); err != nil {
			return err
		}
		if err := ensureSQLiteUsageEventsFirstTokenLatencyColumn(db); err != nil {
			return err
		}
		if err := ensureSQLiteUsageEventsPriceMultiplierColumns(db); err != nil {
			return err
		}
		if err := ensureSQLiteSubscriptionPlansPriceMultiplierColumn(db); err != nil {
			return err
		}
		if err := ensureSQLiteUserTokensPlainColumn(db); err != nil {
			return err
		}
		if err := ensureSQLiteUsersUsernameRules(db); err != nil {
			return err
		}
		if err := ensureSQLiteManagedModelGroupNameColumn(db); err != nil {
			return err
		}
		if err := ensureSQLiteSessionBindingsTable(db); err != nil {
			return err
		}
		if err := ensureSQLiteOpenAIObjectRefsTable(db); err != nil {
			return err
		}
		if err := ensureSQLiteChannelGroupPointers(db); err != nil {
			return err
		}
		return ensureSQLiteChannelGroupMembers(db)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("检查 SQLite schema 状态失败: %w", err)
	}

	b, err := sqliteSchemaFS.ReadFile("schema_sqlite.sql")
	if err != nil {
		return fmt.Errorf("读取 schema_sqlite.sql 失败: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始 schema 初始化事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmts := splitSQLStatements(string(b))
	for i, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("执行 SQLite schema 初始化失败 (stmt %d/%d): %w", i+1, len(stmts), err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 初始化失败: %w", err)
	}
	if err := ensureSQLiteUsersUsernameRules(db); err != nil {
		return err
	}
	if err := ensureSQLiteUsersMainGroupColumn(db); err != nil {
		return err
	}
	if err := ensureSQLiteMainGroupsTables(db); err != nil {
		return err
	}
	if err := ensureSQLiteTokenGroupsTable(db); err != nil {
		return err
	}
	if err := ensureSQLiteManagedModelGroupNameColumn(db); err != nil {
		return err
	}
	if err := ensureSQLiteUsageEventsPriceMultiplierColumns(db); err != nil {
		return err
	}
	if err := ensureSQLiteSubscriptionPlansPriceMultiplierColumn(db); err != nil {
		return err
	}
	if err := ensureSQLiteSessionBindingsTable(db); err != nil {
		return err
	}
	if err := ensureSQLiteOpenAIObjectRefsTable(db); err != nil {
		return err
	}
	if err := ensureSQLiteChannelGroupPointers(db); err != nil {
		return err
	}
	return ensureSQLiteChannelGroupMembers(db)
}
