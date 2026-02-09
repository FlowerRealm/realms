package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type OpenAIObjectRef struct {
	ObjectType    string
	ObjectID      string
	UserID        int64
	TokenID       int64
	SelectionJSON string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (s *Store) UpsertOpenAIObjectRef(ctx context.Context, ref OpenAIObjectRef) error {
	if s == nil || s.db == nil {
		return errors.New("store 未初始化")
	}
	ref.ObjectType = strings.TrimSpace(ref.ObjectType)
	ref.ObjectID = strings.TrimSpace(ref.ObjectID)
	ref.SelectionJSON = strings.TrimSpace(ref.SelectionJSON)
	if ref.ObjectType == "" || ref.ObjectID == "" {
		return nil
	}
	if ref.UserID <= 0 {
		return nil
	}
	if ref.SelectionJSON == "" {
		return nil
	}

	stmt := `
INSERT INTO openai_object_refs(object_type, object_id, user_id, token_id, selection_json, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
  user_id=VALUES(user_id),
  token_id=VALUES(token_id),
  selection_json=VALUES(selection_json),
  updated_at=CURRENT_TIMESTAMP
`
	if s.dialect == DialectSQLite {
		stmt = `
INSERT INTO openai_object_refs(object_type, object_id, user_id, token_id, selection_json, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(object_type, object_id) DO UPDATE SET
  user_id=excluded.user_id,
  token_id=excluded.token_id,
  selection_json=excluded.selection_json,
  updated_at=CURRENT_TIMESTAMP
`
	}
	if _, err := s.db.ExecContext(ctx, stmt, ref.ObjectType, ref.ObjectID, ref.UserID, ref.TokenID, ref.SelectionJSON); err != nil {
		return fmt.Errorf("写入 openai_object_refs 失败: %w", err)
	}
	return nil
}

func (s *Store) GetOpenAIObjectRef(ctx context.Context, objectType string, objectID string) (OpenAIObjectRef, bool, error) {
	if s == nil || s.db == nil {
		return OpenAIObjectRef{}, false, errors.New("store 未初始化")
	}
	objectType = strings.TrimSpace(objectType)
	objectID = strings.TrimSpace(objectID)
	if objectType == "" || objectID == "" {
		return OpenAIObjectRef{}, false, nil
	}
	var out OpenAIObjectRef
	err := s.db.QueryRowContext(ctx, `
SELECT object_type, object_id, user_id, token_id, selection_json, created_at, updated_at
FROM openai_object_refs
WHERE object_type=? AND object_id=?
`, objectType, objectID).Scan(&out.ObjectType, &out.ObjectID, &out.UserID, &out.TokenID, &out.SelectionJSON, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OpenAIObjectRef{}, false, nil
		}
		return OpenAIObjectRef{}, false, fmt.Errorf("查询 openai_object_refs 失败: %w", err)
	}
	return out, true, nil
}

func (s *Store) GetOpenAIObjectRefForUser(ctx context.Context, userID int64, objectType string, objectID string) (OpenAIObjectRef, bool, error) {
	if s == nil || s.db == nil {
		return OpenAIObjectRef{}, false, errors.New("store 未初始化")
	}
	if userID <= 0 {
		return OpenAIObjectRef{}, false, nil
	}
	objectType = strings.TrimSpace(objectType)
	objectID = strings.TrimSpace(objectID)
	if objectType == "" || objectID == "" {
		return OpenAIObjectRef{}, false, nil
	}
	var out OpenAIObjectRef
	err := s.db.QueryRowContext(ctx, `
SELECT object_type, object_id, user_id, token_id, selection_json, created_at, updated_at
FROM openai_object_refs
WHERE object_type=? AND object_id=? AND user_id=?
`, objectType, objectID, userID).Scan(&out.ObjectType, &out.ObjectID, &out.UserID, &out.TokenID, &out.SelectionJSON, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OpenAIObjectRef{}, false, nil
		}
		return OpenAIObjectRef{}, false, fmt.Errorf("查询 openai_object_refs 失败: %w", err)
	}
	return out, true, nil
}

func (s *Store) ListOpenAIObjectRefsByUser(ctx context.Context, userID int64, objectType string, limit int) ([]OpenAIObjectRef, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store 未初始化")
	}
	if userID <= 0 {
		return nil, nil
	}
	objectType = strings.TrimSpace(objectType)
	if objectType == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT object_type, object_id, user_id, token_id, selection_json, created_at, updated_at
FROM openai_object_refs
WHERE user_id=? AND object_type=?
ORDER BY created_at DESC
LIMIT ?
`, userID, objectType, limit)
	if err != nil {
		return nil, fmt.Errorf("查询 openai_object_refs 列表失败: %w", err)
	}
	defer rows.Close()

	var out []OpenAIObjectRef
	for rows.Next() {
		var v OpenAIObjectRef
		if err := rows.Scan(&v.ObjectType, &v.ObjectID, &v.UserID, &v.TokenID, &v.SelectionJSON, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 openai_object_refs 失败: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 openai_object_refs 失败: %w", err)
	}
	return out, nil
}

func (s *Store) DeleteOpenAIObjectRef(ctx context.Context, objectType string, objectID string) error {
	if s == nil || s.db == nil {
		return errors.New("store 未初始化")
	}
	objectType = strings.TrimSpace(objectType)
	objectID = strings.TrimSpace(objectID)
	if objectType == "" || objectID == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM openai_object_refs WHERE object_type=? AND object_id=?`, objectType, objectID); err != nil {
		return fmt.Errorf("删除 openai_object_refs 失败: %w", err)
	}
	return nil
}
