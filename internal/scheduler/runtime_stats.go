package scheduler

import "time"

type RuntimeBindingStats struct {
	MemoryHits int64
	StoreHits  int64
	Misses     int64

	Sets              int64
	SetBySelect       int64
	SetByTouch        int64
	SetByStoreRestore int64
	Refreshes         int64

	Clears            int64
	ClearExpired      int64
	ClearManual       int64
	ClearIneligible   int64
	ClearProbePending int64
	ClearParseError   int64

	StoreReadErrors   int64
	StoreWriteErrors  int64
	StoreDeleteErrors int64
}

type RuntimeChannelStats struct {
	FailScore int

	BannedUntil *time.Time
	BanStreak   int

	Pointer bool
}

func (s *Scheduler) RuntimeChannelStats(channelID int64) RuntimeChannelStats {
	if s == nil || s.state == nil || channelID == 0 {
		return RuntimeChannelStats{}
	}

	now := time.Now()
	st := s.state
	st.mu.Lock()
	defer st.mu.Unlock()

	out := RuntimeChannelStats{
		FailScore: st.channelFails[channelID],
		BanStreak: st.channelBanStreak[channelID],
		Pointer:   st.channelPointerID == channelID,
	}

	if until, ok := st.channelBanUntil[channelID]; ok {
		if now.After(until) {
			delete(st.channelBanUntil, channelID)
			delete(st.channelBanStreak, channelID)
			if _, probeOK := st.channelProbeDueAt[channelID]; !probeOK {
				st.channelProbeDueAt[channelID] = now
			}
			delete(st.channelProbeClaimUntil, channelID)
		} else {
			u := until
			out.BannedUntil = &u
		}
	}

	return out
}

func (s *Scheduler) RuntimeBindingStats() RuntimeBindingStats {
	if s == nil || s.state == nil {
		return RuntimeBindingStats{}
	}
	return s.state.BindingStatsSnapshot()
}
