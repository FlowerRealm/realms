package store

import (
	"context"
	"database/sql"
	"fmt"
)

func (s *Store) ListActiveUsersByRole(ctx context.Context, role string) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  id, email, username, password_hash, role, status, created_at, updated_at,
  (SELECT GROUP_CONCAT(group_name ORDER BY group_name SEPARATOR ',') FROM user_groups ug WHERE ug.user_id=users.id) AS groups_csv
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
		var groupsCSV sql.NullString
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt, &groupsCSV); err != nil {
			return nil, fmt.Errorf("扫描 users 失败: %w", err)
		}
		u.Groups = s.scanUserGroupsCSV(groupsCSV)
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 users 失败: %w", err)
	}
	return out, nil
}
