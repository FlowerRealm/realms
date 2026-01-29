package scheduler

import (
	"context"
	"database/sql"
	"errors"

	"realms/internal/store"
)

func buildDefaultChannelRing(ctx context.Context, st ChannelGroupStore) ([]int64, error) {
	if st == nil {
		return nil, errors.New("channel ring 未配置")
	}

	root, err := st.GetChannelGroupByName(ctx, store.DefaultGroupName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("default 分组不存在")
		}
		return nil, err
	}
	if root.Status != 1 {
		return nil, errors.New("default 分组已禁用")
	}

	seenGroups := map[int64]struct{}{}
	seenChannels := map[int64]struct{}{}
	out := make([]int64, 0, 32)

	var walk func(groupID int64) error
	walk = func(groupID int64) error {
		if groupID == 0 {
			return nil
		}
		if _, ok := seenGroups[groupID]; ok {
			return nil
		}
		seenGroups[groupID] = struct{}{}

		g, err := st.GetChannelGroupByID(ctx, groupID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}
		if g.Status != 1 {
			return nil
		}

		ms, err := st.ListChannelGroupMembers(ctx, groupID)
		if err != nil {
			return err
		}
		for _, m := range ms {
			if m.MemberChannelID != nil {
				chID := *m.MemberChannelID
				if chID <= 0 {
					continue
				}
				if m.MemberChannelStatus != nil && *m.MemberChannelStatus != 1 {
					continue
				}
				if _, ok := seenChannels[chID]; ok {
					continue
				}
				seenChannels[chID] = struct{}{}
				out = append(out, chID)
				continue
			}
			if m.MemberGroupID != nil {
				gid := *m.MemberGroupID
				if gid <= 0 {
					continue
				}
				if m.MemberGroupStatus != nil && *m.MemberGroupStatus != 1 {
					continue
				}
				if err := walk(gid); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := walk(root.ID); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("default 分组未配置可用上游 channel")
	}
	return out, nil
}
