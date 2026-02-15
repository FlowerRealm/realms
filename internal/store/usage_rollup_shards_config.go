package store

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// usageRollupShardsFromEnv parses rollup sharding config.
//
// Safety: sharding is considered enabled only when BOTH env vars are set:
// - REALMS_USAGE_ROLLUP_SHARDS
// - REALMS_USAGE_ROLLUP_SHARDS_CUTOVER_AT (RFC3339 UTC)
//
// This avoids multi-instance inconsistencies where different instances would otherwise
// pick different implicit cutover timestamps.
func usageRollupShardsFromEnv() (shards int, cutoverAt time.Time) {
	rawN := strings.TrimSpace(os.Getenv("REALMS_USAGE_ROLLUP_SHARDS"))
	if rawN == "" {
		return 0, time.Time{}
	}
	n, err := strconv.Atoi(rawN)
	if err != nil || n <= 0 {
		return 0, time.Time{}
	}

	rawCutover := strings.TrimSpace(os.Getenv("REALMS_USAGE_ROLLUP_SHARDS_CUTOVER_AT"))
	if rawCutover == "" {
		return 0, time.Time{}
	}
	t, err := time.Parse(time.RFC3339, rawCutover)
	if err != nil {
		return 0, time.Time{}
	}
	return n, t.UTC()
}
