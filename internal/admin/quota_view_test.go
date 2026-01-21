package admin

import (
	"testing"
	"time"
)

func TestFormatQuotaWindowDetail(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	reset := time.Date(2026, 1, 17, 0, 0, 0, 0, time.UTC)

	t.Run("nil used percent", func(t *testing.T) {
		if got := formatQuotaWindowDetail(nil, &reset, 6.0, loc); got != "-" {
			t.Fatalf("expected '-', got %q", got)
		}
	})

	t.Run("0 percent used", func(t *testing.T) {
		used := 0
		got := formatQuotaWindowDetail(&used, &reset, 6.0, loc)
		want := "剩余 $6.00/$6.00 · 重置 2026-01-17T08:00:00+08:00"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("50 percent used", func(t *testing.T) {
		used := 50
		got := formatQuotaWindowDetail(&used, &reset, 6.0, loc)
		want := "剩余 $3.00/$6.00 · 重置 2026-01-17T08:00:00+08:00"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("100 percent used", func(t *testing.T) {
		used := 100
		got := formatQuotaWindowDetail(&used, &reset, 6.0, loc)
		want := "剩余 $0.00/$6.00 · 重置 2026-01-17T08:00:00+08:00"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("out of range used percent clamps", func(t *testing.T) {
		usedTooLow := -10
		if got := formatQuotaWindowDetail(&usedTooLow, &reset, 6.0, loc); got != "剩余 $6.00/$6.00 · 重置 2026-01-17T08:00:00+08:00" {
			t.Fatalf("unexpected clamped low result: %q", got)
		}

		usedTooHigh := 150
		if got := formatQuotaWindowDetail(&usedTooHigh, &reset, 6.0, loc); got != "剩余 $0.00/$6.00 · 重置 2026-01-17T08:00:00+08:00" {
			t.Fatalf("unexpected clamped high result: %q", got)
		}
	})

	t.Run("nil reset at", func(t *testing.T) {
		used := 50
		got := formatQuotaWindowDetail(&used, nil, 6.0, loc)
		want := "剩余 $3.00/$6.00"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})
}

func TestFormatDaysLeftSeconds(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		if got := formatDaysLeftSeconds(-1); got != "0" {
			t.Fatalf("expected 0, got %q", got)
		}
		if got := formatDaysLeftSeconds(0); got != "0" {
			t.Fatalf("expected 0, got %q", got)
		}
	})

	t.Run("less than 1 day", func(t *testing.T) {
		if got := formatDaysLeftSeconds(1); got != "<1" {
			t.Fatalf("expected <1, got %q", got)
		}
		if got := formatDaysLeftSeconds(86399); got != "<1" {
			t.Fatalf("expected <1, got %q", got)
		}
	})

	t.Run("at least 1 day", func(t *testing.T) {
		if got := formatDaysLeftSeconds(86400); got != "1" {
			t.Fatalf("expected 1, got %q", got)
		}
		if got := formatDaysLeftSeconds(86401); got != "1" {
			t.Fatalf("expected 1, got %q", got)
		}
		if got := formatDaysLeftSeconds(172799); got != "1" {
			t.Fatalf("expected 1, got %q", got)
		}
		if got := formatDaysLeftSeconds(172800); got != "2" {
			t.Fatalf("expected 2, got %q", got)
		}
	})
}
