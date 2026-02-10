package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

func (s *Store) ListTokenGroupBindings(ctx context.Context, tokenID int64) ([]TokenGroupBinding, error) {
	if tokenID <= 0 {
		return nil, errors.New("token_id 不合法")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT token_id, group_name, priority, created_at, updated_at
FROM token_groups
WHERE token_id=?
ORDER BY priority DESC, group_name ASC
`, tokenID)
	if err != nil {
		return nil, fmt.Errorf("查询 token_groups 失败: %w", err)
	}
	defer rows.Close()

	var out []TokenGroupBinding
	for rows.Next() {
		var row TokenGroupBinding
		if err := rows.Scan(&row.TokenID, &row.GroupName, &row.Priority, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 token_groups 失败: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 token_groups 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListTokenGroups(ctx context.Context, tokenID int64) ([]string, error) {
	rows, err := s.ListTokenGroupBindings(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(row.GroupName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		out = append(out, DefaultGroupName)
	}
	return out, nil
}

func normalizeTokenGroups(in []string) ([]string, error) {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		name, err := normalizeGroupName(raw)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		out = append(out, DefaultGroupName)
	}
	if _, ok := seen[DefaultGroupName]; !ok {
		out = append(out, DefaultGroupName)
	}
	if len(out) > 20 {
		keep := make([]string, 0, 20)
		for _, g := range out {
			if g == DefaultGroupName {
				continue
			}
			keep = append(keep, g)
			if len(keep) >= 19 {
				break
			}
		}
		out = append(keep, DefaultGroupName)
	}
	return out, nil
}

func (s *Store) ReplaceTokenGroups(ctx context.Context, tokenID int64, groups []string) error {
	if tokenID <= 0 {
		return errors.New("token_id 不合法")
	}
	norm, err := normalizeTokenGroups(groups)
	if err != nil {
		return err
	}

	// ensure token exists and load its owner's main_group (用于限制可选范围)
	var tokID int64
	var userMainGroup sql.NullString
	if err := s.db.QueryRowContext(ctx, `
SELECT t.id, u.main_group
FROM user_tokens t
JOIN users u ON u.id=t.user_id
WHERE t.id=?
LIMIT 1
`, tokenID).Scan(&tokID, &userMainGroup); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("查询 token 失败: %w", err)
	}

	mainGroup := DefaultGroupName
	if userMainGroup.Valid && strings.TrimSpace(userMainGroup.String) != "" {
		mainGroup = strings.TrimSpace(userMainGroup.String)
	}
	allowed, err := s.ListMainGroupSubgroups(ctx, mainGroup)
	if err != nil {
		return err
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, row := range allowed {
		name := strings.TrimSpace(row.Subgroup)
		if name == "" {
			continue
		}
		allowedSet[name] = struct{}{}
	}
	if len(allowedSet) == 0 {
		allowedSet[DefaultGroupName] = struct{}{}
	}

	// validate groups exist and enabled
	for _, g := range norm {
		if _, ok := allowedSet[g]; !ok {
			return errors.New("分组不在用户分组可选范围内: " + g)
		}
		row, err := s.GetChannelGroupByName(ctx, g)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("分组不存在: " + g)
			}
			return err
		}
		if row.Status != 1 {
			return errors.New("分组已禁用: " + g)
		}
	}

	prioBase := len(norm) * 10
	priorityByName := make(map[string]int, len(norm))
	for i, g := range norm {
		priorityByName[g] = prioBase - i*10
	}
	ordered := append([]string(nil), norm...)
	sort.SliceStable(ordered, func(i, j int) bool { return priorityByName[ordered[i]] > priorityByName[ordered[j]] })

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM token_groups WHERE token_id=?`, tokenID); err != nil {
		return fmt.Errorf("清理 token_groups 失败: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO token_groups(token_id, group_name, priority, created_at, updated_at)
VALUES(?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`)
	if err != nil {
		return fmt.Errorf("准备写入 token_groups 失败: %w", err)
	}
	defer stmt.Close()

	for _, g := range ordered {
		if _, err := stmt.ExecContext(ctx, tokenID, g, priorityByName[g]); err != nil {
			return fmt.Errorf("写入 token_groups 失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) ListEffectiveTokenGroupBindings(ctx context.Context, tokenID int64) ([]TokenGroupBinding, error) {
	if tokenID <= 0 {
		return nil, errors.New("token_id 不合法")
	}

	var mainGroup sql.NullString
	if err := s.db.QueryRowContext(ctx, `
SELECT u.main_group
FROM user_tokens t
JOIN users u ON u.id=t.user_id
WHERE t.id=?
LIMIT 1
	`, tokenID).Scan(&mainGroup); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("查询 token 用户分组失败: %w", err)
	}

	mainGroupName := DefaultGroupName
	if mainGroup.Valid && strings.TrimSpace(mainGroup.String) != "" {
		mainGroupName = strings.TrimSpace(mainGroup.String)
	}
	allowed, err := s.ListMainGroupSubgroups(ctx, mainGroupName)
	if err != nil {
		return nil, err
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, row := range allowed {
		name := strings.TrimSpace(row.Subgroup)
		if name == "" {
			continue
		}
		allowedSet[name] = struct{}{}
	}
	if len(allowedSet) == 0 {
		allowedSet[DefaultGroupName] = struct{}{}
	}

	bindings, err := s.ListTokenGroupBindings(ctx, tokenID)
	if err != nil {
		return nil, err
	}

	out := make([]TokenGroupBinding, 0, len(bindings))
	for _, b := range bindings {
		name := strings.TrimSpace(b.GroupName)
		if name == "" {
			continue
		}
		if _, ok := allowedSet[name]; !ok {
			continue
		}
		g, err := s.GetChannelGroupByName(ctx, name)
		if err != nil {
			continue
		}
		if g.Status != 1 {
			continue
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		out = append(out, TokenGroupBinding{
			TokenID:   tokenID,
			GroupName: DefaultGroupName,
			Priority:  0,
		})
	}
	return out, nil
}

func (s *Store) ListEffectiveTokenGroups(ctx context.Context, tokenID int64) ([]string, error) {
	bindings, err := s.ListEffectiveTokenGroupBindings(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(bindings))
	seen := make(map[string]struct{}, len(bindings))
	for _, b := range bindings {
		name := strings.TrimSpace(b.GroupName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		out = append(out, DefaultGroupName)
	}
	return out, nil
}
