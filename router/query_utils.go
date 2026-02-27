package router

import (
	"strconv"
	"strings"
)

func queryBool(raw string) bool {
	v := strings.TrimSpace(raw)
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	return err == nil && b
}

