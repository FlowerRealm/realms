package admin

import (
	"fmt"
	"strings"
)

const defaultChannelGroup = "default"

func isDefaultGroup(name string) bool {
	return strings.TrimSpace(name) == defaultChannelGroup
}

func normalizeSingleGroup(raw string) (string, error) {
	g := strings.TrimSpace(raw)
	if g == "" {
		return defaultChannelGroup, nil
	}
	if len(g) > 64 {
		return "", fmt.Errorf("分组名过长（最多 64 字符）")
	}
	for _, r := range g {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			continue
		}
		return "", fmt.Errorf("分组名仅允许字母/数字/下划线/连字符")
	}
	return g, nil
}

func normalizeGroupsValues(rawValues []string) (string, error) {
	if len(rawValues) == 0 {
		return defaultChannelGroup, nil
	}

	var parts []string
	for _, raw := range rawValues {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts = append(parts, strings.Split(raw, ",")...)
	}
	if len(parts) == 0 {
		return defaultChannelGroup, nil
	}

	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		g, err := normalizeSingleGroup(part)
		if err != nil {
			return "", err
		}
		if g == "" {
			continue
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		out = append(out, g)
	}
	if len(out) == 0 {
		return defaultChannelGroup, nil
	}
	if len(out) > 20 {
		return "", fmt.Errorf("分组数量过多（最多 20 个）")
	}
	return strings.Join(out, ","), nil
}

func normalizeGroupsCSV(raw string) (string, error) {
	return normalizeGroupsValues([]string{raw})
}

// normalizeOptionalGroupsValues 与 normalizeGroupsValues 类似，但当输入为空时返回空字符串。
// 用于“可选配置项”（例如对话分组集合）：空表示未配置/关闭，而不是回退到 default。
func normalizeOptionalGroupsValues(rawValues []string) (string, error) {
	if len(rawValues) == 0 {
		return "", nil
	}

	var parts []string
	for _, raw := range rawValues {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts = append(parts, strings.Split(raw, ",")...)
	}
	if len(parts) == 0 {
		return "", nil
	}

	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		g, err := normalizeSingleGroup(part)
		if err != nil {
			return "", err
		}
		if g == "" {
			continue
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		out = append(out, g)
	}
	if len(out) == 0 {
		return "", nil
	}
	if len(out) > 20 {
		return "", fmt.Errorf("分组数量过多（最多 20 个）")
	}
	return strings.Join(out, ","), nil
}

func normalizeUserGroupsValues(rawValues []string) (string, error) {
	csv, err := normalizeGroupsValues(rawValues)
	if err != nil {
		return "", err
	}
	if !csvHasGroup(csv, defaultChannelGroup) {
		return normalizeGroupsValues([]string{csv, defaultChannelGroup})
	}
	return csv, nil
}

func splitGroups(groups string) []string {
	groups = strings.TrimSpace(groups)
	if groups == "" {
		return []string{defaultChannelGroup}
	}
	parts := strings.Split(groups, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{defaultChannelGroup}
	}
	return out
}

func splitOptionalGroups(groups string) []string {
	groups = strings.TrimSpace(groups)
	if groups == "" {
		return nil
	}
	parts := strings.Split(groups, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func csvHasGroup(groups string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, g := range splitGroups(groups) {
		if g == want {
			return true
		}
	}
	return false
}

// csvHasOptionalGroup 与 csvHasGroup 类似，但不会将空 CSV 视为 default。
// 用于“可选分组集合”（例如对话分组集合）：空表示未配置，而不是包含 default。
func csvHasOptionalGroup(groups string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, g := range splitOptionalGroups(groups) {
		if g == want {
			return true
		}
	}
	return false
}
