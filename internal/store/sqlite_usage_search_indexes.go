package store

import (
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteUsageSearchIndexes(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_user_tokens_user_id_name ON user_tokens(user_id, name)`); err != nil {
		return fmt.Errorf("创建 idx_user_tokens_user_id_name 失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_usage_events_token_time_id ON usage_events(token_id, time, id)`); err != nil {
		return fmt.Errorf("创建 idx_usage_events_token_time_id 失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_usage_events_upstream_channel_time_id ON usage_events(upstream_channel_id, time, id)`); err != nil {
		return fmt.Errorf("创建 idx_usage_events_upstream_channel_time_id 失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_usage_events_model ON usage_events(model)`); err != nil {
		return fmt.Errorf("创建 idx_usage_events_model 失败: %w", err)
	}
	return nil
}
