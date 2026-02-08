package metrics_test

import (
	"math"
	"testing"
	"time"

	"realms/internal/metrics"
)

func TestRatePerMinute(t *testing.T) {
	t.Run("fractional_rate", func(t *testing.T) {
		got := metrics.RatePerMinute(1, 5*time.Minute)
		if math.Abs(got-0.2) > 1e-9 {
			t.Fatalf("expected 0.2, got=%v", got)
		}
	})

	t.Run("zero_window", func(t *testing.T) {
		if got := metrics.RatePerMinute(10, 0); got != 0 {
			t.Fatalf("expected 0, got=%v", got)
		}
	})

	t.Run("negative_window", func(t *testing.T) {
		if got := metrics.RatePerMinute(10, -time.Minute); got != 0 {
			t.Fatalf("expected 0, got=%v", got)
		}
	})
}

func TestFormatRatePerMinute(t *testing.T) {
	t.Run("round_to_integer", func(t *testing.T) {
		if got := metrics.FormatRatePerMinute(3, 2*time.Minute); got != "2" {
			t.Fatalf("expected %q, got=%q", "2", got)
		}
	})

	t.Run("fractional_below_half", func(t *testing.T) {
		if got := metrics.FormatRatePerMinute(1, 5*time.Minute); got != "0" {
			t.Fatalf("expected %q, got=%q", "0", got)
		}
	})

	t.Run("zero", func(t *testing.T) {
		if got := metrics.FormatRatePerMinute(0, time.Minute); got != "0" {
			t.Fatalf("expected %q, got=%q", "0", got)
		}
	})
}
