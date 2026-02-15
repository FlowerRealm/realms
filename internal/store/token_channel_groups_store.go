package store

// token_channel_groups_store.go: Token 绑定渠道组（token_channel_groups）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

func (s *Store) ListTokenChannelGroupBindings(ctx context.Context, tokenID int64) ([]TokenChannelGroupBinding, error) {
	if tokenID <= 0 {
		return nil, errors.New("token_id 不合法")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT token_id, channel_group_name, priority, created_at, updated_at
FROM token_channel_groups
WHERE token_id=?
ORDER BY priority DESC, channel_group_name ASC
`, tokenID)
	if err != nil {
		return nil, fmt.Errorf("查询 token_channel_groups 失败: %w", err)
	}
	defer rows.Close()

	var out []TokenChannelGroupBinding
	for rows.Next() {
		var row TokenChannelGroupBinding
		if err := rows.Scan(&row.TokenID, &row.ChannelGroupName, &row.Priority, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 token_channel_groups 失败: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 token_channel_groups 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListTokenChannelGroups(ctx context.Context, tokenID int64) ([]string, error) {
	rows, err := s.ListTokenChannelGroupBindings(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(row.ChannelGroupName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out, nil
}

func normalizeTokenChannelGroups(in []string) ([]string, error) {
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
	if len(out) > 20 {
		out = out[:20]
	}
	return out, nil
}

func (s *Store) ReplaceTokenChannelGroups(ctx context.Context, tokenID int64, channelGroups []string) error {
	if tokenID <= 0 {
		return errors.New("token_id 不合法")
	}
	norm, err := normalizeTokenChannelGroups(channelGroups)
	if err != nil {
		return err
	}
	if len(norm) == 0 {
		return errors.New("至少选择 1 个渠道组")
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

	mainGroup := ""
	if userMainGroup.Valid {
		mainGroup = strings.TrimSpace(userMainGroup.String)
	}
	if mainGroup == "" {
		return errors.New("用户未配置用户分组")
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
		return errors.New("用户分组未配置可选渠道组")
	}

	// validate groups exist and enabled
	for _, g := range norm {
		if _, ok := allowedSet[g]; !ok {
			return errors.New("渠道组不在用户分组可选范围内: " + g)
		}
		row, err := s.GetChannelGroupByName(ctx, g)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("渠道组不存在: " + g)
			}
			return err
		}
		if row.Status != 1 {
			return errors.New("渠道组已禁用: " + g)
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

	if _, err := tx.ExecContext(ctx, `DELETE FROM token_channel_groups WHERE token_id=?`, tokenID); err != nil {
		return fmt.Errorf("清理 token_channel_groups 失败: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO token_channel_groups(token_id, channel_group_name, priority, created_at, updated_at)
VALUES(?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`)
	if err != nil {
		return fmt.Errorf("准备写入 token_channel_groups 失败: %w", err)
	}
	defer stmt.Close()

	for _, g := range ordered {
		if _, err := stmt.ExecContext(ctx, tokenID, g, priorityByName[g]); err != nil {
			return fmt.Errorf("写入 token_channel_groups 失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	if s != nil && s.tokenAuthCache != nil {
		s.tokenAuthCache.purgeTokenID(tokenID)
	}
	_ = s.BumpCacheInvalidation(ctx, CacheInvalidationKeyTokenAuth)
	return nil
}

func (s *Store) ListEffectiveTokenChannelGroupBindings(ctx context.Context, tokenID int64) ([]TokenChannelGroupBinding, error) {
	if tokenID <= 0 {
		return nil, errors.New("token_id 不合法")
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT
  t.id AS token_id,
  eb.channel_group_name,
  eb.priority,
  eb.created_at,
  eb.updated_at
FROM user_tokens t
JOIN users u ON u.id=t.user_id
LEFT JOIN (
  SELECT
    tcg.token_id,
    TRIM(tcg.channel_group_name) AS channel_group_name,
    tcg.priority,
    tcg.created_at,
    tcg.updated_at,
    mgs.main_group
  FROM token_channel_groups tcg
  JOIN channel_groups cg ON cg.name=TRIM(tcg.channel_group_name) AND cg.status=1
  JOIN main_group_subgroups mgs ON mgs.subgroup=TRIM(tcg.channel_group_name)
) eb ON eb.token_id=t.id AND eb.main_group=u.main_group
WHERE t.id=?
ORDER BY eb.priority DESC, eb.channel_group_name ASC
`, tokenID)
	if err != nil {
		return nil, fmt.Errorf("查询有效 token_channel_groups 失败: %w", err)
	}
	defer rows.Close()

	var out []TokenChannelGroupBinding
	foundToken := false
	for rows.Next() {
		foundToken = true

		var tokID int64
		var name sql.NullString
		var prio sql.NullInt64
		var createdAt sql.NullTime
		var updatedAt sql.NullTime
		if err := rows.Scan(&tokID, &name, &prio, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("扫描有效 token_channel_groups 失败: %w", err)
		}
		if !name.Valid {
			continue
		}
		n := strings.TrimSpace(name.String)
		if n == "" {
			continue
		}
		if !prio.Valid || !createdAt.Valid || !updatedAt.Valid {
			continue
		}
		out = append(out, TokenChannelGroupBinding{
			TokenID:          tokID,
			ChannelGroupName: n,
			Priority:         int(prio.Int64),
			CreatedAt:        createdAt.Time,
			UpdatedAt:        updatedAt.Time,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历有效 token_channel_groups 失败: %w", err)
	}
	if !foundToken {
		return nil, sql.ErrNoRows
	}
	return out, nil
}

func (s *Store) ListEffectiveTokenChannelGroups(ctx context.Context, tokenID int64) ([]string, error) {
	bindings, err := s.ListEffectiveTokenChannelGroupBindings(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(bindings))
	seen := make(map[string]struct{}, len(bindings))
	for _, b := range bindings {
		name := strings.TrimSpace(b.ChannelGroupName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out, nil
}
