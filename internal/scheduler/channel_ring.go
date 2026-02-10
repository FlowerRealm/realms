package scheduler

import (
	"context"
	"errors"
	"sort"

	"realms/internal/store"
)

func buildPinnedChannelRing(ctx context.Context, st UpstreamStore) ([]int64, error) {
	if st == nil {
		return nil, errors.New("channel ring 未配置")
	}

	channels, err := st.ListUpstreamChannels(ctx)
	if err != nil {
		return nil, err
	}
	candidates := make([]store.UpstreamChannel, 0, len(channels))
	for _, ch := range channels {
		if ch.Status != 1 {
			continue
		}
		if ch.Type != store.UpstreamTypeOpenAICompatible && ch.Type != store.UpstreamTypeCodexOAuth && ch.Type != store.UpstreamTypeAnthropic {
			continue
		}
		candidates = append(candidates, ch)
	}
	if len(candidates) == 0 {
		return nil, errors.New("未配置可用上游 channel")
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Promotion != candidates[j].Promotion {
			return candidates[i].Promotion
		}
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		return candidates[i].ID > candidates[j].ID
	})

	out := make([]int64, 0, len(candidates))
	seen := make(map[int64]struct{}, len(candidates))
	for _, ch := range candidates {
		if ch.ID <= 0 {
			continue
		}
		if _, ok := seen[ch.ID]; ok {
			continue
		}
		seen[ch.ID] = struct{}{}
		out = append(out, ch.ID)
	}
	if len(out) == 0 {
		return nil, errors.New("未配置可用上游 channel")
	}
	return out, nil
}
