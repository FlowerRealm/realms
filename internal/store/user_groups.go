package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const DefaultGroupName = "default"

func normalizeGroupName(raw string) (string, error) {
	g := strings.TrimSpace(raw)
	if g == "" {
		return "", errors.New("分组名不能为空")
	}
	if len(g) > 64 {
		return "", fmt.Errorf("分组名过长（最多 64 字符）")
	}
	for _, r := range g {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			continue
		}
		return "", fmt.Errorf("分组名仅允许字母/数字/下划线/连字符")
	}
	return g, nil
}

func normalizeUserGroups(in []string) ([]string, error) {
	if len(in) == 0 {
		return []string{DefaultGroupName}, nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in)+1)
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		g, err := normalizeGroupName(raw)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		out = append(out, g)
	}
	if len(out) == 0 {
		out = append(out, DefaultGroupName)
	}
	if _, ok := seen[DefaultGroupName]; !ok {
		out = append(out, DefaultGroupName)
	}
	sort.Strings(out)
	if len(out) > 20 {
		return nil, fmt.Errorf("分组数量过多（最多 20 个）")
	}
	return out, nil
}

func splitGroupsCSV(groupsCSV string) []string {
	groupsCSV = strings.TrimSpace(groupsCSV)
	if groupsCSV == "" {
		return []string{DefaultGroupName}
	}
	parts := strings.Split(groupsCSV, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{DefaultGroupName}
	}
	norm, err := normalizeUserGroups(out)
	if err != nil {
		return []string{DefaultGroupName}
	}
	return norm
}

func groupsToCSV(groups []string) string {
	groups, err := normalizeUserGroups(groups)
	if err != nil {
		return DefaultGroupName
	}
	return strings.Join(groups, ",")
}

func (s *Store) ListUserGroups(ctx context.Context, userID int64) ([]string, error) {
	if userID == 0 {
		return nil, errors.New("userID 不能为空")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT group_name
FROM user_groups
WHERE user_id=?
ORDER BY group_name ASC
`, userID)
	if err != nil {
		return nil, fmt.Errorf("查询 user_groups 失败: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, fmt.Errorf("扫描 user_groups 失败: %w", err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 user_groups 失败: %w", err)
	}
	norm, err := normalizeUserGroups(out)
	if err != nil {
		return nil, err
	}
	return norm, nil
}

func (s *Store) ReplaceUserGroups(ctx context.Context, userID int64, groups []string) error {
	if userID == 0 {
		return errors.New("userID 不能为空")
	}
	groups, err := normalizeUserGroups(groups)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_groups WHERE user_id=?`, userID); err != nil {
		return fmt.Errorf("清理 user_groups 失败: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO user_groups(user_id, group_name, created_at) VALUES(?, ?, CURRENT_TIMESTAMP)`)
	if err != nil {
		return fmt.Errorf("准备写入 user_groups 失败: %w", err)
	}
	defer stmt.Close()

	for _, g := range groups {
		if _, err := stmt.ExecContext(ctx, userID, g); err != nil {
			return fmt.Errorf("写入 user_groups 失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) UserHasGroup(ctx context.Context, userID int64, groupName string) (bool, error) {
	if userID == 0 {
		return false, errors.New("userID 不能为空")
	}
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return false, errors.New("groupName 不能为空")
	}
	var n int64
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1)
FROM user_groups
WHERE user_id=? AND group_name=?
`, userID, groupName).Scan(&n); err != nil {
		return false, fmt.Errorf("查询 user_groups 失败: %w", err)
	}
	return n > 0, nil
}

func (s *Store) scanUserGroupsCSV(groupsCSV sql.NullString) []string {
	if !groupsCSV.Valid {
		return []string{DefaultGroupName}
	}
	return splitGroupsCSV(groupsCSV.String)
}
