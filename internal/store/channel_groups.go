package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

type ChannelGroupDeleteSummary struct {
	UsersUnbound     int64
	ChannelsUpdated  int64
	ChannelsDisabled int64
}

func (s *Store) ListChannelGroups(ctx context.Context) ([]ChannelGroup, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, description, price_multiplier, max_attempts, status, created_at, updated_at
FROM channel_groups
ORDER BY status DESC, name ASC, id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("查询 channel_groups 失败: %w", err)
	}
	defer rows.Close()

	var out []ChannelGroup
	for rows.Next() {
		var g ChannelGroup
		var desc sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &desc, &g.PriceMultiplier, &g.MaxAttempts, &g.Status, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 channel_groups 失败: %w", err)
		}
		if desc.Valid {
			v := desc.String
			g.Description = &v
		}
		if g.PriceMultiplier.IsNegative() {
			g.PriceMultiplier = DefaultGroupPriceMultiplier
		}
		if g.MaxAttempts <= 0 {
			g.MaxAttempts = 5
		}
		g.PriceMultiplier = g.PriceMultiplier.Truncate(PriceMultiplierScale)
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 channel_groups 失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetChannelGroupByID(ctx context.Context, id int64) (ChannelGroup, error) {
	var g ChannelGroup
	var desc sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, name, description, price_multiplier, max_attempts, status, created_at, updated_at
FROM channel_groups
WHERE id=?
`, id).Scan(&g.ID, &g.Name, &desc, &g.PriceMultiplier, &g.MaxAttempts, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ChannelGroup{}, sql.ErrNoRows
		}
		return ChannelGroup{}, fmt.Errorf("查询 channel_group 失败: %w", err)
	}
	if desc.Valid {
		v := desc.String
		g.Description = &v
	}
	if g.PriceMultiplier.IsNegative() {
		g.PriceMultiplier = DefaultGroupPriceMultiplier
	}
	if g.MaxAttempts <= 0 {
		g.MaxAttempts = 5
	}
	g.PriceMultiplier = g.PriceMultiplier.Truncate(PriceMultiplierScale)
	return g, nil
}

func (s *Store) GetChannelGroupByName(ctx context.Context, name string) (ChannelGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ChannelGroup{}, errors.New("name 不能为空")
	}
	var g ChannelGroup
	var desc sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, name, description, price_multiplier, max_attempts, status, created_at, updated_at
FROM channel_groups
WHERE name=?
LIMIT 1
`, name).Scan(&g.ID, &g.Name, &desc, &g.PriceMultiplier, &g.MaxAttempts, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ChannelGroup{}, sql.ErrNoRows
		}
		return ChannelGroup{}, fmt.Errorf("查询 channel_group 失败: %w", err)
	}
	if desc.Valid {
		v := desc.String
		g.Description = &v
	}
	if g.PriceMultiplier.IsNegative() {
		g.PriceMultiplier = DefaultGroupPriceMultiplier
	}
	if g.MaxAttempts <= 0 {
		g.MaxAttempts = 5
	}
	g.PriceMultiplier = g.PriceMultiplier.Truncate(PriceMultiplierScale)
	return g, nil
}

