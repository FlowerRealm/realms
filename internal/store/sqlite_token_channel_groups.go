package store

// sqlite_token_channel_groups.go: SQLite schema ensure/migrate for token_channel_groups.

import (
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteTokenChannelGroupsTable(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS token_channel_groups (
  token_id INTEGER NOT NULL,
  channel_group_name TEXT NOT NULL,
  priority INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (token_id, channel_group_name)
)
`); err != nil {
		return fmt.Errorf("创建 token_channel_groups 表失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_token_channel_groups_token_id ON token_channel_groups (token_id)`); err != nil {
		return fmt.Errorf("创建 token_channel_groups token_id 索引失败: %w", err)
	}

	// migrate legacy token_groups -> token_channel_groups (best-effort).
	var hasLegacyTokenGroups int
	if err := db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name='token_groups' LIMIT 1`).Scan(&hasLegacyTokenGroups); err == nil && hasLegacyTokenGroups == 1 {
		if _, err := db.Exec(`
INSERT OR IGNORE INTO token_channel_groups(token_id, channel_group_name, priority, created_at, updated_at)
SELECT token_id, group_name, priority, created_at, updated_at
FROM token_groups
`); err != nil {
			return fmt.Errorf("迁移 token_groups -> token_channel_groups 失败: %w", err)
		}
		if _, err := db.Exec(`DROP TABLE IF EXISTS token_groups`); err != nil {
			return fmt.Errorf("清理旧 token_groups 表失败: %w", err)
		}
	}

	// best-effort backfill: 将旧 user_groups 迁移到 token 维度（仅对活跃 token）。
	// 注意：新 SQLite schema 已不再创建 user_groups；此处仅在旧表存在时尝试回填。
	var hasLegacyUserGroups int
	if err := db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name='user_groups' LIMIT 1`).Scan(&hasLegacyUserGroups); err == nil && hasLegacyUserGroups == 1 {
		// 说明：这里不依赖 main_group_subgroups，以便升级后旧 Token 仍可工作；后续由管理面重新收敛权限范围。
		if _, err := db.Exec(`
INSERT OR IGNORE INTO token_channel_groups(token_id, channel_group_name, priority, created_at, updated_at)
SELECT t.id, ug.group_name, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM user_tokens t
JOIN user_groups ug ON ug.user_id=t.user_id
WHERE t.status=1
`); err != nil {
			return fmt.Errorf("回填 token_channel_groups(从 user_groups) 失败: %w", err)
		}
	}

	return nil
}
