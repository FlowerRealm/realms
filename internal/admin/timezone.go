package admin

import (
	"context"
	"strings"
	"time"

	"realms/internal/store"
)

const defaultAdminTimeZone = "Asia/Shanghai"

func normalizeAdminTimeZoneName(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if strings.EqualFold(v, "utc") {
		return "UTC"
	}
	return v
}

func loadAdminLocation(name string) (*time.Location, error) {
	n := normalizeAdminTimeZoneName(name)
	if n == "" {
		n = defaultAdminTimeZone
	}
	switch n {
	case "UTC":
		return time.UTC, nil
	case defaultAdminTimeZone:
		if loc, err := time.LoadLocation(defaultAdminTimeZone); err == nil {
			return loc, nil
		}
		// 某些最小化镜像可能缺少 tzdata；为确保“默认北京时间”可用，回退到固定 +08:00。
		return time.FixedZone("CST", 8*60*60), nil
	default:
		return time.LoadLocation(n)
	}
}

func (s *Server) adminTimeLocation(ctx context.Context) (*time.Location, string) {
	defaultName := defaultAdminTimeZone
	if s != nil && strings.TrimSpace(s.adminTimeZoneDefault) != "" {
		defaultName = s.adminTimeZoneDefault
	}

	name := defaultName
	if s != nil && s.st != nil {
		if v, ok, err := s.st.GetStringAppSetting(ctx, store.SettingAdminTimeZone); err == nil && ok {
			if vv := normalizeAdminTimeZoneName(v); vv != "" {
				name = vv
			}
		}
	}

	loc, err := loadAdminLocation(name)
	if err != nil || loc == nil {
		name = defaultName
		loc, _ = loadAdminLocation(name)
	}
	if loc == nil {
		loc = time.UTC
	}
	return loc, name
}

func formatTimeIn(t time.Time, layout string, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format(layout)
}

func formatTimePtrIn(t *time.Time, layout string, loc *time.Location) string {
	if t == nil {
		return "-"
	}
	return formatTimeIn(*t, layout, loc)
}
