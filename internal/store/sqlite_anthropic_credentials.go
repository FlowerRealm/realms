package store

import (
	"database/sql"
	"fmt"
)

func ensureSQLiteAnthropicCredentialsTable(db *sql.DB) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始 SQLite schema 修补事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS anthropic_credentials (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  endpoint_id INTEGER NOT NULL,
  name TEXT NULL,
  api_key_enc BLOB NOT NULL,
  api_key_hint TEXT NULL,
  status INTEGER NOT NULL DEFAULT 1,
  last_used_at DATETIME NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
`); err != nil {
		return fmt.Errorf("创建 SQLite 表 anthropic_credentials 失败: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_anthropic_credentials_endpoint_id ON anthropic_credentials(endpoint_id);`); err != nil {
		return fmt.Errorf("创建 SQLite 索引 idx_anthropic_credentials_endpoint_id 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}
