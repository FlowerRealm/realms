package store

import (
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteRedemptionCodeTables(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS redemption_codes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  batch_name TEXT NOT NULL,
  code TEXT NOT NULL,
  distribution_mode TEXT NOT NULL,
  reward_type TEXT NOT NULL,
  subscription_plan_id INTEGER NULL,
  balance_usd DECIMAL(20,6) NOT NULL DEFAULT 0,
  max_redemptions INTEGER NOT NULL DEFAULT 1,
  redeemed_count INTEGER NOT NULL DEFAULT 0,
  expires_at DATETIME NULL,
  status INTEGER NOT NULL DEFAULT 1,
  created_by INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)
`); err != nil {
		return fmt.Errorf("创建 redemption_codes 表失败: %w", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uk_redemption_codes_code ON redemption_codes (code)`); err != nil {
		return fmt.Errorf("创建 redemption_codes code 索引失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_redemption_codes_batch_name ON redemption_codes (batch_name)`); err != nil {
		return fmt.Errorf("创建 redemption_codes batch_name 索引失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_redemption_codes_distribution_mode ON redemption_codes (distribution_mode)`); err != nil {
		return fmt.Errorf("创建 redemption_codes distribution_mode 索引失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_redemption_codes_reward_type ON redemption_codes (reward_type)`); err != nil {
		return fmt.Errorf("创建 redemption_codes reward_type 索引失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_redemption_codes_status ON redemption_codes (status)`); err != nil {
		return fmt.Errorf("创建 redemption_codes status 索引失败: %w", err)
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS redemption_code_redemptions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  code_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  reward_type TEXT NOT NULL,
  balance_usd DECIMAL(20,6) NOT NULL DEFAULT 0,
  subscription_id INTEGER NULL,
  subscription_activation_mode TEXT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)
`); err != nil {
		return fmt.Errorf("创建 redemption_code_redemptions 表失败: %w", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uk_redemption_code_redemptions_code_user ON redemption_code_redemptions (code_id, user_id)`); err != nil {
		return fmt.Errorf("创建 redemption_code_redemptions code/user 索引失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_redemption_code_redemptions_user_id ON redemption_code_redemptions (user_id)`); err != nil {
		return fmt.Errorf("创建 redemption_code_redemptions user_id 索引失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_redemption_code_redemptions_subscription_id ON redemption_code_redemptions (subscription_id)`); err != nil {
		return fmt.Errorf("创建 redemption_code_redemptions subscription_id 索引失败: %w", err)
	}
	return nil
}
