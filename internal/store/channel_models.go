// Package store 封装渠道-模型绑定（channel_models）的读写，用于“渠道绑定模型”模式下的白名单/alias/上游路由。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type ChannelModelCreate struct {
	ChannelID     int64
	PublicID      string
	UpstreamModel string
	Status        int
}

type ChannelModelUpdate struct {
	ID            int64
	ChannelID     int64
	PublicID      string
	UpstreamModel string
	Status        int
}

func (s *Store) ListChannelModelsByChannelID(ctx context.Context, channelID int64) ([]ChannelModel, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, channel_id, public_id, upstream_model, status, created_at, updated_at
FROM channel_models
WHERE channel_id=?
ORDER BY status DESC, id DESC
`, channelID)
	if err != nil {
		return nil, fmt.Errorf("查询 channel_models 失败: %w", err)
	}
	defer rows.Close()

	var out []ChannelModel
	for rows.Next() {
		var m ChannelModel
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.PublicID, &m.UpstreamModel, &m.Status, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 channel_models 失败: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 channel_models 失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetChannelModelByID(ctx context.Context, id int64) (ChannelModel, error) {
	var m ChannelModel
	err := s.db.QueryRowContext(ctx, `
SELECT id, channel_id, public_id, upstream_model, status, created_at, updated_at
FROM channel_models
WHERE id=?
`, id).Scan(&m.ID, &m.ChannelID, &m.PublicID, &m.UpstreamModel, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ChannelModel{}, sql.ErrNoRows
		}
		return ChannelModel{}, fmt.Errorf("查询 channel_model 失败: %w", err)
	}
	return m, nil
}

// ListEnabledChannelModelBindingsByPublicID 返回指定 public_id 的可用绑定：
// - channel_models.status=1
// - upstream_channels.status=1
func (s *Store) ListEnabledChannelModelBindingsByPublicID(ctx context.Context, publicID string) ([]ChannelModelBinding, error) {
	groupsCol := "`groups`"
	query := fmt.Sprintf(`
SELECT cm.id, cm.channel_id, ch.type, ch.%s, cm.public_id, cm.upstream_model, cm.status, cm.created_at, cm.updated_at
FROM channel_models cm
JOIN upstream_channels ch ON ch.id=cm.channel_id
WHERE cm.public_id=? AND cm.status=1 AND ch.status=1
ORDER BY cm.id DESC
`, groupsCol)
	rows, err := s.db.QueryContext(ctx, query, publicID)
	if err != nil {
		return nil, fmt.Errorf("查询 channel_models 失败: %w", err)
	}
	defer rows.Close()

	var out []ChannelModelBinding
	for rows.Next() {
		var b ChannelModelBinding
		if err := rows.Scan(&b.ID, &b.ChannelID, &b.ChannelType, &b.ChannelGroups, &b.PublicID, &b.UpstreamModel, &b.Status, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 channel_models 失败: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 channel_models 失败: %w", err)
	}
	return out, nil
}

func (s *Store) CreateChannelModel(ctx context.Context, in ChannelModelCreate) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO channel_models(channel_id, public_id, upstream_model, status, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, in.ChannelID, in.PublicID, in.UpstreamModel, in.Status)
	if err != nil {
		return 0, fmt.Errorf("创建 channel_model 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取 channel_model id 失败: %w", err)
	}
	return id, nil
}

func (s *Store) UpdateChannelModel(ctx context.Context, in ChannelModelUpdate) error {
	if in.ID == 0 {
		return errors.New("id 不能为空")
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE channel_models
SET channel_id=?, public_id=?, upstream_model=?, status=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, in.ChannelID, in.PublicID, in.UpstreamModel, in.Status, in.ID)
	if err != nil {
		return fmt.Errorf("更新 channel_model 失败: %w", err)
	}
	return nil
}

func (s *Store) DeleteChannelModel(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM channel_models WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("删除 channel_model 失败: %w", err)
	}
	return nil
}
