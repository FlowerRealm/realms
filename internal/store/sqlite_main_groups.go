package store

import (
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteMainGroupsTables(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS main_groups (
  name TEXT PRIMARY KEY,
  description TEXT NULL,
  status INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
)
`); err != nil {
		return fmt.Errorf("创建 main_groups 表失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_main_groups_status ON main_groups (status)`); err != nil {
		return fmt.Errorf("创建 main_groups status 索引失败: %w", err)
	}

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS main_group_subgroups (
  main_group TEXT NOT NULL,
  subgroup TEXT NOT NULL,
  priority INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (main_group, subgroup)
)
`); err != nil {
		return fmt.Errorf("创建 main_group_subgroups 表失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_main_group_subgroups_main_group ON main_group_subgroups (main_group)`); err != nil {
		return fmt.Errorf("创建 main_group_subgroups main_group 索引失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_main_group_subgroups_subgroup ON main_group_subgroups (subgroup)`); err != nil {
		return fmt.Errorf("创建 main_group_subgroups subgroup 索引失败: %w", err)
	}

	return nil
}
