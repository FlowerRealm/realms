// Package store 提供 OAuth Apps（外部客户端授权）相关的持久化能力。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"realms/internal/crypto"
)

const (
	OAuthAppStatusDisabled = 0
	OAuthAppStatusEnabled  = 1
)

func NormalizeOAuthScope(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if len(s) > 2048 {
		return "", fmt.Errorf("scope 过长")
	}
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return "", nil
	}
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return strings.Join(out, " "), nil
}

func NormalizeOAuthRedirectURI(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", fmt.Errorf("redirect_uri 不能为空")
	}
	if len(v) > 2048 {
		return "", fmt.Errorf("redirect_uri 过长")
	}
	u, err := url.Parse(v)
	if err != nil {
		return "", fmt.Errorf("redirect_uri 格式错误")
	}
	u.Scheme = strings.ToLower(strings.TrimSpace(u.Scheme))
	u.Host = strings.ToLower(strings.TrimSpace(u.Host))
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("redirect_uri 仅支持 http/https")
	}
	if u.Host == "" {
		return "", fmt.Errorf("redirect_uri 缺少 host")
	}
	if u.Fragment != "" {
		return "", fmt.Errorf("redirect_uri 不允许包含 fragment")
	}
	if u.User != nil {
		return "", fmt.Errorf("redirect_uri 不允许包含 userinfo")
	}
	return u.String(), nil
}

func (s *Store) CreateOAuthApp(ctx context.Context, clientID string, name string, clientSecretHash []byte, status int) (int64, error) {
	if s == nil {
		return 0, errors.New("store 为空")
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return 0, errors.New("clientID 不能为空")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("name 不能为空")
	}
	if status != OAuthAppStatusEnabled && status != OAuthAppStatusDisabled {
		status = OAuthAppStatusEnabled
	}

	res, err := s.db.ExecContext(ctx, `
INSERT INTO oauth_apps(client_id, name, client_secret_hash, status, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, clientID, name, nullableBytes(clientSecretHash), status)
	if err != nil {
		return 0, fmt.Errorf("创建 oauth_apps 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取 oauth_app id 失败: %w", err)
	}
	return id, nil
}

func (s *Store) UpdateOAuthApp(ctx context.Context, appID int64, name string, status int) error {
	if s == nil {
		return errors.New("store 为空")
	}
	if appID == 0 {
		return errors.New("appID 不能为空")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name 不能为空")
	}
	if status != OAuthAppStatusEnabled && status != OAuthAppStatusDisabled {
		status = OAuthAppStatusEnabled
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE oauth_apps
SET name=?, status=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, name, status, appID)
	if err != nil {
		return fmt.Errorf("更新 oauth_apps 失败: %w", err)
	}
	return nil
}

func (s *Store) UpdateOAuthAppSecretHash(ctx context.Context, appID int64, clientSecretHash []byte) error {
	if s == nil {
		return errors.New("store 为空")
	}
	if appID == 0 {
		return errors.New("appID 不能为空")
	}
	if len(clientSecretHash) == 0 {
		return errors.New("clientSecretHash 不能为空")
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE oauth_apps
SET client_secret_hash=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, clientSecretHash, appID)
	if err != nil {
		return fmt.Errorf("更新 oauth_apps secret 失败: %w", err)
	}
	return nil
}

func (s *Store) GetOAuthAppByID(ctx context.Context, appID int64) (OAuthApp, error) {
	if s == nil {
		return OAuthApp{}, errors.New("store 为空")
	}
	var a OAuthApp
	err := s.db.QueryRowContext(ctx, `
SELECT id, client_id, name, client_secret_hash, status, created_at, updated_at
FROM oauth_apps
WHERE id=?
`, appID).Scan(&a.ID, &a.ClientID, &a.Name, &a.ClientSecretHash, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OAuthApp{}, sql.ErrNoRows
		}
		return OAuthApp{}, fmt.Errorf("查询 oauth_apps 失败: %w", err)
	}
	return a, nil
}

func (s *Store) GetOAuthAppByClientID(ctx context.Context, clientID string) (OAuthApp, bool, error) {
	if s == nil {
		return OAuthApp{}, false, errors.New("store 为空")
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return OAuthApp{}, false, errors.New("clientID 不能为空")
	}
	var a OAuthApp
	err := s.db.QueryRowContext(ctx, `
SELECT id, client_id, name, client_secret_hash, status, created_at, updated_at
FROM oauth_apps
WHERE client_id=?
`, clientID).Scan(&a.ID, &a.ClientID, &a.Name, &a.ClientSecretHash, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OAuthApp{}, false, nil
		}
		return OAuthApp{}, false, fmt.Errorf("查询 oauth_apps 失败: %w", err)
	}
	return a, true, nil
}

func (s *Store) ListOAuthApps(ctx context.Context) ([]OAuthApp, error) {
	if s == nil {
		return nil, errors.New("store 为空")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, client_id, name, client_secret_hash, status, created_at, updated_at
FROM oauth_apps
ORDER BY id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("查询 oauth_apps 失败: %w", err)
	}
	defer rows.Close()

	var out []OAuthApp
	for rows.Next() {
		var a OAuthApp
		if err := rows.Scan(&a.ID, &a.ClientID, &a.Name, &a.ClientSecretHash, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("读取 oauth_apps 失败: %w", err)
		}
		out = append(out, a)
	}
	return out, nil
}

func (s *Store) ReplaceOAuthAppRedirectURIs(ctx context.Context, appID int64, redirectURIs []string) error {
	if s == nil {
		return errors.New("store 为空")
	}
	if appID == 0 {
		return errors.New("appID 不能为空")
	}

	seen := make(map[string]struct{}, len(redirectURIs))
	normalized := make([]string, 0, len(redirectURIs))
	for _, raw := range redirectURIs {
		u, err := NormalizeOAuthRedirectURI(raw)
		if err != nil {
			return err
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		normalized = append(normalized, u)
	}
	if len(normalized) == 0 {
		return fmt.Errorf("redirect_uri 不能为空")
	}
	sort.Strings(normalized)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_app_redirect_uris WHERE app_id=?`, appID); err != nil {
		return fmt.Errorf("清理 redirect_uri 失败: %w", err)
	}
	for _, u := range normalized {
		h := crypto.TokenHash(u)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO oauth_app_redirect_uris(app_id, redirect_uri, redirect_uri_hash, created_at)
VALUES(?, ?, ?, CURRENT_TIMESTAMP)
`, appID, u, h); err != nil {
			return fmt.Errorf("写入 redirect_uri 失败: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) ListOAuthAppRedirectURIs(ctx context.Context, appID int64) ([]string, error) {
	if s == nil {
		return nil, errors.New("store 为空")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT redirect_uri
FROM oauth_app_redirect_uris
WHERE app_id=?
ORDER BY id ASC
`, appID)
	if err != nil {
		return nil, fmt.Errorf("查询 oauth_app_redirect_uris 失败: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, fmt.Errorf("读取 oauth_app_redirect_uris 失败: %w", err)
		}
		out = append(out, u)
	}
	return out, nil
}

