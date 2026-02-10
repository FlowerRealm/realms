package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

func (s *Store) ListMainGroups(ctx context.Context) ([]MainGroup, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT name, description, status, created_at, updated_at
FROM main_groups
ORDER BY (name='default') DESC, status DESC, name ASC
`)
	if err != nil {
		return nil, fmt.Errorf("查询 main_groups 失败: %w", err)
	}
	defer rows.Close()

	var out []MainGroup
	for rows.Next() {
		var g MainGroup
		var desc sql.NullString
		if err := rows.Scan(&g.Name, &desc, &g.Status, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 main_groups 失败: %w", err)
		}
		if desc.Valid {
			v := strings.TrimSpace(desc.String)
			if v != "" {
				g.Description = &v
			}
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 main_groups 失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetMainGroupByName(ctx context.Context, name string) (MainGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return MainGroup{}, errors.New("name 不能为空")
	}

	var g MainGroup
	var desc sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT name, description, status, created_at, updated_at
FROM main_groups
WHERE name=?
LIMIT 1
`, name).Scan(&g.Name, &desc, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MainGroup{}, sql.ErrNoRows
		}
		return MainGroup{}, fmt.Errorf("查询 main_group 失败: %w", err)
	}
	if desc.Valid {
		v := strings.TrimSpace(desc.String)
		if v != "" {
			g.Description = &v
		}
	}
	return g, nil
}

func (s *Store) CreateMainGroup(ctx context.Context, name string, description *string, status int) error {
	name, err := normalizeGroupName(strings.TrimSpace(name))
	if err != nil {
		return err
	}
	if status != 0 && status != 1 {
		return errors.New("status 不合法")
	}

	var desc any
	if description != nil {
		v := strings.TrimSpace(*description)
		if v != "" {
			if len(v) > 255 {
				v = v[:255]
			}
			desc = v
		}
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO main_groups(name, description, status, created_at, updated_at)
VALUES(?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, name, desc, status)
	if err != nil {
		return fmt.Errorf("创建 main_group 失败: %w", err)
	}
	return nil
}

func (s *Store) UpdateMainGroup(ctx context.Context, name string, description *string, status int) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name 不能为空")
	}
	if status != 0 && status != 1 {
		return errors.New("status 不合法")
	}
	if name == DefaultGroupName && status != 1 {
		return errors.New("default 用户分组不允许禁用")
	}

	var desc any
	if description != nil {
		v := strings.TrimSpace(*description)
		if v != "" {
			if len(v) > 255 {
				v = v[:255]
			}
			desc = v
		}
	}

	_, err := s.db.ExecContext(ctx, `
UPDATE main_groups
SET description=?, status=?, updated_at=CURRENT_TIMESTAMP
WHERE name=?
`, desc, status, name)
	if err != nil {
		return fmt.Errorf("更新 main_group 失败: %w", err)
	}
	return nil
}

func (s *Store) DeleteMainGroup(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name 不能为空")
	}
	if name == DefaultGroupName {
		return errors.New("default 用户分组不允许删除")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `UPDATE users SET main_group=? WHERE main_group=?`, DefaultGroupName, name); err != nil {
		return fmt.Errorf("回退 users.main_group 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM main_group_subgroups WHERE main_group=?`, name); err != nil {
		return fmt.Errorf("删除 main_group_subgroups 失败: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM main_groups WHERE name=?`, name)
	if err != nil {
		return fmt.Errorf("删除 main_groups 失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func normalizeMainGroupSubgroups(subgroups []string) ([]string, error) {
	seen := make(map[string]struct{}, len(subgroups))
	out := make([]string, 0, len(subgroups))
	for _, raw := range subgroups {
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
	if len(out) > 50 {
		out = out[:50]
	}
	return out, nil
}

func (s *Store) ListMainGroupSubgroups(ctx context.Context, mainGroup string) ([]MainGroupSubgroup, error) {
	mainGroup = strings.TrimSpace(mainGroup)
	if mainGroup == "" {
		return nil, errors.New("main_group 不能为空")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT main_group, subgroup, priority, created_at, updated_at
FROM main_group_subgroups
WHERE main_group=?
ORDER BY priority DESC, subgroup ASC
`, mainGroup)
	if err != nil {
		return nil, fmt.Errorf("查询 main_group_subgroups 失败: %w", err)
	}
	defer rows.Close()

	var out []MainGroupSubgroup
	for rows.Next() {
		var row MainGroupSubgroup
		if err := rows.Scan(&row.MainGroup, &row.Subgroup, &row.Priority, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 main_group_subgroups 失败: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 main_group_subgroups 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ReplaceMainGroupSubgroups(ctx context.Context, mainGroup string, subgroups []string) error {
	mainGroup = strings.TrimSpace(mainGroup)
	if mainGroup == "" {
		return errors.New("main_group 不能为空")
	}
	if _, err := s.GetMainGroupByName(ctx, mainGroup); err != nil {
		return err
	}

	norm, err := normalizeMainGroupSubgroups(subgroups)
	if err != nil {
		return err
	}

	// 校验子组存在且可用（避免映射到不存在/禁用的 channel_group）。
	for _, sg := range norm {
		g, err := s.GetChannelGroupByName(ctx, sg)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("子组不存在: " + sg)
			}
			return err
		}
		if g.Status != 1 {
			return errors.New("子组已禁用: " + sg)
		}
	}

	// 允许显式顺序；priority 越大越靠前。
	type item struct {
		name string
	}
	items := make([]item, 0, len(norm))
	for _, n := range norm {
		items = append(items, item{name: n})
	}

	// 规范化：保证 default 始终可用（便于兜底）。
	hasDefault := false
	for _, it := range items {
		if it.name == DefaultGroupName {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		items = append(items, item{name: DefaultGroupName})
	}

	// 去重后保持稳定排序：先按输入顺序，追加 default 后置；再按 name 稳定。
	seen := map[string]struct{}{}
	dedup := make([]item, 0, len(items))
	for _, it := range items {
		if _, ok := seen[it.name]; ok {
			continue
		}
		seen[it.name] = struct{}{}
		dedup = append(dedup, it)
	}
	items = dedup

	// 使 priority 与顺序一致（不依赖旧值）。
	// 例：3 个元素 => 30, 20, 10
	prioBase := len(items) * 10
	priorityByName := make(map[string]int, len(items))
	for i, it := range items {
		priorityByName[it.name] = prioBase - i*10
	}
	// 仅用于写入前的 determinism（调试/测试）。
	sort.SliceStable(items, func(i, j int) bool { return priorityByName[items[i].name] > priorityByName[items[j].name] })

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM main_group_subgroups WHERE main_group=?`, mainGroup); err != nil {
		return fmt.Errorf("清理 main_group_subgroups 失败: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO main_group_subgroups(main_group, subgroup, priority, created_at, updated_at)
VALUES(?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`)
	if err != nil {
		return fmt.Errorf("准备写入 main_group_subgroups 失败: %w", err)
	}
	defer stmt.Close()

	for _, it := range items {
		if _, err := stmt.ExecContext(ctx, mainGroup, it.name, priorityByName[it.name]); err != nil {
			return fmt.Errorf("写入 main_group_subgroups 失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}
