package store

import (
	"context"
	"fmt"
	"strings"
)

func (s *Store) ListActiveUsersByRole(ctx context.Context, role string) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  id, email, username, password_hash, role, status, created_at, updated_at
FROM users
WHERE role=? AND status=1
ORDER BY id DESC
`, role)
	if err != nil {
		return nil, fmt.Errorf("查询 users 失败: %w", err)
	}
	defer rows.Close()

	var out []User
	var userIDs []int64
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 users 失败: %w", err)
		}
		out = append(out, u)
		userIDs = append(userIDs, u.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 users 失败: %w", err)
	}

	if len(out) == 0 {
		return out, nil
	}

	var b strings.Builder
	b.WriteString("SELECT user_id, group_name FROM user_groups WHERE user_id IN (")
	for i := range userIDs {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("?")
	}
	b.WriteString(") ORDER BY user_id ASC, group_name ASC")
	args := make([]any, 0, len(userIDs))
	for _, id := range userIDs {
		args = append(args, id)
	}
	gRows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("查询 user_groups 失败: %w", err)
	}
	defer gRows.Close()

	groupsByUserID := make(map[int64][]string, len(out))
	for gRows.Next() {
		var userID int64
		var groupName string
		if err := gRows.Scan(&userID, &groupName); err != nil {
			return nil, fmt.Errorf("扫描 user_groups 失败: %w", err)
		}
		groupsByUserID[userID] = append(groupsByUserID[userID], groupName)
	}
	if err := gRows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 user_groups 失败: %w", err)
	}

	for i := range out {
		groups := groupsByUserID[out[i].ID]
		norm, err := normalizeUserGroups(groups)
		if err != nil {
			out[i].Groups = []string{DefaultGroupName}
			continue
		}
		out[i].Groups = norm
	}
	return out, nil
}
