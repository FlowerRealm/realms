// Package store 提供数据库读写的封装与基础约束，保证业务层只处理领域语义而不是 SQL 细节。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"realms/internal/config"
	"realms/internal/crypto"
)

const (
	UserRoleRoot = "root"
	UserRoleUser = "user"
)

var (
	ErrUserTokenRevoked       = errors.New("user token revoked")
	ErrUserTokenNotRevealable = errors.New("user token not revealable")
)

type Store struct {
	db      *sql.DB
	dialect Dialect

	appSettingsDefaults    config.AppSettingsDefaultsConfig
	hasAppSettingsDefaults bool
}

func New(db *sql.DB) *Store {
	return &Store{
		db:      db,
		dialect: DialectMySQL,
	}
}

func (s *Store) SetDialect(d Dialect) {
	if strings.TrimSpace(string(d)) == "" {
		return
	}
	s.dialect = d
}

func (s *Store) SetAppSettingsDefaults(v config.AppSettingsDefaultsConfig) {
	s.appSettingsDefaults = v
	s.hasAppSettingsDefaults = true
}

func (s *Store) CountUsers(ctx context.Context) (int64, error) {
	var n int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("统计用户失败: %w", err)
	}
	return n, nil
}

func (s *Store) CreateUser(ctx context.Context, email string, username string, passwordHash []byte, role string) (int64, error) {
	if role == "" {
		role = UserRoleUser
	}
	if strings.TrimSpace(username) == "" {
		return 0, fmt.Errorf("账号名不能为空")
	}
	res, err := s.db.ExecContext(ctx, `
	INSERT INTO users(email, username, password_hash, role, status, created_at, updated_at)
	VALUES(?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, email, username, passwordHash, role)
	if err != nil {
		return 0, fmt.Errorf("创建用户失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取用户 id 失败: %w", err)
	}
	return id, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `
	SELECT
	  id, email, username, password_hash, role, main_group, status, created_at, updated_at
	FROM users
	WHERE email=?
	`, email).Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.Role, &u.MainGroup, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, sql.ErrNoRows
		}
		return User{}, fmt.Errorf("查询用户失败: %w", err)
	}
	return u, nil
}

func (s *Store) GetUserByID(ctx context.Context, userID int64) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `
	SELECT
	  id, email, username, password_hash, role, main_group, status, created_at, updated_at
	FROM users
	WHERE id=?
	`, userID).Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.Role, &u.MainGroup, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, sql.ErrNoRows
		}
		return User{}, fmt.Errorf("查询用户失败: %w", err)
	}
	return u, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `
	SELECT
	  id, email, username, password_hash, role, main_group, status, created_at, updated_at
	FROM users
	WHERE username=?
	`, username).Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.Role, &u.MainGroup, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, sql.ErrNoRows
		}
		return User{}, fmt.Errorf("查询用户失败: %w", err)
	}
	return u, nil
}

func (s *Store) UpdateUserEmail(ctx context.Context, userID int64, email string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE users
SET email=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, email, userID)
	if err != nil {
		return fmt.Errorf("更新用户邮箱失败: %w", err)
	}
	return nil
}

func (s *Store) UpdateUserPasswordHash(ctx context.Context, userID int64, passwordHash []byte) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE users
SET password_hash=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("更新用户密码失败: %w", err)
	}
	return nil
}

func (s *Store) DeleteSessionsByUserID(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE user_id=?`, userID)
	if err != nil {
		return fmt.Errorf("清理用户会话失败: %w", err)
	}
	return nil
}

