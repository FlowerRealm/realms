package store

import "time"

type usageRollupHourSegment struct {
	Since   time.Time
	Until   time.Time
	Sharded bool
}

func (s *Store) usageHourRollupSegments(since, until time.Time) []usageRollupHourSegment {
	if s == nil || s.usageRollupShards <= 0 || s.usageRollupShardsCutoverAt.IsZero() {
		return []usageRollupHourSegment{{Since: since, Until: until, Sharded: false}}
	}
	cutover := s.usageRollupShardsCutoverAt
	if !until.After(cutover) {
		return []usageRollupHourSegment{{Since: since, Until: until, Sharded: false}}
	}
	if !since.Before(cutover) {
		return []usageRollupHourSegment{{Since: since, Until: until, Sharded: true}}
	}
	return []usageRollupHourSegment{
		{Since: since, Until: cutover, Sharded: false},
		{Since: cutover, Until: until, Sharded: true},
	}
}
