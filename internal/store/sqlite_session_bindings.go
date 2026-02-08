package store

import (
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteSessionBindingsTable(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS session_bindings (
  user_id INTEGER NOT NULL,
  route_key_hash TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  expires_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, route_key_hash)
)
`); err != nil {
		return fmt.Errorf("创建 session_bindings 表失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_session_bindings_expires_at ON session_bindings (expires_at)`); err != nil {
		return fmt.Errorf("创建 session_bindings expires_at 索引失败: %w", err)
	}
	return nil
}