func (s *Store) CreateChannelGroup(ctx context.Context, name string, description *string, status int, priceMultiplier decimal.Decimal, maxAttempts int) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("name 不能为空")
	}
	if status != 0 && status != 1 {
		return 0, errors.New("status 不合法")
	}
	if priceMultiplier.IsNegative() {
		return 0, errors.New("price_multiplier 不合法")
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}

	var desc any
	if description != nil && strings.TrimSpace(*description) != "" {
		v := strings.TrimSpace(*description)
		if len(v) > 255 {
			v = v[:255]
		}
		desc = v
	}

	priceMultiplier = priceMultiplier.Truncate(PriceMultiplierScale)
	res, err := s.db.ExecContext(ctx, `
INSERT INTO channel_groups(name, description, price_multiplier, max_attempts, status, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, name, desc, priceMultiplier, maxAttempts, status)
	if err != nil {
		return 0, fmt.Errorf("创建 channel_group 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取 channel_group id 失败: %w", err)
	}
	return id, nil
}

func (s *Store) UpdateChannelGroup(ctx context.Context, id int64, description *string, status int, priceMultiplier decimal.Decimal, maxAttempts int) error {
	_, err := s.UpdateChannelGroupWithRename(ctx, id, nil, description, status, priceMultiplier, maxAttempts)
	return err
}

// UpdateChannelGroupWithRename updates a channel_group (description/status/price_multiplier/max_attempts) and optionally renames it (newName).
// When renamed, it also updates references in:
// - upstream_channels.groups
// - managed_models.group_name
// - subscription_plans.group_name
// - token_channel_groups.channel_group_name
// - main_group_subgroups.subgroup
//
// It returns the effective channel_group name (old or new).
func (s *Store) UpdateChannelGroupWithRename(ctx context.Context, id int64, newName *string, description *string, status int, priceMultiplier decimal.Decimal, maxAttempts int) (string, error) {
	if id == 0 {
		return "", errors.New("id 不能为空")
	}
	if status != 0 && status != 1 {
		return "", errors.New("status 不合法")
	}
	if priceMultiplier.IsNegative() {
		return "", errors.New("price_multiplier 不合法")
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}

	var desc any
	if description != nil && strings.TrimSpace(*description) != "" {
		v := strings.TrimSpace(*description)
		if len(v) > 255 {
			v = v[:255]
		}
		desc = v
	}

	g, err := s.GetChannelGroupByID(ctx, id)
	if err != nil {
		return "", err
	}
	oldName := strings.TrimSpace(g.Name)
	if oldName == "" {
		return "", errors.New("name 不能为空")
	}

	renameTo := ""
	if newName != nil {
		v := strings.TrimSpace(*newName)
		if v == "" {
			return "", errors.New("name 不能为空")
		}
		if v != oldName {
			norm, err := normalizeGroupName(v)
			if err != nil {
				return "", err
			}
			renameTo = norm
		}
	}

	clearDefault := false
	if status != 1 {
		defaultID, ok, err := s.GetInt64AppSetting(ctx, SettingDefaultChannelGroupID)
		if err != nil {
			return "", err
		}
		if ok && defaultID == id {
			clearDefault = true
		}
	}

	priceMultiplier = priceMultiplier.Truncate(PriceMultiplierScale)

	// No rename: fast path.
	if renameTo == "" && !clearDefault {
		_, err := s.db.ExecContext(ctx, `
UPDATE channel_groups
SET description=?, price_multiplier=?, max_attempts=?, status=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, desc, priceMultiplier, maxAttempts, status, id)
		if err != nil {
			return "", fmt.Errorf("更新 channel_group 失败: %w", err)
		}
		s.PurgeTokenAuthCacheAll()
		_ = s.BumpCacheInvalidation(ctx, CacheInvalidationKeyTokenAuth)
		_ = s.BumpCacheInvalidation(ctx, CacheInvalidationKeyUpstreamSnapshot)
		return oldName, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// No rename but need to clear default: update + delete app_settings atomically.
	if renameTo == "" {
		if _, err := tx.ExecContext(ctx, `
UPDATE channel_groups
SET description=?, price_multiplier=?, max_attempts=?, status=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, desc, priceMultiplier, maxAttempts, status, id); err != nil {
			return "", fmt.Errorf("更新 channel_group 失败: %w", err)
		}
		if clearDefault {
			if _, err := tx.ExecContext(ctx, "DELETE FROM app_settings WHERE `key`=? AND value=?", SettingDefaultChannelGroupID, strconv.FormatInt(id, 10)); err != nil {
				return "", fmt.Errorf("清理默认渠道组设置失败: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return "", fmt.Errorf("提交事务失败: %w", err)
		}
		s.PurgeTokenAuthCacheAll()
		_ = s.BumpCacheInvalidation(ctx, CacheInvalidationKeyTokenAuth)
		_ = s.BumpCacheInvalidation(ctx, CacheInvalidationKeyUpstreamSnapshot)
		return oldName, nil
	}

	qGroup := "SELECT name FROM channel_groups WHERE id=?" + forUpdateClause(s.dialect)
	var lockedOldName string
	if err := tx.QueryRowContext(ctx, qGroup, id).Scan(&lockedOldName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", sql.ErrNoRows
		}
		return "", fmt.Errorf("查询 channel_groups 失败: %w", err)
	}
	lockedOldName = strings.TrimSpace(lockedOldName)
	if lockedOldName == "" {
		return "", errors.New("name 不能为空")
	}

	var dup int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM channel_groups WHERE name=? AND id<>?`, renameTo, id).Scan(&dup); err != nil {
		return "", fmt.Errorf("查询 channel_groups 失败: %w", err)
	}
	if dup > 0 {
		return "", errors.New("渠道组名称已存在")
	}

	var tokenDup int64
	if err := tx.QueryRowContext(ctx, `
	SELECT COUNT(1)
	FROM token_channel_groups a
	JOIN token_channel_groups b ON b.token_id=a.token_id
	WHERE a.channel_group_name=? AND b.channel_group_name=?
	`, lockedOldName, renameTo).Scan(&tokenDup); err != nil {
		return "", fmt.Errorf("查询 token_channel_groups 失败: %w", err)
	}
	if tokenDup > 0 {
		return "", errors.New("目标名称已被占用（存在 Token 渠道组绑定冲突）")
	}

	var subgroupDup int64
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(1)
FROM main_group_subgroups a
JOIN main_group_subgroups b ON b.main_group=a.main_group
WHERE a.subgroup=? AND b.subgroup=?
`, lockedOldName, renameTo).Scan(&subgroupDup); err != nil {
		return "", fmt.Errorf("查询 main_group_subgroups 失败: %w", err)
	}
	if subgroupDup > 0 {
		return "", errors.New("目标名称已被占用（存在用户分组子组映射冲突）")
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE channel_groups
SET name=?, description=?, price_multiplier=?, max_attempts=?, status=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, renameTo, desc, priceMultiplier, maxAttempts, status, id); err != nil {
		return "", fmt.Errorf("更新 channel_group 失败: %w", err)
	}

	// Precise update: upstream_channels.groups (CSV) old -> new.
	rows, err := tx.QueryContext(ctx, "SELECT id, `groups` FROM upstream_channels")
	if err != nil {
		return "", fmt.Errorf("查询 upstream_channels.groups 失败: %w", err)
	}
	type chRow struct {
		id        int64
		groupsCSV string
	}
	var chans []chRow
	for rows.Next() {
		var row chRow
		if err := rows.Scan(&row.id, &row.groupsCSV); err != nil {
			_ = rows.Close()
			return "", fmt.Errorf("扫描 upstream_channels 失败: %w", err)
		}
		if csvHasExactGroup(row.groupsCSV, lockedOldName) {
			chans = append(chans, row)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return "", fmt.Errorf("遍历 upstream_channels 失败: %w", err)
	}
	_ = rows.Close()
	for _, ch := range chans {
		parts := splitUpstreamChannelGroupsCSV(ch.groupsCSV)
		out := make([]string, 0, len(parts))
		seen := make(map[string]struct{}, len(parts))
		for _, p := range parts {
			v := strings.TrimSpace(p)
			if v == "" {
				continue
			}
			if v == lockedOldName {
				v = renameTo
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
		newCSV := strings.Join(out, ",")
		if _, err := tx.ExecContext(ctx, "UPDATE upstream_channels SET `groups`=?, updated_at=CURRENT_TIMESTAMP WHERE id=?", newCSV, ch.id); err != nil {
			return "", fmt.Errorf("更新 upstream_channels.groups 失败: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE managed_models
SET group_name=?
WHERE group_name=?
`, renameTo, lockedOldName); err != nil {
		return "", fmt.Errorf("更新 managed_models.group_name 失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE subscription_plans
SET group_name=?, updated_at=CURRENT_TIMESTAMP
WHERE group_name=?
`, renameTo, lockedOldName); err != nil {
		return "", fmt.Errorf("更新 subscription_plans.group_name 失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
	UPDATE token_channel_groups
	SET channel_group_name=?, updated_at=CURRENT_TIMESTAMP
	WHERE channel_group_name=?
	`, renameTo, lockedOldName); err != nil {
		return "", fmt.Errorf("更新 token_channel_groups.channel_group_name 失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE main_group_subgroups
SET subgroup=?, updated_at=CURRENT_TIMESTAMP
WHERE subgroup=?
`, renameTo, lockedOldName); err != nil {
		return "", fmt.Errorf("更新 main_group_subgroups.subgroup 失败: %w", err)
	}

	if clearDefault {
		if _, err := tx.ExecContext(ctx, "DELETE FROM app_settings WHERE `key`=? AND value=?", SettingDefaultChannelGroupID, strconv.FormatInt(id, 10)); err != nil {
			return "", fmt.Errorf("清理默认渠道组设置失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("提交事务失败: %w", err)
	}
	s.PurgeTokenAuthCacheAll()
	_ = s.BumpCacheInvalidation(ctx, CacheInvalidationKeyTokenAuth)
	_ = s.BumpCacheInvalidation(ctx, CacheInvalidationKeyUpstreamSnapshot)
	return renameTo, nil
}

func (s *Store) DeleteChannelGroup(ctx context.Context, id int64) error {
	if id == 0 {
		return errors.New("id 不能为空")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM channel_groups WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("删除 channel_group 失败: %w", err)
	}
	return nil
}

func csvHasExactGroup(groupsCSV string, group string) bool {
	group = strings.TrimSpace(group)
	if group == "" {
		return false
	}
	for _, g := range splitUpstreamChannelGroupsCSV(groupsCSV) {
		if g == group {
			return true
		}
	}
	return false
}

func removeGroupFromCSV(groupsCSV string, group string) (string, bool) {
	group = strings.TrimSpace(group)
	if group == "" {
		return groupsCSV, false
	}
	groupsCSV = strings.TrimSpace(groupsCSV)
	if groupsCSV == "" {
		return "", false
	}
	parts := strings.Split(groupsCSV, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	changed := false
	for _, part := range parts {
		g := strings.TrimSpace(part)
		if g == "" {
			changed = true
			continue
		}
		if g == group {
			changed = true
			continue
		}
		if _, ok := seen[g]; ok {
			changed = true
			continue
		}
		seen[g] = struct{}{}
		out = append(out, g)
	}
	return strings.Join(out, ","), changed
}

// ForceDeleteChannelGroup 删除渠道组字典项，并级联清理引用：
// - upstream_channels.groups: 移除渠道 CSV 中的该 group（允许移除后为空）
func (s *Store) ForceDeleteChannelGroup(ctx context.Context, id int64) (ChannelGroupDeleteSummary, error) {
	if id == 0 {
		return ChannelGroupDeleteSummary{}, errors.New("id 不能为空")
	}

	clearDefault := false
	defaultID, ok, err := s.GetInt64AppSetting(ctx, SettingDefaultChannelGroupID)
	if err != nil {
		return ChannelGroupDeleteSummary{}, err
	}
	if ok && defaultID == id {
		clearDefault = true
	}

	g, err := s.GetChannelGroupByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ChannelGroupDeleteSummary{}, sql.ErrNoRows
		}
		return ChannelGroupDeleteSummary{}, err
	}
	group := strings.TrimSpace(g.Name)
	if group == "" {
		return ChannelGroupDeleteSummary{}, errors.New("group 不能为空")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ChannelGroupDeleteSummary{}, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var sum ChannelGroupDeleteSummary

	groupIDByName := make(map[string]int64)
	{
		rows, err := tx.QueryContext(ctx, `SELECT id, name FROM channel_groups`)
		if err != nil {
			return ChannelGroupDeleteSummary{}, fmt.Errorf("查询 channel_groups 失败: %w", err)
		}
		for rows.Next() {
			var id int64
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				_ = rows.Close()
				return ChannelGroupDeleteSummary{}, fmt.Errorf("扫描 channel_groups 失败: %w", err)
			}
			name = strings.TrimSpace(name)
			if id == 0 || name == "" {
				continue
			}
			groupIDByName[name] = id
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return ChannelGroupDeleteSummary{}, fmt.Errorf("遍历 channel_groups 失败: %w", err)
		}
		_ = rows.Close()
	}

	rows, err := tx.QueryContext(ctx, "SELECT id, `groups` FROM upstream_channels")
	if err != nil {
		return ChannelGroupDeleteSummary{}, fmt.Errorf("查询 upstream_channels.groups 失败: %w", err)
	}
	type chUpd struct {
		id        int64
		groupsCSV string
	}
	var chans []chUpd
	for rows.Next() {
		var row chUpd
		if err := rows.Scan(&row.id, &row.groupsCSV); err != nil {
			_ = rows.Close()
			return ChannelGroupDeleteSummary{}, fmt.Errorf("扫描 upstream_channels 失败: %w", err)
		}
		if csvHasExactGroup(row.groupsCSV, group) {
			chans = append(chans, row)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return ChannelGroupDeleteSummary{}, fmt.Errorf("遍历 upstream_channels 失败: %w", err)
	}
	_ = rows.Close()
	sum.ChannelsUpdated = int64(len(chans))

	for _, ch := range chans {
		newCSV, _ := removeGroupFromCSV(ch.groupsCSV, group)

		// 清理脏 groups：移除不存在的渠道组名（避免后续 SSOT 重建失败）。
		var kept []string
		for _, name := range splitUpstreamChannelGroupsCSV(newCSV) {
			name = strings.TrimSpace(name)
			if name == "" || name == group {
				continue
			}
			if _, ok := groupIDByName[name]; ok {
				kept = append(kept, name)
				continue
			}
		}
		newCSV = strings.Join(kept, ",")

		if _, err := tx.ExecContext(ctx, "UPDATE upstream_channels SET `groups`=?, updated_at=CURRENT_TIMESTAMP WHERE id=?", newCSV, ch.id); err != nil {
			return ChannelGroupDeleteSummary{}, fmt.Errorf("更新 upstream_channels.groups 失败: %w", err)
		}
	}

	// 清理与该渠道组相关的成员关系（作为父/作为子）。
	if _, err := tx.ExecContext(ctx, `DELETE FROM channel_group_members WHERE parent_group_id=? OR member_group_id=?`, id, id); err != nil {
		return ChannelGroupDeleteSummary{}, fmt.Errorf("清理 channel_group_members 失败: %w", err)
	}

	if clearDefault {
		if _, err := tx.ExecContext(ctx, "DELETE FROM app_settings WHERE `key`=? AND value=?", SettingDefaultChannelGroupID, strconv.FormatInt(id, 10)); err != nil {
			return ChannelGroupDeleteSummary{}, fmt.Errorf("清理默认渠道组设置失败: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM channel_groups WHERE id=?`, id); err != nil {
		return ChannelGroupDeleteSummary{}, fmt.Errorf("删除 channel_group 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ChannelGroupDeleteSummary{}, fmt.Errorf("提交事务失败: %w", err)
	}
	s.PurgeTokenAuthCacheAll()
	_ = s.BumpCacheInvalidation(ctx, CacheInvalidationKeyTokenAuth)
	_ = s.BumpCacheInvalidation(ctx, CacheInvalidationKeyUpstreamSnapshot)
	return sum, nil
}

func (s *Store) CountUpstreamChannelsByGroup(ctx context.Context, group string) (int64, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return 0, errors.New("group 不能为空")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT `groups` FROM upstream_channels")
	if err != nil {
		return 0, fmt.Errorf("统计 upstream_channels.groups 失败: %w", err)
	}
	defer rows.Close()

	var n int64
	for rows.Next() {
		var groupsCSV string
		if err := rows.Scan(&groupsCSV); err != nil {
			return 0, fmt.Errorf("统计 upstream_channels.groups 失败: %w", err)
		}
		if csvHasExactGroup(groupsCSV, group) {
			n++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("统计 upstream_channels.groups 失败: %w", err)
	}
	return n, nil
}
