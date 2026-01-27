package metrics

import (
	"fmt"
	"time"
)

// RatePerMinute returns count per minute over the given window.
//
// Note: RPM/TPM are rates, so they can be fractional (e.g. 1 request in 5 minutes => 0.2 RPM).
func RatePerMinute(count int64, window time.Duration) float64 {
	minutes := window.Minutes()
	if minutes <= 0 {
		return 0
	}
	return float64(count) / minutes
}

// FormatRatePerMinute formats RatePerMinute with 1 decimal place.
func FormatRatePerMinute(count int64, window time.Duration) string {
	return fmt.Sprintf("%.1f", RatePerMinute(count, window))
}
