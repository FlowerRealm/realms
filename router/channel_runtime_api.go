package router

import (
	"context"
	"time"
)

type channelRuntimeInfo struct {
	Available bool `json:"available"`

	FailScore int `json:"fail_score"`

	BannedUntil     string `json:"banned_until,omitempty"`
	BannedRemaining string `json:"banned_remaining,omitempty"`
	BanStreak       int    `json:"ban_streak"`
	BannedActive    bool   `json:"banned_active"`

	PinnedActive bool `json:"pinned_active"`
}

func channelRuntimeForAPI(ctx context.Context, opts Options, channelID int64, loc *time.Location) channelRuntimeInfo {
	out := channelRuntimeInfo{Available: opts.Sched != nil}
	if opts.Sched == nil || channelID <= 0 {
		return out
	}

	now := time.Now()
	rt := opts.Sched.RuntimeChannelStats(channelID)

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
