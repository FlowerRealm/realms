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
}

type channelModelRuntimeInfo struct {
	Available bool `json:"available"`

	FailScore int `json:"fail_score"`

	BannedUntil     string `json:"banned_until,omitempty"`
	BannedRemaining string `json:"banned_remaining,omitempty"`
	BanStreak       int    `json:"ban_streak"`
	BannedActive    bool   `json:"banned_active"`
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
	if rt.BannedUntil != nil {
		out.BannedActive = true
		out.BannedUntil = formatTimeIn(*rt.BannedUntil, time.RFC3339, loc)
		out.BannedRemaining = formatRemainingUntilZH(*rt.BannedUntil, now)
	}

	return out
}

func channelModelRuntimeForAPI(ctx context.Context, opts Options, bindingID int64, loc *time.Location) channelModelRuntimeInfo {
	out := channelModelRuntimeInfo{Available: opts.Sched != nil}
	if opts.Sched == nil || bindingID <= 0 {
		return out
	}

	now := time.Now()
	rt := opts.Sched.RuntimeChannelModelStats(bindingID)

	out.FailScore = rt.FailScore
	out.BanStreak = rt.BanStreak
	if rt.BannedUntil != nil {
		out.BannedActive = true
		out.BannedUntil = formatTimeIn(*rt.BannedUntil, time.RFC3339, loc)
		out.BannedRemaining = formatRemainingUntilZH(*rt.BannedUntil, now)
	}

	return out
}
