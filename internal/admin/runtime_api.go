package admin

import (
	"context"
	"time"
)

// ChannelRuntimeInfo 提供上游渠道的运行态信息（由 scheduler 维护），用于 SPA/new-api。
type ChannelRuntimeInfo struct {
	Available bool `json:"available"`

	FailScore int `json:"fail_score"`

	BannedUntil     string `json:"banned_until,omitempty"`
	BannedRemaining string `json:"banned_remaining,omitempty"`
	BanStreak       int    `json:"ban_streak"`
	BannedActive    bool   `json:"banned_active"`

	PinnedActive bool `json:"pinned_active"`
}

func (s *Server) ChannelRuntimeForAPI(ctx context.Context, channelID int64) ChannelRuntimeInfo {
	out := ChannelRuntimeInfo{Available: s != nil && s.sched != nil}
	if s == nil || s.sched == nil || channelID <= 0 {
		return out
	}

	loc, _ := s.adminTimeLocation(ctx)
	now := time.Now()

	rt := s.sched.RuntimeChannelStats(channelID)
	out.FailScore = rt.FailScore
	out.BanStreak = rt.BanStreak
	out.PinnedActive = rt.Pointer
	if rt.BannedUntil != nil {
		out.BannedActive = true
		out.BannedUntil = formatTimeIn(*rt.BannedUntil, time.RFC3339, loc)
		out.BannedRemaining = formatRemainingUntilZH(*rt.BannedUntil, now)
	}
	return out
}
