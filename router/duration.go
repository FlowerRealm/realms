package router

import (
	"fmt"
	"strings"
	"time"
)

func formatDurationShortZH(d time.Duration) string {
	if d <= 0 {
		return "0秒"
	}
	d = d.Round(time.Second)
	if d < time.Second {
		return "0秒"
	}

	sec := int64(d.Seconds())
	days := sec / (24 * 60 * 60)
	sec %= 24 * 60 * 60
	hours := sec / (60 * 60)
	sec %= 60 * 60
	mins := sec / 60
	sec %= 60

	// 最多显示 2 个单位，避免过长。
	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d天", days))
	}
	if hours > 0 && len(parts) < 2 {
		parts = append(parts, fmt.Sprintf("%d小时", hours))
	}
	if mins > 0 && len(parts) < 2 {
		parts = append(parts, fmt.Sprintf("%d分", mins))
	}
	if sec > 0 && len(parts) < 2 {
		parts = append(parts, fmt.Sprintf("%d秒", sec))
	}
	if len(parts) == 0 {
		return "0秒"
	}
	return strings.Join(parts, "")
}

func formatRemainingUntilZH(until time.Time, now time.Time) string {
	return formatDurationShortZH(until.Sub(now))
}

