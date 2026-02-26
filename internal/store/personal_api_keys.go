package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"realms/internal/crypto"
)

type PersonalAPIKey struct {
	ID         int64
	Name       *string
	KeyHash    []byte
	KeyHint    *string
	Status     int
	CreatedAt  time.Time
	RevokedAt  *time.Time
	LastUsedAt *time.Time
}

func (s *Store) CreatePersonalAPIKey(ctx context.Context, name *string, rawKey string) (int64, *string, error) {
	if s == nil || s.db == nil {
		return 0, nil, errors.New("store 未初始化")
	}
	if strings.TrimSpace(rawKey) == "" {
		return 0, nil, errors.New("key 不能为空")
	}

	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			name = nil
		} else {
			name = &n
		}
	}

	keyHash := crypto.TokenHash(rawKey)
	hint := tokenHint(rawKey)

	res, err := s.db.ExecContext(ctx, `
INSERT INTO personal_api_keys(name, key_hash, key_hint, status, created_at)
VALUES(?, ?, ?, 1, CURRENT_TIMESTAMP)
`, name, keyHash, hint)
	if err != nil {
		return 0, nil, fmt.Errorf("创建 personal_api_key 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, nil, fmt.Errorf("获取 personal_api_key id 失败: %w", err)
	}
	return id, hint, nil
}

func (s *Store) ListPersonalAPIKeys(ctx context.Context) ([]PersonalAPIKey, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store 未初始化")
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, key_hash, key_hint, status, created_at, revoked_at, last_used_at
FROM personal_api_keys
ORDER BY id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("查询 personal_api_keys 失败: %w", err)
	}
	defer rows.Close()

	var out []PersonalAPIKey
	for rows.Next() {
		var row PersonalAPIKey
		if err := rows.Scan(&row.ID, &row.Name, &row.KeyHash, &row.KeyHint, &row.Status, &row.CreatedAt, &row.RevokedAt, &row.LastUsedAt); err != nil {
			return nil, fmt.Errorf("扫描 personal_api_keys 失败: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 personal_api_keys 失败: %w", err)
	}
	return out, nil
}

func (s *Store) RevokePersonalAPIKey(ctx context.Context, id int64) error {
	if s == nil || s.db == nil {
		return errors.New("store 未初始化")
	}
	if id <= 0 {
		return errors.New("id 不合法")
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE personal_api_keys
SET status=0, revoked_at=CURRENT_TIMESTAMP
WHERE id=? AND status=1
`, id)
	if err != nil {
		return fmt.Errorf("撤销 personal_api_key 失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetPersonalAPIKeyIDByHash(ctx context.Context, keyHash []byte) (int64, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("store 未初始化")
	}
	if len(keyHash) == 0 {
		return 0, errors.New("key_hash 为空")
	}

	var id int64
	err := s.db.QueryRowContext(ctx, `
SELECT id
FROM personal_api_keys
WHERE key_hash=? AND status=1
`, keyHash).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, sql.ErrNoRows
		}
		return 0, fmt.Errorf("查询 personal_api_key 失败: %w", err)
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE personal_api_keys SET last_used_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	return id, nil
}
