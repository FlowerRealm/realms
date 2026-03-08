package store

import (
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteAnthropicCredentialsTable(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS anthropic_credentials (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  endpoint_id INTEGER NOT NULL,
  name TEXT NULL,
  api_key_enc BLOB NOT NULL,
  api_key_hint TEXT NULL,
  status INTEGER NOT NULL DEFAULT 1,
  last_used_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)
`); err != nil {
		return fmt.Errorf("创建 anthropic_credentials 表失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_anthropic_credentials_endpoint_id ON anthropic_credentials (endpoint_id)`); err != nil {
		return fmt.Errorf("创建 anthropic_credentials endpoint_id 索引失败: %w", err)
	}
	return nil
}
