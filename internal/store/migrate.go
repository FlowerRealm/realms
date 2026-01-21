// Package store 实现内置 SQL 迁移，保证单体部署时可自举初始化。
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func ApplyMigrations(db *sql.DB) error {
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
  version VARCHAR(255) PRIMARY KEY,
  applied_at DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`); err != nil {
		return fmt.Errorf("创建 schema_migrations: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("读取 migrations 目录: %w", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, file := range files {
		applied, err := isMigrationApplied(db, file)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		b, err := migrationsFS.ReadFile("migrations/" + file)
		if err != nil {
			return fmt.Errorf("读取迁移 %s: %w", file, err)
		}
		if err := applyMigration(db, file, string(b)); err != nil {
			return err
		}
	}
	return nil
}

func isMigrationApplied(db *sql.DB, version string) (bool, error) {
	var v string
	err := db.QueryRow(`SELECT version FROM schema_migrations WHERE version=?`, version).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("查询迁移状态: %w", err)
	}
	return true, nil
}

func applyMigration(db *sql.DB, version, sqlText string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始迁移事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmts := splitSQLStatements(sqlText)
	for i, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("执行迁移 %s (stmt %d/%d): %w", version, i+1, len(stmts), err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES(?, CURRENT_TIMESTAMP)`, version); err != nil {
		return fmt.Errorf("记录迁移 %s: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交迁移 %s: %w", version, err)
	}
	return nil
}

func splitSQLStatements(sqlText string) []string {
	parts := strings.Split(sqlText, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		stmt := strings.TrimSpace(p)
		if stmt == "" {
			continue
		}
		out = append(out, stmt)
	}
	return out
}
