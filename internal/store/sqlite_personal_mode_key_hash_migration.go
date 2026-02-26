package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ensureSQLitePersonalModeKeyHashMigrated 将历史 app_settings['self_mode_key_hash'] 的值迁移到
// app_settings['personal_mode_key_hash']（不会覆盖已存在且非空的 personal_mode_key_hash）。
func ensureSQLitePersonalModeKeyHashMigrated(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite app_settings 迁移事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var table int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM sqlite_master WHERE type='table' AND name='app_settings' LIMIT 1`).Scan(&table); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// 旧库可能还没有 app_settings 表，直接跳过。
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("提交 SQLite app_settings 迁移事务失败: %w", err)
			}
			return nil
		}
		return fmt.Errorf("检查 SQLite app_settings 表失败: %w", err)
	}

	var personal sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key=? LIMIT 1`, SettingPersonalModeKeyHash).Scan(&personal)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("读取 app_settings[%s] 失败: %w", SettingPersonalModeKeyHash, err)
	}
	if err == nil && strings.TrimSpace(personal.String) != "" {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交 SQLite app_settings 迁移事务失败: %w", err)
		}
		return nil
	}

	var (
		legacyValue     sql.NullString
		legacyCreatedAt sql.NullString
		legacyUpdatedAt sql.NullString
	)
	err = tx.QueryRowContext(ctx, `
SELECT value, created_at, updated_at
FROM app_settings
WHERE key='self_mode_key_hash'
LIMIT 1
`).Scan(&legacyValue, &legacyCreatedAt, &legacyUpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("提交 SQLite app_settings 迁移事务失败: %w", err)
			}
			return nil
		}
		return fmt.Errorf("读取 app_settings[self_mode_key_hash] 失败: %w", err)
	}
	if strings.TrimSpace(legacyValue.String) == "" {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交 SQLite app_settings 迁移事务失败: %w", err)
		}
		return nil
	}

	// personal key 不存在或为空：补齐（不覆盖非空值）。
	if err == nil {
		if _, err := tx.ExecContext(ctx, `
UPDATE app_settings
SET value=?, updated_at=CURRENT_TIMESTAMP
WHERE key=?
  AND TRIM(value)=''
`, legacyValue.String, SettingPersonalModeKeyHash); err != nil {
			return fmt.Errorf("回填 app_settings[%s] 失败: %w", SettingPersonalModeKeyHash, err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO app_settings(key, value, created_at, updated_at)
VALUES(?, ?, COALESCE(?, CURRENT_TIMESTAMP), COALESCE(?, CURRENT_TIMESTAMP))
ON CONFLICT(key) DO NOTHING
`, SettingPersonalModeKeyHash, legacyValue.String, legacyCreatedAt, legacyUpdatedAt); err != nil {
			return fmt.Errorf("写入 app_settings[%s] 失败: %w", SettingPersonalModeKeyHash, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite app_settings 迁移事务失败: %w", err)
	}
	return nil
}
