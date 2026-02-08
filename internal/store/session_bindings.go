package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) GetSessionBindingPayload(ctx context.Context, userID int64, routeKeyHash string, now time.Time) (string, bool, error) {
	if s == nil || s.db == nil {
		return "", false, errors.New("store 未初始化")
	}
	if userID <= 0 {
		return "", false, nil
	}
	routeKeyHash = strings.TrimSpace(routeKeyHash)
	if routeKeyHash == "" {
		return "", false, nil
	}
	var payload string
	err := s.db.QueryRowContext(ctx, `
SELECT payload_json
FROM session_bindings
WHERE user_id=? AND route_key_hash=? AND expires_at > ?
`, userID, routeKeyHash, now.UTC()).Scan(&payload)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("查询会话绑定失败: %w", err)
	}
	return payload, true, nil
}

func (s *Store) UpsertSessionBindingPayload(ctx context.Context, userID int64, routeKeyHash string, payload string, expiresAt time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("store 未初始化")
	}
	if userID <= 0 {
		return nil
	}
	routeKeyHash = strings.TrimSpace(routeKeyHash)
	if routeKeyHash == "" {
		return nil
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil
	}
	expiresAt = expiresAt.UTC()
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(30 * time.Minute).UTC()
	}

	stmt := `
INSERT INTO session_bindings(user_id, route_key_hash, payload_json, expires_at, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
  payload_json=VALUES(payload_json),
  expires_at=VALUES(expires_at),
  updated_at=CURRENT_TIMESTAMP
`
	if s.dialect == DialectSQLite {
		stmt = `
INSERT INTO session_bindings(user_id, route_key_hash, payload_json, expires_at, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(user_id, route_key_hash) DO UPDATE SET
  payload_json=excluded.payload_json,
  expires_at=excluded.expires_at,
  updated_at=CURRENT_TIMESTAMP
`
	}
	if _, err := s.db.ExecContext(ctx, stmt, userID, routeKeyHash, payload, expiresAt); err != nil {
		return fmt.Errorf("写入会话绑定失败: %w", err)
	}
	return nil
}

func (s *Store) DeleteSessionBinding(ctx context.Context, userID int64, routeKeyHash string) error {
	if s == nil || s.db == nil {
		return errors.New("store 未初始化")
	}
	if userID <= 0 {
		return nil
	}
	routeKeyHash = strings.TrimSpace(routeKeyHash)
	if routeKeyHash == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM session_bindings WHERE user_id=? AND route_key_hash=?`, userID, routeKeyHash); err != nil {
		return fmt.Errorf("删除会话绑定失败: %w", err)
	}
	return nil
}
