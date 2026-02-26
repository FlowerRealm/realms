package store

import (
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLitePersonalAPIKeysTable(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}
	var v int
	err := db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name='personal_api_keys' LIMIT 1`).Scan(&v)
	if err == nil && v == 1 {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("检查 personal_api_keys 表失败: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始 personal_api_keys 初始化事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS personal_api_keys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NULL,
  key_hash BLOB NOT NULL,
  key_hint TEXT NULL,
  status INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  revoked_at DATETIME NULL,
  last_used_at DATETIME NULL
);
`); err != nil {
		return fmt.Errorf("创建 personal_api_keys 表失败: %w", err)
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uk_personal_api_keys_hash ON personal_api_keys(key_hash)`); err != nil {
		return fmt.Errorf("创建 personal_api_keys 索引失败: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_personal_api_keys_status ON personal_api_keys(status)`); err != nil {
		return fmt.Errorf("创建 personal_api_keys 索引失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 personal_api_keys 初始化事务失败: %w", err)
	}
	return nil
}