func (s *Store) OAuthAppHasRedirectURI(ctx context.Context, appID int64, redirectURI string) (bool, error) {
	if s == nil {
		return false, errors.New("store 为空")
	}
	if appID == 0 {
		return false, errors.New("appID 不能为空")
	}
	redirectURI, err := NormalizeOAuthRedirectURI(redirectURI)
	if err != nil {
		return false, err
	}
	h := crypto.TokenHash(redirectURI)
	var v int
	err = s.db.QueryRowContext(ctx, `
SELECT 1
FROM oauth_app_redirect_uris
WHERE app_id=? AND redirect_uri_hash=?
LIMIT 1
`, appID, h).Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("查询 oauth_app_redirect_uris 失败: %w", err)
	}
	return true, nil
}

func (s *Store) UpsertOAuthUserGrant(ctx context.Context, userID int64, appID int64, scope string) error {
	if s == nil {
		return errors.New("store 为空")
	}
	if userID == 0 {
		return errors.New("userID 不能为空")
	}
	if appID == 0 {
		return errors.New("appID 不能为空")
	}
	scope, err := NormalizeOAuthScope(scope)
	if err != nil {
		return err
	}
	stmt := `
INSERT INTO oauth_user_grants(user_id, app_id, scope, created_at, updated_at)
VALUES(?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE scope=VALUES(scope), updated_at=CURRENT_TIMESTAMP
`
	if s.dialect == DialectSQLite {
		stmt = `
INSERT INTO oauth_user_grants(user_id, app_id, scope, created_at, updated_at)
VALUES(?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(user_id, app_id) DO UPDATE SET scope=excluded.scope, updated_at=CURRENT_TIMESTAMP
`
	}
	_, err = s.db.ExecContext(ctx, stmt, userID, appID, scope)
	if err != nil {
		return fmt.Errorf("写入 oauth_user_grants 失败: %w", err)
	}
	return nil
}

