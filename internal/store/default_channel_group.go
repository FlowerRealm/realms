package store

import (
	"context"
	"database/sql"
	"errors"
)

// EnsureDefaultChannelGroupID 返回默认渠道组（channel_groups.id）。
//
// 该值由管理员通过后台显式设置（app_settings[default_channel_group_id]）。
// 当未设置或设置无效（渠道组不存在/已禁用）时，将自动清理该设置并返回空。
func (s *Store) EnsureDefaultChannelGroupID(ctx context.Context) (int64, bool, error) {
	return s.GetDefaultChannelGroupID(ctx)
}

func (s *Store) GetDefaultChannelGroupID(ctx context.Context) (int64, bool, error) {
	id, ok, err := s.GetInt64AppSetting(ctx, SettingDefaultChannelGroupID)
	if err != nil {
		return 0, false, err
	}
	if !ok || id <= 0 {
		return 0, false, nil
	}
	g, err := s.GetChannelGroupByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = s.DeleteAppSetting(ctx, SettingDefaultChannelGroupID)
			return 0, false, nil
		}
		return 0, false, err
	}
	if g.Status != 1 {
		_ = s.DeleteAppSetting(ctx, SettingDefaultChannelGroupID)
		return 0, false, nil
	}
	return id, true, nil
}

func (s *Store) SetDefaultChannelGroupID(ctx context.Context, groupID int64) error {
	if groupID <= 0 {
		return errors.New("group_id 不合法")
	}
	g, err := s.GetChannelGroupByID(ctx, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("渠道组不存在")
		}
		return err
	}
	if g.Status != 1 {
		return errors.New("默认渠道组必须为启用状态")
	}
	return s.UpsertInt64AppSetting(ctx, SettingDefaultChannelGroupID, groupID)
}
