package metrics

import (
	"math"
	"strconv"
	"time"
)

// RatePerMinute returns count per minute over the given window.
func RatePerMinute(count int64, window time.Duration) float64 {
	minutes := window.Minutes()
	if minutes <= 0 {
		return 0
	}
	return float64(count) / minutes
}

// FormatRatePerMinute formats RatePerMinute as an integer string.
//
// RPM/TPM are quota-style "per-minute counts", so UI/output should stay integer.
func FormatRatePerMinute(count int64, window time.Duration) string {
	return strconv.FormatInt(int64(math.Round(RatePerMinute(count, window))), 10)
}
