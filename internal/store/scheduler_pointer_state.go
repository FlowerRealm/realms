package store

import (
	"encoding/json"
	"strings"
	"time"
)

// SchedulerChannelPointerState 用于在 app_settings 中持久化调度器“渠道指针”运行态（跨实例共享）。
//
// 约束：
// - 该结构仅用于运行态展示与跨实例同步，不作为业务强一致的调度依据。
// - moved_at_unix_ms 仅表示“指针发生变更”的时间戳（毫秒），不用于统计/计费。
type SchedulerChannelPointerState struct {
	V             int    `json:"v"`
	ChannelID     int64  `json:"channel_id"`
	Pinned        bool   `json:"pinned"`
	MovedAtUnixMS int64  `json:"moved_at_unix_ms"`
	Reason        string `json:"reason"`
}

func (s SchedulerChannelPointerState) MovedAt() time.Time {
	if s.MovedAtUnixMS <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(s.MovedAtUnixMS)
}

func (s SchedulerChannelPointerState) Marshal() (string, error) {
	out := s
	if out.V <= 0 {
		out.V = 1
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func ParseSchedulerChannelPointerState(raw string) (SchedulerChannelPointerState, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SchedulerChannelPointerState{}, false, nil
	}
	var out SchedulerChannelPointerState
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return SchedulerChannelPointerState{}, false, err
	}
	if out.V <= 0 {
		out.V = 1
	}
	out.Reason = strings.TrimSpace(out.Reason)
	return out, true, nil
}
