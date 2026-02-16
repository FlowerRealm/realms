package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (s *Store) ListMainGroups(ctx context.Context) ([]MainGroup, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT name, description, status, created_at, updated_at
FROM main_groups
ORDER BY status DESC, name ASC
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
	_, err := s.UpdateMainGroupWithRename(ctx, name, nil, description, status)
	return err
}

// UpdateMainGroupWithRename updates a main_group (description/status) and optionally renames it (newName).
// When renamed, it also updates references in:
// - users.main_group
// - main_group_subgroups.main_group
//
// It returns the effective main_group name (old or new).
func (s *Store) UpdateMainGroupWithRename(ctx context.Context, name string, newName *string, description *string, status int) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("name 不能为空")
	}
	if status != 0 && status != 1 {
		return "", errors.New("status 不合法")
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

	renameTo := ""
	if newName != nil {
		v := strings.TrimSpace(*newName)
		if v == "" {
			return "", errors.New("new_name 不能为空")
		}
		if v != name {
			norm, err := normalizeGroupName(v)
			if err != nil {
				return "", err
			}
			renameTo = norm
		}
	}

	// No rename: fast path.
	if renameTo == "" {
		res, err := s.db.ExecContext(ctx, `
UPDATE main_groups
SET description=?, status=?, updated_at=CURRENT_TIMESTAMP
WHERE name=?
`, desc, status, name)
		if err != nil {
			return "", fmt.Errorf("更新 main_group 失败: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return "", sql.ErrNoRows
		}
		return name, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM main_groups WHERE name=?`, name).Scan(&exists); err != nil {
		return "", fmt.Errorf("查询 main_groups 失败: %w", err)
	}
	if exists == 0 {
		return "", sql.ErrNoRows
	}

	var dup int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM main_groups WHERE name=?`, renameTo).Scan(&dup); err != nil {
		return "", fmt.Errorf("查询 main_groups 失败: %w", err)
	}
	if dup > 0 {
		return "", errors.New("用户分组名称已存在")
	}

	var subgroupDup int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM main_group_subgroups WHERE main_group=?`, renameTo).Scan(&subgroupDup); err != nil {
		return "", fmt.Errorf("查询 main_group_subgroups 失败: %w", err)
	}
	if subgroupDup > 0 {
		return "", errors.New("目标名称已被占用（存在子组映射）")
	}

	res, err := tx.ExecContext(ctx, `
UPDATE main_groups
SET name=?, description=?, status=?, updated_at=CURRENT_TIMESTAMP
WHERE name=?
`, renameTo, desc, status, name)
	if err != nil {
		return "", fmt.Errorf("更新 main_group 失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", sql.ErrNoRows
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE users
SET main_group=?, updated_at=CURRENT_TIMESTAMP
WHERE main_group=?
`, renameTo, name); err != nil {
		return "", fmt.Errorf("更新 users.main_group 失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE main_group_subgroups
SET main_group=?, updated_at=CURRENT_TIMESTAMP
WHERE main_group=?
`, renameTo, name); err != nil {
		return "", fmt.Errorf("更新 main_group_subgroups.main_group 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("提交事务失败: %w", err)
	}
	return renameTo, nil
}

func (s *Store) DeleteMainGroup(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name 不能为空")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var nUsers int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM users WHERE main_group=?`, name).Scan(&nUsers); err != nil {
		return fmt.Errorf("检查 users.main_group 引用失败: %w", err)
	}
	if nUsers > 0 {
		return errors.New("该用户分组仍有关联用户，请先迁移用户到其他用户分组")
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
ORDER BY subgroup ASC
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

	for _, name := range norm {
		if _, err := stmt.ExecContext(ctx, mainGroup, name, 0); err != nil {
			return fmt.Errorf("写入 main_group_subgroups 失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}
