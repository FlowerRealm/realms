package store

import (
	"errors"
	"fmt"
	"strings"
)

const DefaultGroupName = "default"

func normalizeGroupName(raw string) (string, error) {
	g := strings.TrimSpace(raw)
	if g == "" {
		return "", errors.New("分组名不能为空")
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
