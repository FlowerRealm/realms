package store

import (
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteUsageEventDetailsTable(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS usage_event_details (
  usage_event_id INTEGER PRIMARY KEY,
  upstream_request_body TEXT NULL,
  upstream_response_body TEXT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_usage_event_details_updated_at ON usage_event_details (updated_at);
`)
	if err != nil {
		return fmt.Errorf("创建 usage_event_details 表失败: %w", err)
	}
	return nil
}
