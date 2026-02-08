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

type bindingRuntimeInfo struct {
	Available bool `json:"available"`

	MemoryHits int64 `json:"memory_hits"`
	StoreHits  int64 `json:"store_hits"`
	Misses     int64 `json:"misses"`

	Sets              int64 `json:"sets"`
	SetBySelect       int64 `json:"set_by_select"`
	SetByTouch        int64 `json:"set_by_touch"`
	SetByStoreRestore int64 `json:"set_by_store_restore"`
	Refreshes         int64 `json:"refreshes"`
	Clears            int64 `json:"clears"`
	ClearExpired      int64 `json:"clear_expired"`
	ClearManual       int64 `json:"clear_manual"`
	ClearIneligible   int64 `json:"clear_ineligible"`
	ClearProbePending int64 `json:"clear_probe_pending"`
	ClearParseError   int64 `json:"clear_parse_error"`
	StoreReadErrors   int64 `json:"store_read_errors"`
	StoreWriteErrors  int64 `json:"store_write_errors"`
	StoreDeleteErrors int64 `json:"store_delete_errors"`
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

func bindingRuntimeForAPI(opts Options) bindingRuntimeInfo {
	out := bindingRuntimeInfo{Available: opts.Sched != nil}
	if opts.Sched == nil {
		return out
	}
	rt := opts.Sched.RuntimeBindingStats()
	out.MemoryHits = rt.MemoryHits
	out.StoreHits = rt.StoreHits
	out.Misses = rt.Misses
	out.Sets = rt.Sets
	out.SetBySelect = rt.SetBySelect
	out.SetByTouch = rt.SetByTouch
	out.SetByStoreRestore = rt.SetByStoreRestore
	out.Refreshes = rt.Refreshes
	out.Clears = rt.Clears
	out.ClearExpired = rt.ClearExpired
	out.ClearManual = rt.ClearManual
	out.ClearIneligible = rt.ClearIneligible
	out.ClearProbePending = rt.ClearProbePending
	out.ClearParseError = rt.ClearParseError
	out.StoreReadErrors = rt.StoreReadErrors
	out.StoreWriteErrors = rt.StoreWriteErrors
	out.StoreDeleteErrors = rt.StoreDeleteErrors
	return out
}
