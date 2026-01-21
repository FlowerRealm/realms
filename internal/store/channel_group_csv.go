package store

import (
	"strings"
)

// splitUpstreamChannelGroupsCSV 解析 upstream_channels.groups（CSV）。
//
// 兼容性：
// - 空字符串视为 default
// - 去重、去空白项
// - 不强制包含 default（仅当为空时回退）
func splitUpstreamChannelGroupsCSV(groupsCSV string) []string {
	groupsCSV = strings.TrimSpace(groupsCSV)
	if groupsCSV == "" {
		return []string{DefaultGroupName}
	}
	parts := strings.Split(groupsCSV, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{DefaultGroupName}
	}
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}
