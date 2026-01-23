package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func ensureSQLiteChannelGroupMembers(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 SQLite 分组自举事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var defaultGroupID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM channel_groups WHERE name=? LIMIT 1`, DefaultGroupName).Scan(&defaultGroupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("default 分组不存在")
		}
		return fmt.Errorf("查询 default 分组失败: %w", err)
	}
	if defaultGroupID == 0 {
		return errors.New("default 分组不存在")
	}

	// 将“无父级”的分组默认挂到 default 根组（单父约束：已存在父级则忽略）。
	if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO channel_group_members(parent_group_id, member_group_id, priority, promotion, created_at, updated_at)
SELECT d.id, g.id, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM channel_groups d, channel_groups g
WHERE d.name=? AND g.name<>?
  AND NOT EXISTS (SELECT 1 FROM channel_group_members m WHERE m.member_group_id=g.id)
`, DefaultGroupName, DefaultGroupName); err != nil {
		return fmt.Errorf("回填 default 子组失败: %w", err)
	}

	// 以 upstream_channels.groups（兼容缓存）为来源，为每个 channel 写入“组 -> 渠道”成员关系。
	rows, err := tx.QueryContext(ctx, `SELECT id, name FROM channel_groups`)
	if err != nil {
		return fmt.Errorf("查询 channel_groups 失败: %w", err)
	}
	groupIDByName := make(map[string]int64)
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			_ = rows.Close()
			return fmt.Errorf("扫描 channel_groups 失败: %w", err)
		}
		name = strings.TrimSpace(name)
		if id == 0 || name == "" {
			continue
		}
		groupIDByName[name] = id
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("遍历 channel_groups 失败: %w", err)
	}
	_ = rows.Close()

	upsertMember := `
INSERT INTO channel_group_members(parent_group_id, member_channel_id, priority, promotion, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(parent_group_id, member_channel_id) DO UPDATE SET priority=excluded.priority, promotion=excluded.promotion, updated_at=CURRENT_TIMESTAMP
`

	chRows, err := tx.QueryContext(ctx, "SELECT id, `groups`, priority, promotion FROM upstream_channels")
	if err != nil {
		return fmt.Errorf("查询 upstream_channels 失败: %w", err)
	}
	defer chRows.Close()

	for chRows.Next() {
		var channelID int64
		var groupsCSV string
		var priority int
		var promotion int
		if err := chRows.Scan(&channelID, &groupsCSV, &priority, &promotion); err != nil {
			return fmt.Errorf("扫描 upstream_channels 失败: %w", err)
		}
		if channelID == 0 {
			continue
		}
		var hasAny int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM channel_group_members WHERE member_channel_id=? LIMIT 1`, channelID).Scan(&hasAny); err == nil {
			continue
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("检查 channel_group_members 失败: %w", err)
		}
		for _, name := range splitUpstreamChannelGroupsCSV(groupsCSV) {
			gid := groupIDByName[name]
			if gid == 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx, upsertMember, gid, channelID, priority, promotion); err != nil {
				return fmt.Errorf("写入 channel_group_members 失败: %w", err)
			}
		}
	}
	if err := chRows.Err(); err != nil {
		return fmt.Errorf("遍历 upstream_channels 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite 分组自举事务失败: %w", err)
	}
	return nil
}
