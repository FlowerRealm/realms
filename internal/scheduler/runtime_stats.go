package scheduler

import "time"

type RuntimeChannelStats struct {
	FailScore int

	BannedUntil *time.Time
	BanStreak   int
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
