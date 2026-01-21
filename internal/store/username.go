package store

import (
	"fmt"
	"regexp"
	"strings"
)

var usernameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func NormalizeUsername(raw string) (string, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", fmt.Errorf("账号名不能为空")
	}
	u = strings.ToLower(u)

	if len(u) > 32 {
		return "", fmt.Errorf("账号名长度不能超过 32 位")
	}
	if !usernameRE.MatchString(u) {
		return "", fmt.Errorf("账号名仅支持字母/数字及 . _ -，且需以字母或数字开头")
	}
	return u, nil
}
