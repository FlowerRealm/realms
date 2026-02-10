package store

import (
	"context"
	"fmt"
)

func (s *Store) ListActiveUsersByRole(ctx context.Context, role string) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  id, email, username, password_hash, role, main_group, status, created_at, updated_at
FROM users
WHERE role=? AND status=1
ORDER BY id DESC
`, role)
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
