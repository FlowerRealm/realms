package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ChannelGroupMemberDetail struct {
	MemberID      int64
	ParentGroupID int64

	MemberGroupID          *int64
	MemberGroupName        *string
	MemberGroupStatus      *int
	MemberGroupMaxAttempts *int

	MemberChannelID     *int64
	MemberChannelName   *string
	MemberChannelType   *string
	MemberChannelGroups *string
	MemberChannelStatus *int

	Priority  int
	Promotion bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *Store) ListChannelGroupMembers(ctx context.Context, parentGroupID int64) ([]ChannelGroupMemberDetail, error) {
	if parentGroupID == 0 {
		return nil, errors.New("parentGroupID 不能为空")
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT\n"+
			"  m.id, m.parent_group_id, m.member_group_id, cg.name, cg.status, cg.max_attempts,\n"+
			"  m.member_channel_id, uc.name, uc.type, uc.`groups`, uc.status,\n"+
			"  m.priority, m.promotion, m.created_at, m.updated_at\n"+
			"FROM channel_group_members m\n"+
			"LEFT JOIN channel_groups cg ON cg.id=m.member_group_id\n"+
			"LEFT JOIN upstream_channels uc ON uc.id=m.member_channel_id\n"+
			"WHERE m.parent_group_id=?\n"+
			"ORDER BY m.promotion DESC, m.priority DESC, m.id DESC\n",
		parentGroupID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询 channel_group_members 失败: %w", err)
	}
	defer rows.Close()

	var out []ChannelGroupMemberDetail
	for rows.Next() {
		var row ChannelGroupMemberDetail
		var memberGroupID sql.NullInt64
		var memberGroupName sql.NullString
		var memberGroupStatus sql.NullInt64
		var memberGroupMaxAttempts sql.NullInt64
		var memberChannelID sql.NullInt64
		var memberChannelName sql.NullString
		var memberChannelType sql.NullString
		var memberChannelGroups sql.NullString
		var memberChannelStatus sql.NullInt64
		var promotion int
		if err := rows.Scan(
			&row.MemberID,
			&row.ParentGroupID,
			&memberGroupID,
			&memberGroupName,
			&memberGroupStatus,
			&memberGroupMaxAttempts,
			&memberChannelID,
			&memberChannelName,
			&memberChannelType,
			&memberChannelGroups,
			&memberChannelStatus,
			&row.Priority,
			&promotion,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描 channel_group_members 失败: %w", err)
		}
		row.Promotion = promotion != 0
		if memberGroupID.Valid {
			v := memberGroupID.Int64
			row.MemberGroupID = &v
		}
		if memberGroupName.Valid {
			v := memberGroupName.String
			row.MemberGroupName = &v
		}
		if memberGroupStatus.Valid {
			v := int(memberGroupStatus.Int64)
			row.MemberGroupStatus = &v
		}
		if memberGroupMaxAttempts.Valid {
			v := int(memberGroupMaxAttempts.Int64)
			row.MemberGroupMaxAttempts = &v
		}
		if memberChannelID.Valid {
			v := memberChannelID.Int64
			row.MemberChannelID = &v
		}
		if memberChannelName.Valid {
			v := memberChannelName.String
			row.MemberChannelName = &v
		}
		if memberChannelType.Valid {
			v := memberChannelType.String
			row.MemberChannelType = &v
		}
		if memberChannelGroups.Valid {
			v := memberChannelGroups.String
			row.MemberChannelGroups = &v
		}
		if memberChannelStatus.Valid {
			v := int(memberChannelStatus.Int64)
			row.MemberChannelStatus = &v
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 channel_group_members 失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetChannelGroupParentID(ctx context.Context, groupID int64) (int64, bool, error) {
	if groupID == 0 {
		return 0, false, errors.New("groupID 不能为空")
	}
	var parentID int64
	err := s.db.QueryRowContext(ctx, `SELECT parent_group_id FROM channel_group_members WHERE member_group_id=? LIMIT 1`, groupID).Scan(&parentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("查询父分组失败: %w", err)
	}
	if parentID == 0 {
		return 0, false, nil
	}
	return parentID, true, nil
}

func (s *Store) AddChannelGroupMemberGroup(ctx context.Context, parentGroupID int64, memberGroupID int64, priority int, promotion bool) error {
	if parentGroupID == 0 {
		return errors.New("parentGroupID 不能为空")
	}
	if memberGroupID == 0 {
		return errors.New("memberGroupID 不能为空")
	}
	if parentGroupID == memberGroupID {
		return errors.New("不允许将分组添加为自身的子组")
	}

	parent, err := s.GetChannelGroupByID(ctx, parentGroupID)
	if err != nil {
		return err
	}
	child, err := s.GetChannelGroupByID(ctx, memberGroupID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(child.Name) == DefaultGroupName {
		return errors.New("default 分组不允许作为子组")
	}
	if strings.TrimSpace(parent.Name) == "" || strings.TrimSpace(child.Name) == "" {
		return errors.New("分组名不能为空")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 子组单父校验：若已有父组且不是当前 parent，则拒绝（避免 silent move）。
	var existingParentID int64
	err = tx.QueryRowContext(ctx, `SELECT parent_group_id FROM channel_group_members WHERE member_group_id=? LIMIT 1`, memberGroupID).Scan(&existingParentID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("查询子组父级失败: %w", err)
	}
	if err == nil && existingParentID != 0 && existingParentID != parentGroupID {
		return errors.New("子组只能归属一个父组（请先从原父组移除）")
	}

	// 防环：从 parent 向上追溯父链，若遇到 child 则形成环。
	cur := parentGroupID
	for i := 0; i < 512; i++ {
		var pid sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT parent_group_id FROM channel_group_members WHERE member_group_id=? LIMIT 1`, cur).Scan(&pid); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			return fmt.Errorf("防环检测失败: %w", err)
		}
		if !pid.Valid || pid.Int64 == 0 {
			break
		}
		if pid.Int64 == memberGroupID {
			return errors.New("检测到环：不允许将该分组加入当前父组")
		}
		cur = pid.Int64
	}

	p := 0
	if promotion {
		p = 1
	}
	stmt := `
INSERT INTO channel_group_members(parent_group_id, member_group_id, priority, promotion, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE priority=VALUES(priority), promotion=VALUES(promotion), updated_at=CURRENT_TIMESTAMP
`
	if s.dialect == DialectSQLite {
		stmt = `
INSERT INTO channel_group_members(parent_group_id, member_group_id, priority, promotion, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(parent_group_id, member_group_id) DO UPDATE SET priority=excluded.priority, promotion=excluded.promotion, updated_at=CURRENT_TIMESTAMP
`
	}
	_, err = tx.ExecContext(ctx, stmt, parentGroupID, memberGroupID, priority, p)
	if err != nil {
		return fmt.Errorf("写入子组成员失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) AddChannelGroupMemberChannel(ctx context.Context, parentGroupID int64, channelID int64, priority int, promotion bool) error {
	if parentGroupID == 0 {
		return errors.New("parentGroupID 不能为空")
	}
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}
	if _, err := s.GetChannelGroupByID(ctx, parentGroupID); err != nil {
		return err
	}
	ch, err := s.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	p := 0
	if promotion {
		p = 1
	}
	stmt := `
INSERT INTO channel_group_members(parent_group_id, member_channel_id, priority, promotion, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE priority=VALUES(priority), promotion=VALUES(promotion), updated_at=CURRENT_TIMESTAMP
`
	if s.dialect == DialectSQLite {
		stmt = `
INSERT INTO channel_group_members(parent_group_id, member_channel_id, priority, promotion, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(parent_group_id, member_channel_id) DO UPDATE SET priority=excluded.priority, promotion=excluded.promotion, updated_at=CURRENT_TIMESTAMP
`
	}
	_, err = tx.ExecContext(ctx, stmt, parentGroupID, channelID, priority, p)
	if err != nil {
		return fmt.Errorf("写入渠道成员失败: %w", err)
	}

	if err := s.syncUpstreamChannelGroupsCacheTx(ctx, tx, ch.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) RemoveChannelGroupMemberGroup(ctx context.Context, parentGroupID int64, memberGroupID int64) error {
	if parentGroupID == 0 {
		return errors.New("parentGroupID 不能为空")
	}
	if memberGroupID == 0 {
		return errors.New("memberGroupID 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM channel_group_members WHERE parent_group_id=? AND member_group_id=?`, parentGroupID, memberGroupID)
	if err != nil {
		return fmt.Errorf("删除子组成员失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) RemoveChannelGroupMemberChannel(ctx context.Context, parentGroupID int64, channelID int64) error {
	if parentGroupID == 0 {
		return errors.New("parentGroupID 不能为空")
	}
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM channel_group_members WHERE parent_group_id=? AND member_channel_id=?`, parentGroupID, channelID)
	if err != nil {
		return fmt.Errorf("删除渠道成员失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if err := s.syncUpstreamChannelGroupsCacheTx(ctx, tx, channelID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) ReorderChannelGroupMembers(ctx context.Context, parentGroupID int64, memberIDs []int64) error {
	if parentGroupID == 0 {
		return errors.New("parentGroupID 不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	count := len(memberIDs)
	for i, id := range memberIDs {
		if id == 0 {
			continue
		}
		priority := count - i
		res, err := tx.ExecContext(ctx, `UPDATE channel_group_members SET priority=?, updated_at=CURRENT_TIMESTAMP WHERE id=? AND parent_group_id=?`, priority, id, parentGroupID)
		if err != nil {
			return fmt.Errorf("更新成员(%d) priority 失败: %w", id, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("成员(%d) 不属于该分组", id)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) syncUpstreamChannelGroupsCacheTx(ctx context.Context, tx *sql.Tx, channelID int64) error {
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}

	rows, err := tx.QueryContext(ctx, `
SELECT cg.name
FROM channel_group_members m
JOIN channel_groups cg ON cg.id=m.parent_group_id
WHERE m.member_channel_id=?
ORDER BY (cg.name='default') DESC, cg.name ASC, cg.id DESC
`, channelID)
	if err != nil {
		return fmt.Errorf("查询渠道所属分组失败: %w", err)
	}
	defer rows.Close()

	var names []string
	seen := make(map[string]struct{})
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return fmt.Errorf("扫描渠道所属分组失败: %w", err)
		}
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历渠道所属分组失败: %w", err)
	}

	if len(names) == 0 {
		// 兼容历史语义：空分组视为 default。
		names = append(names, DefaultGroupName)

		var defaultGroupID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM channel_groups WHERE name='default' LIMIT 1`).Scan(&defaultGroupID); err == nil && defaultGroupID > 0 {
			stmt := fmt.Sprintf(`
%s INTO channel_group_members(parent_group_id, member_channel_id, priority, promotion, created_at, updated_at)
VALUES(?, ?, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, insertIgnoreVerb(s.dialect))
			_, _ = tx.ExecContext(ctx, stmt, defaultGroupID, channelID)
		}
	}

	csv := strings.Join(names, ",")
	if _, err := tx.ExecContext(ctx, "UPDATE upstream_channels SET `groups`=?, updated_at=CURRENT_TIMESTAMP WHERE id=?", csv, channelID); err != nil {
		return fmt.Errorf("回填 upstream_channels.groups 失败: %w", err)
	}
	return nil
}
