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

	// channel_group_members（SQLite）首次引入时做最小自举：
	// - 创建表与索引（若不存在）
	// - 以 upstream_channels.groups（兼容缓存）为来源，为每个 channel 写入“组 -> 渠道”成员关系。
	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS channel_group_members (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  parent_group_id INTEGER NOT NULL,
  member_group_id INTEGER NULL,
  member_channel_id INTEGER NULL,
  priority INTEGER NOT NULL DEFAULT 0,
  promotion INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
)
`); err != nil {
		return fmt.Errorf("创建 channel_group_members 表失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS uk_parent_member_group ON channel_group_members(parent_group_id, member_group_id)`); err != nil {
		return fmt.Errorf("创建 channel_group_members 索引失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS uk_parent_member_channel ON channel_group_members(parent_group_id, member_channel_id)`); err != nil {
		return fmt.Errorf("创建 channel_group_members 索引失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS uk_group_single_parent ON channel_group_members(member_group_id)`); err != nil {
		return fmt.Errorf("创建 channel_group_members 索引失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_parent_order ON channel_group_members(parent_group_id, promotion, priority, id)`); err != nil {
		return fmt.Errorf("创建 channel_group_members 索引失败: %w", err)
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
