package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteUsersUsernameRules(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite schema 修补事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	hasInsertTrigger, err := sqliteHasObject(ctx, tx, "trigger", "trg_users_username_validate_insert")
	if err != nil {
		return err
	}
	hasImmutableTrigger, err := sqliteHasObject(ctx, tx, "trigger", "trg_users_username_immutable")
	if err != nil {
		return err
	}
	if hasInsertTrigger && hasImmutableTrigger {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
		}
		return nil
	}

	// 存量修复：将不符合新规则的 username 回填为稳定且唯一的 `uid{id}`（仅字母/数字）。
	if _, err := tx.ExecContext(ctx, `
UPDATE users
SET username = 'uid' || id
WHERE username IS NULL OR username = '' OR username GLOB '*[^A-Za-z0-9]*'
`); err != nil {
		return fmt.Errorf("修复 users.username 存量数据失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
CREATE TRIGGER IF NOT EXISTS trg_users_username_validate_insert
BEFORE INSERT ON users
BEGIN
  SELECT RAISE(ABORT, '账号名仅支持字母/数字（区分大小写），不允许空格或特殊字符')
  WHERE NEW.username IS NULL OR NEW.username = '' OR NEW.username GLOB '*[^A-Za-z0-9]*';
END;
`); err != nil {
		return fmt.Errorf("创建 SQLite trigger trg_users_username_validate_insert 失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
CREATE TRIGGER IF NOT EXISTS trg_users_username_immutable
BEFORE UPDATE OF username ON users
BEGIN
  SELECT RAISE(ABORT, '账号名不可修改') WHERE NEW.username <> OLD.username;
END;
`); err != nil {
		return fmt.Errorf("创建 SQLite trigger trg_users_username_immutable 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite schema 修补事务失败: %w", err)
	}
	return nil
}

func sqliteHasObject(ctx context.Context, tx *sql.Tx, typ string, name string) (bool, error) {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM sqlite_master WHERE type=? AND name=?`, typ, name).Scan(&n); err != nil {
		return false, fmt.Errorf("检查 sqlite_master 失败: %w", err)
	}
	return n > 0, nil
}
