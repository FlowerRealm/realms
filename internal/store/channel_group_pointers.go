package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ChannelGroupPointer 用于持久化调度器“分组内渠道指针”运行态（每组独立）。
//
// 约束：
// - 该结构仅用于运行态展示与跨实例同步，不作为业务强一致的调度依据。
// - moved_at_unix_ms 仅表示“指针发生变更”的时间戳（毫秒），不用于统计/计费。
type ChannelGroupPointer struct {
	GroupID       int64
	ChannelID     int64
	Pinned        bool
	MovedAtUnixMS int64
	Reason        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ChannelGroupPointerSnapshot 用于列表页展示每个分组当前指针（不包含 moved_at/reason 等扩展信息）。
type ChannelGroupPointerSnapshot struct {
	GroupID     int64
	ChannelID   int64
	ChannelName string
	Pinned      bool
}

func (p ChannelGroupPointer) MovedAt() time.Time {
	if p.MovedAtUnixMS <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(p.MovedAtUnixMS)
}

func (s *Store) GetChannelGroupPointerSnapshots(ctx context.Context, groupIDs []int64) (map[int64]ChannelGroupPointerSnapshot, error) {
	out := make(map[int64]ChannelGroupPointerSnapshot, len(groupIDs))
	if len(groupIDs) == 0 {
		return out, nil
	}
	if s.db == nil {
		return out, nil
	}

	var b strings.Builder
	b.WriteString(`
SELECT p.group_id, p.channel_id, p.pinned, COALESCE(TRIM(c.name), '')
FROM channel_group_pointers p
LEFT JOIN upstream_channels c ON c.id=p.channel_id
WHERE p.group_id IN (`)
	args := make([]any, 0, len(groupIDs))
	for i, id := range groupIDs {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("?")
		args = append(args, id)
	}
	b.WriteString(")")

	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("查询 channel_group_pointers 失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var v ChannelGroupPointerSnapshot
		var pinned int
		if err := rows.Scan(&v.GroupID, &v.ChannelID, &pinned, &v.ChannelName); err != nil {
			return nil, fmt.Errorf("扫描 channel_group_pointers 失败: %w", err)
		}
		v.Pinned = pinned != 0
		v.ChannelName = strings.TrimSpace(v.ChannelName)
		out[v.GroupID] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 channel_group_pointers 失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetChannelGroupPointer(ctx context.Context, groupID int64) (ChannelGroupPointer, bool, error) {
	if groupID <= 0 {
		return ChannelGroupPointer{}, false, errors.New("groupID 不合法")
	}
	if s.db == nil {
		return ChannelGroupPointer{}, false, nil
	}

	var out ChannelGroupPointer
	var pinned int
	err := s.db.QueryRowContext(ctx, `
SELECT
  group_id, channel_id, pinned, moved_at_unix_ms, reason, created_at, updated_at
FROM channel_group_pointers
WHERE group_id=?
LIMIT 1
`, groupID).Scan(&out.GroupID, &out.ChannelID, &pinned, &out.MovedAtUnixMS, &out.Reason, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ChannelGroupPointer{}, false, nil
		}
		return ChannelGroupPointer{}, false, fmt.Errorf("查询 channel_group_pointers 失败: %w", err)
	}
	out.Pinned = pinned != 0
	out.Reason = strings.TrimSpace(out.Reason)
	return out, true, nil
}

func (s *Store) UpsertChannelGroupPointer(ctx context.Context, in ChannelGroupPointer) error {
	if s.db == nil {
		return errors.New("db 为空")
	}
	if in.GroupID <= 0 {
		return errors.New("group_id 不合法")
	}
	reason := strings.TrimSpace(in.Reason)
	p := 0
	if in.Pinned {
		p = 1
	}

	stmt := `
INSERT INTO channel_group_pointers(group_id, channel_id, pinned, moved_at_unix_ms, reason, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
  channel_id=VALUES(channel_id),
  pinned=VALUES(pinned),
  moved_at_unix_ms=VALUES(moved_at_unix_ms),
  reason=VALUES(reason),
  updated_at=CURRENT_TIMESTAMP
`
	if s.dialect == DialectSQLite {
		stmt = `
INSERT INTO channel_group_pointers(group_id, channel_id, pinned, moved_at_unix_ms, reason, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(group_id) DO UPDATE SET
  channel_id=excluded.channel_id,
  pinned=excluded.pinned,
  moved_at_unix_ms=excluded.moved_at_unix_ms,
  reason=excluded.reason,
  updated_at=CURRENT_TIMESTAMP
`
	}

	if _, err := s.db.ExecContext(ctx, stmt, in.GroupID, in.ChannelID, p, in.MovedAtUnixMS, reason); err != nil {
		return fmt.Errorf("写入 channel_group_pointers 失败: %w", err)
	}
	return nil
}
