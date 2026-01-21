package admin

import (
	"strings"

	"realms/internal/store"
)

func mergeChannelGroupsOptions(groups []store.ChannelGroup, usedNames []string) []store.ChannelGroup {
	seen := make(map[string]struct{}, len(groups))
	out := make([]store.ChannelGroup, 0, len(groups))
	for _, g := range groups {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, g)
	}

	for _, name := range usedNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		desc := "未在“渠道分组”中注册"
		out = append(out, store.ChannelGroup{
			ID:              0,
			Name:            name,
			Description:     &desc,
			PriceMultiplier: store.DefaultGroupPriceMultiplier,
			MaxAttempts:     5,
			Status:          0,
		})
	}
	return out
}
