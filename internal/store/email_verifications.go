// Package store 提供邮箱验证码的持久化能力（注册/找回密码等场景）。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *Store) UpsertEmailVerification(ctx context.Context, userID *int64, email string, codeHash []byte, expiresAt time.Time) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM email_verifications WHERE email=?`, email); err != nil {
		return 0, fmt.Errorf("清理旧验证码失败: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
INSERT INTO email_verifications(user_id, email, code_hash, expires_at, verified_at, created_at)
VALUES(?, ?, ?, ?, NULL, CURRENT_TIMESTAMP)
`, userID, email, codeHash, expiresAt)
	if err != nil {
		return 0, fmt.Errorf("写入验证码失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取验证码 id 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交事务失败: %w", err)
	}
	return id, nil
}

func (s *Store) ConsumeEmailVerification(ctx context.Context, email string, codeHash []byte) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
DELETE FROM email_verifications
WHERE email=? AND code_hash=? AND verified_at IS NULL AND expires_at >= CURRENT_TIMESTAMP
`, email, codeHash)
	if err != nil {
		return false, fmt.Errorf("校验验证码失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("读取验证码校验结果失败: %w", err)
	}
	return n > 0, nil
}

func (s *Store) DeleteExpiredEmailVerifications(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM email_verifications WHERE expires_at < CURRENT_TIMESTAMP OR verified_at IS NOT NULL`)
	if err != nil {
		return 0, fmt.Errorf("清理过期验证码失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取清理结果失败: %w", err)
	}
	return n, nil
}

func (s *Store) GetEmailVerificationByEmail(ctx context.Context, email string) (EmailVerification, error) {
	var v EmailVerification
	var userID sql.NullInt64
	var verifiedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, email, code_hash, expires_at, verified_at, created_at
FROM email_verifications
WHERE email=?
`, email).Scan(&v.ID, &userID, &v.Email, &v.CodeHash, &v.ExpiresAt, &verifiedAt, &v.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EmailVerification{}, sql.ErrNoRows
		}
		return EmailVerification{}, fmt.Errorf("查询验证码失败: %w", err)
	}
	if userID.Valid {
		v.UserID = &userID.Int64
	}
	if verifiedAt.Valid {
		v.VerifiedAt = &verifiedAt.Time
	}
	return v, nil
}
