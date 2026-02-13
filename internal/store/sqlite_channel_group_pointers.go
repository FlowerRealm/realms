package store

import (
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteChannelGroupPointers(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS channel_group_pointers (
  group_id INTEGER NOT NULL,
  channel_id INTEGER NOT NULL DEFAULT 0,
  pinned INTEGER NOT NULL DEFAULT 0,
  moved_at_unix_ms INTEGER NOT NULL DEFAULT 0,
  reason TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (group_id)
)
`); err != nil {
		return fmt.Errorf("创建 channel_group_pointers 表失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_channel_group_pointers_channel_id ON channel_group_pointers (channel_id)`); err != nil {
		return fmt.Errorf("创建 channel_group_pointers channel_id 索引失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_channel_group_pointers_updated_at ON channel_group_pointers (updated_at)`); err != nil {
		return fmt.Errorf("创建 channel_group_pointers updated_at 索引失败: %w", err)
	}
	return nil
}

