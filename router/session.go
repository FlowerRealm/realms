package router

import (
	"strconv"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func sessionUserID(c *gin.Context) (int64, bool) {
	return sessionInt64(c, "id")
}

func sessionInt64(c *gin.Context, key string) (int64, bool) {
	if c == nil {
		return 0, false
	}
	v := sessions.Default(c).Get(key)
	switch x := v.(type) {
	case int64:
		if x <= 0 {
			return 0, false
		}
		return x, true
	case int:
		if x <= 0 {
			return 0, false
		}
		return int64(x), true
	case float64:
		if x <= 0 {
			return 0, false
		}
		return int64(x), true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		if err != nil || n <= 0 {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}
