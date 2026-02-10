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

	// seed default main group + mapping (default → default).
	if _, err := db.Exec(`
INSERT INTO main_groups(name, description, status, created_at, updated_at)
SELECT 'default', '默认用户分组', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM main_groups WHERE name='default' LIMIT 1)
`); err != nil {
		return fmt.Errorf("初始化 main_groups(default) 失败: %w", err)
	}
	if _, err := db.Exec(`
INSERT OR IGNORE INTO main_group_subgroups(main_group, subgroup, priority, created_at, updated_at)
VALUES('default', 'default', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`); err != nil {
		return fmt.Errorf("初始化 main_group_subgroups(default→default) 失败: %w", err)
	}

	return nil
}
