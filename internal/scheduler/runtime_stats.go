package scheduler

import "time"

type RuntimeCredentialStats struct {
	RPM      int
	TPM      int
	Sessions int

	CoolingUntil *time.Time
	FailScore    int
}

type RuntimeChannelStats struct {
	FailScore int

	BannedUntil *time.Time
	BanStreak   int

	ForcedUntil *time.Time
}

func (s *Scheduler) RuntimeCredentialStats(credentialKey string) RuntimeCredentialStats {
	if s == nil || s.state == nil || credentialKey == "" {
		return RuntimeCredentialStats{}
	}

	now := time.Now()
	window := s.rpmWindow

	st := s.state
	st.mu.Lock()
	defer st.mu.Unlock()

	st.maybeSweepBindingsLocked(now)

	out := RuntimeCredentialStats{
		Sessions:  st.credentialSessions[credentialKey],
		FailScore: st.credFails[credentialKey],
	}

	// RPM: prune events outside window.
	rpmEvents := st.rpm[credentialKey]
	if len(rpmEvents) > 0 {
		cutoff := now.Add(-window)
		kept := rpmEvents[:0]
		for _, t := range rpmEvents {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		st.rpm[credentialKey] = kept
		out.RPM = len(kept)
	}

	// TPM: sum tokens inside window.
	tokenEvents := st.tokens[credentialKey]
	if len(tokenEvents) > 0 {
		cutoff := now.Add(-window)
		kept := tokenEvents[:0]
		total := 0
		for _, e := range tokenEvents {
			if e.time.After(cutoff) {
				kept = append(kept, e)
				total += e.tokens
			}
		}
		st.tokens[credentialKey] = kept
		out.TPM = total
	}

	if until, ok := st.credentialCooldown[credentialKey]; ok {
		if now.After(until) {
			delete(st.credentialCooldown, credentialKey)
		} else {
			u := until
			out.CoolingUntil = &u
		}
	}

	return out
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

	if st.forcedChannelID != 0 && !st.forcedChannelUntil.IsZero() && now.After(st.forcedChannelUntil) {
		st.forcedChannelID = 0
		st.forcedChannelUntil = time.Time{}
	}
	if st.forcedChannelID == channelID && !st.forcedChannelUntil.IsZero() {
		u := st.forcedChannelUntil
		out.ForcedUntil = &u
	}

	if until, ok := st.channelBanUntil[channelID]; ok {
		if now.After(until) {
			delete(st.channelBanUntil, channelID)
			delete(st.channelBanStreak, channelID)
		} else {
			u := until
			out.BannedUntil = &u
		}
	}

	return out
}
