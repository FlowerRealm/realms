package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  id, email, username, password_hash, role, main_group, status, created_at, updated_at
FROM users
ORDER BY id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("查询 users 失败: %w", err)
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.Role, &u.MainGroup, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 users 失败: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 users 失败: %w", err)
	}

	return out, nil
}

func (s *Store) SetUserRole(ctx context.Context, userID int64, role string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE users
SET role=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, role, userID)
	if err != nil {
		return fmt.Errorf("更新用户角色失败: %w", err)
	}
	s.PurgeTokenAuthCacheAll()
	_ = s.BumpCacheInvalidation(ctx, CacheInvalidationKeyTokenAuth)
	return nil
}

func (s *Store) SetUserStatus(ctx context.Context, userID int64, status int) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE users
SET status=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, status, userID)
	if err != nil {
		return fmt.Errorf("更新用户状态失败: %w", err)
	}
	s.PurgeTokenAuthCacheAll()
	_ = s.BumpCacheInvalidation(ctx, CacheInvalidationKeyTokenAuth)
	return nil
}

func (s *Store) SetUserMainGroup(ctx context.Context, userID int64, mainGroup string) error {
	if userID <= 0 {
		return errors.New("userID 不能为空")
	}
	mainGroup = strings.TrimSpace(mainGroup)
	if mainGroup == "" {
		return errors.New("用户分组不能为空")
	}
	name, err := normalizeGroupName(mainGroup)
	if err != nil {
		return err
	}
	g, err := s.GetMainGroupByName(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("用户分组不存在")
		}
		return err
	}
	if g.Status != 1 {
		return errors.New("用户分组已禁用")
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE users
SET main_group=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, name, userID)
	if err != nil {
		return fmt.Errorf("更新 users.main_group 失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	s.PurgeTokenAuthCacheAll()
	_ = s.BumpCacheInvalidation(ctx, CacheInvalidationKeyTokenAuth)
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, userID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 注意：audit_events 里 user_id/token_id 均可能为空，为了彻底清理需要同时按 user_id 与 token_id 兜底删除。
	if _, err := tx.ExecContext(ctx, `
DELETE FROM audit_events
WHERE user_id=?
   OR token_id IN (SELECT id FROM user_tokens WHERE user_id=?)
`, userID, userID); err != nil {
		return fmt.Errorf("删除 audit_events 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM usage_events
WHERE user_id=?
   OR token_id IN (SELECT id FROM user_tokens WHERE user_id=?)
`, userID, userID); err != nil {
		return fmt.Errorf("删除 usage_events 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM email_verifications WHERE user_id=?`, userID); err != nil {
		return fmt.Errorf("删除 email_verifications 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_sessions WHERE user_id=?`, userID); err != nil {
		return fmt.Errorf("删除 user_sessions 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_subscriptions WHERE user_id=?`, userID); err != nil {
		return fmt.Errorf("删除 user_subscriptions 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM subscription_orders WHERE user_id=?`, userID); err != nil {
		return fmt.Errorf("删除 subscription_orders 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM topup_orders WHERE user_id=?`, userID); err != nil {
		return fmt.Errorf("删除 topup_orders 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_balances WHERE user_id=?`, userID); err != nil {
		return fmt.Errorf("删除 user_balances 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_tokens WHERE user_id=?`, userID); err != nil {
		return fmt.Errorf("删除 user_tokens 失败: %w", err)
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id=?`, userID)
	if err != nil {
		return fmt.Errorf("删除 users 失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	s.PurgeTokenAuthCacheAll()
	_ = s.BumpCacheInvalidation(ctx, CacheInvalidationKeyTokenAuth)
	return nil
}
