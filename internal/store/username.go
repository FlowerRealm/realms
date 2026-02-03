package store

import (
	"fmt"
	"regexp"
	"strings"
)

var usernameRE = regexp.MustCompile(`^[A-Za-z0-9]+$`)

func NormalizeUsername(raw string) (string, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", fmt.Errorf("账号名不能为空")
	}

	if len(u) > 64 {
		return "", fmt.Errorf("账号名长度不能超过 64 位")
	}
	if !usernameRE.MatchString(u) {
		return "", fmt.Errorf("账号名仅支持字母/数字（区分大小写），不允许空格或特殊字符")
	}
	return u, nil
}