func (s *Store) GetOAuthUserGrant(ctx context.Context, userID int64, appID int64) (OAuthUserGrant, bool, error) {
	if s == nil {
		return OAuthUserGrant{}, false, errors.New("store 为空")
	}
	var g OAuthUserGrant
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, app_id, scope, created_at, updated_at
FROM oauth_user_grants
WHERE user_id=? AND app_id=?
`, userID, appID).Scan(&g.ID, &g.UserID, &g.AppID, &g.Scope, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OAuthUserGrant{}, false, nil
		}
		return OAuthUserGrant{}, false, fmt.Errorf("查询 oauth_user_grants 失败: %w", err)
	}
	return g, true, nil
}

func (s *Store) InsertOAuthAuthCode(ctx context.Context, codeHash []byte, appID int64, userID int64, redirectURI string, scope string, codeChallenge *string, codeChallengeMethod *string, expiresAt time.Time) (int64, error) {
	if s == nil {
		return 0, errors.New("store 为空")
	}
	if len(codeHash) != 32 {
		return 0, errors.New("codeHash 非法")
	}
	if appID == 0 {
		return 0, errors.New("appID 不能为空")
	}
	if userID == 0 {
		return 0, errors.New("userID 不能为空")
	}
	redirectURI, err := NormalizeOAuthRedirectURI(redirectURI)
	if err != nil {
		return 0, err
	}
	scope, err = NormalizeOAuthScope(scope)
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO oauth_auth_codes(code_hash, app_id, user_id, redirect_uri, scope, code_challenge, code_challenge_method, expires_at, consumed_at, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, NULL, CURRENT_TIMESTAMP)
`, codeHash, appID, userID, redirectURI, scope, nullableString(codeChallenge), nullableString(codeChallengeMethod), expiresAt)
	if err != nil {
		return 0, fmt.Errorf("写入 oauth_auth_codes 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取 oauth_auth_code id 失败: %w", err)
	}
	return id, nil
}

func (s *Store) ConsumeOAuthAuthCode(ctx context.Context, code string, appID int64, redirectURI string) (OAuthAuthCode, bool, error) {
	if s == nil {
		return OAuthAuthCode{}, false, errors.New("store 为空")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return OAuthAuthCode{}, false, errors.New("code 不能为空")
	}
	if appID == 0 {
		return OAuthAuthCode{}, false, errors.New("appID 不能为空")
	}
	redirectURI, err := NormalizeOAuthRedirectURI(redirectURI)
	if err != nil {
		return OAuthAuthCode{}, false, err
	}
	codeHash := crypto.TokenHash(code)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OAuthAuthCode{}, false, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
UPDATE oauth_auth_codes
SET consumed_at=CURRENT_TIMESTAMP
WHERE code_hash=? AND app_id=? AND redirect_uri=? AND consumed_at IS NULL AND expires_at >= CURRENT_TIMESTAMP
`, codeHash, appID, redirectURI)
	if err != nil {
		return OAuthAuthCode{}, false, fmt.Errorf("消费授权码失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return OAuthAuthCode{}, false, fmt.Errorf("读取授权码消费结果失败: %w", err)
	}
	if n == 0 {
		return OAuthAuthCode{}, false, nil
	}

	var out OAuthAuthCode
	var codeChallenge sql.NullString
	var codeChallengeMethod sql.NullString
	var consumedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
SELECT id, code_hash, app_id, user_id, redirect_uri, scope, code_challenge, code_challenge_method, expires_at, consumed_at, created_at
FROM oauth_auth_codes
WHERE code_hash=?
`, codeHash).Scan(
		&out.ID,
		&out.CodeHash,
		&out.AppID,
		&out.UserID,
		&out.RedirectURI,
		&out.Scope,
		&codeChallenge,
		&codeChallengeMethod,
		&out.ExpiresAt,
		&consumedAt,
		&out.CreatedAt,
	)
	if err != nil {
		return OAuthAuthCode{}, false, fmt.Errorf("读取授权码失败: %w", err)
	}
	if codeChallenge.Valid {
		out.CodeChallenge = &codeChallenge.String
	}
	if codeChallengeMethod.Valid {
		out.CodeChallengeMethod = &codeChallengeMethod.String
	}
	if consumedAt.Valid {
		out.ConsumedAt = &consumedAt.Time
	}

	if err := tx.Commit(); err != nil {
		return OAuthAuthCode{}, false, fmt.Errorf("提交事务失败: %w", err)
	}
	return out, true, nil
}

func (s *Store) CreateOAuthAppToken(ctx context.Context, appID int64, userID int64, tokenID int64, scope string) error {
	if s == nil {
		return errors.New("store 为空")
	}
	if appID == 0 {
		return errors.New("appID 不能为空")
	}
	if userID == 0 {
		return errors.New("userID 不能为空")
	}
	if tokenID == 0 {
		return errors.New("tokenID 不能为空")
	}
	scope, err := NormalizeOAuthScope(scope)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO oauth_app_tokens(app_id, user_id, token_id, scope, created_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP)
`, appID, userID, tokenID, scope)
	if err != nil {
		return fmt.Errorf("写入 oauth_app_tokens 失败: %w", err)
	}
	return nil
}

func nullableString(s *string) any {
	if s == nil {
		return nil
	}
	if strings.TrimSpace(*s) == "" {
		return nil
	}
	return strings.TrimSpace(*s)
}