func (s *Store) CreateUserToken(ctx context.Context, userID int64, name *string, rawToken string) (int64, *string, error) {
	tokenHash := crypto.TokenHash(rawToken)
	hint := tokenHint(rawToken)
	res, err := s.db.ExecContext(ctx, `
INSERT INTO user_tokens(user_id, name, token_hash, token_plain, token_hint, status, created_at)
VALUES(?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP)
`, userID, name, tokenHash, rawToken, hint)
	if err != nil {
		return 0, nil, fmt.Errorf("创建 Token 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, nil, fmt.Errorf("获取 Token id 失败: %w", err)
	}
	return id, hint, nil
}

func (s *Store) RotateUserToken(ctx context.Context, userID, tokenID int64, rawToken string) error {
	if userID == 0 {
		return errors.New("userID 不能为空")
	}
	if tokenID == 0 {
		return errors.New("tokenID 不能为空")
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return errors.New("rawToken 不能为空")
	}

	tokenHash := crypto.TokenHash(rawToken)
	hint := tokenHint(rawToken)

	res, err := s.db.ExecContext(ctx, `
UPDATE user_tokens
SET token_hash=?, token_plain=?, token_hint=?, status=1, revoked_at=NULL, last_used_at=NULL
WHERE id=? AND user_id=?
`, tokenHash, rawToken, hint, tokenID, userID)
	if err != nil {
		return fmt.Errorf("重新生成 Token 失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListUserTokens(ctx context.Context, userID int64) ([]UserToken, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, name, token_hash, token_hint, status, created_at, revoked_at, last_used_at
FROM user_tokens
WHERE user_id=?
ORDER BY id DESC
`, userID)
	if err != nil {
		return nil, fmt.Errorf("查询 Token 列表失败: %w", err)
	}
	defer rows.Close()
	var out []UserToken
	for rows.Next() {
		var t UserToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.TokenHint, &t.Status, &t.CreatedAt, &t.RevokedAt, &t.LastUsedAt); err != nil {
			return nil, fmt.Errorf("扫描 Token 失败: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 Token 失败: %w", err)
	}
	return out, nil
}

func (s *Store) RevealUserToken(ctx context.Context, userID, tokenID int64) (string, error) {
	if userID == 0 {
		return "", errors.New("userID 不能为空")
	}
	if tokenID == 0 {
		return "", errors.New("tokenID 不能为空")
	}

	var (
		tokenPlain sql.NullString
		status     int
	)
	err := s.db.QueryRowContext(ctx, `
SELECT token_plain, status
FROM user_tokens
WHERE id=? AND user_id=?
`, tokenID, userID).Scan(&tokenPlain, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", sql.ErrNoRows
		}
		return "", fmt.Errorf("查询 Token 明文失败: %w", err)
	}

	if status != 1 {
		return "", ErrUserTokenRevoked
	}

	token := strings.TrimSpace(tokenPlain.String)
	if !tokenPlain.Valid || token == "" {
		return "", ErrUserTokenNotRevealable
	}
	return token, nil
}

func (s *Store) RevokeUserToken(ctx context.Context, userID, tokenID int64) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE user_tokens
SET status=0, revoked_at=CURRENT_TIMESTAMP, token_plain=NULL
WHERE id=? AND user_id=? AND status=1
`, tokenID, userID)
	if err != nil {
		return fmt.Errorf("撤销 Token 失败: %w", err)
	}
	_, _ = res.RowsAffected()
	return nil
}

func (s *Store) DeleteUserToken(ctx context.Context, userID, tokenID int64) error {
	if userID == 0 {
		return errors.New("userID 不能为空")
	}
	if tokenID == 0 {
		return errors.New("tokenID 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM user_tokens WHERE id=? AND user_id=?`, tokenID, userID)
	if err != nil {
		return fmt.Errorf("删除 Token 失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) RevokeActiveUserTokensByName(ctx context.Context, userID int64, name string) error {
	if userID == 0 {
		return errors.New("userID 不能为空")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name 不能为空")
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE user_tokens
SET status=0, revoked_at=CURRENT_TIMESTAMP
WHERE user_id=? AND name=? AND status=1
`, userID, name)
	if err != nil {
		return fmt.Errorf("撤销 Token 失败: %w", err)
	}
	return nil
}

type TokenAuth struct {
	UserID  int64
	TokenID int64
	Role    string
	Groups  []string
}

func (s *Store) GetTokenAuthByRawToken(ctx context.Context, rawToken string) (TokenAuth, error) {
	tokenHash := crypto.TokenHash(rawToken)
	return s.GetTokenAuthByTokenHash(ctx, tokenHash)
}

func (s *Store) GetTokenAuthByTokenHash(ctx context.Context, tokenHash []byte) (TokenAuth, error) {
	var auth TokenAuth
	err := s.db.QueryRowContext(ctx, `
SELECT
  u.id, t.id, u.role
FROM user_tokens t
JOIN users u ON u.id=t.user_id
WHERE t.token_hash=? AND t.status=1 AND u.status=1
`, tokenHash).Scan(&auth.UserID, &auth.TokenID, &auth.Role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TokenAuth{}, sql.ErrNoRows
		}
		return TokenAuth{}, fmt.Errorf("查询 Token 鉴权失败: %w", err)
	}
	auth.Groups, _ = s.ListEffectiveTokenGroups(ctx, auth.TokenID)
	_, _ = s.db.ExecContext(ctx, `UPDATE user_tokens SET last_used_at=CURRENT_TIMESTAMP WHERE id=?`, auth.TokenID)
	return auth, nil
}

func (s *Store) CreateSession(ctx context.Context, userID int64, rawSession string, csrfToken string, expiresAt time.Time) (int64, error) {
	sessionHash := crypto.TokenHash(rawSession)
	res, err := s.db.ExecContext(ctx, `
INSERT INTO user_sessions(user_id, session_hash, csrf_token, expires_at, created_at, last_seen_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, userID, sessionHash, csrfToken, expiresAt)
	if err != nil {
		return 0, fmt.Errorf("创建会话失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取会话 id 失败: %w", err)
	}
	return id, nil
}

func (s *Store) UpdateSessionCSRFToken(ctx context.Context, sessionID int64, csrfToken string) error {
	token := strings.TrimSpace(csrfToken)
	if sessionID <= 0 {
		return errors.New("session_id 不合法")
	}
	if token == "" {
		return errors.New("csrf_token 不能为空")
	}
	if len(token) > 64 {
		return errors.New("csrf_token 过长")
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE user_sessions
SET csrf_token=?
WHERE id=?
`, token, sessionID)
	if err != nil {
		return fmt.Errorf("更新会话 csrf_token 失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetSessionByRaw(ctx context.Context, rawSession string) (UserSession, error) {
	sessionHash := crypto.TokenHash(rawSession)
	return s.GetSessionByHash(ctx, sessionHash)
}

func (s *Store) GetSessionByHash(ctx context.Context, sessionHash []byte) (UserSession, error) {
	var sess UserSession
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, session_hash, csrf_token, expires_at, created_at, last_seen_at
FROM user_sessions
WHERE session_hash=?
`, sessionHash).Scan(&sess.ID, &sess.UserID, &sess.SessionHash, &sess.CSRFToken, &sess.ExpiresAt, &sess.CreatedAt, &sess.LastSeenAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserSession{}, sql.ErrNoRows
		}
		return UserSession{}, fmt.Errorf("查询会话失败: %w", err)
	}
	if time.Now().After(sess.ExpiresAt) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE id=?`, sess.ID)
		return UserSession{}, sql.ErrNoRows
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE user_sessions SET last_seen_at=CURRENT_TIMESTAMP WHERE id=?`, sess.ID)
	return sess, nil
}

func (s *Store) DeleteSessionByRaw(ctx context.Context, rawSession string) error {
	sessionHash := crypto.TokenHash(rawSession)
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE session_hash=?`, sessionHash)
	if err != nil {
		return fmt.Errorf("删除会话失败: %w", err)
	}
	return nil
}

func tokenHint(raw string) *string {
	if raw == "" {
		return nil
	}
	const keep = 6
	if len(raw) <= keep {
		h := raw
		return &h
	}
	h := raw[len(raw)-keep:]
	return &h
}
