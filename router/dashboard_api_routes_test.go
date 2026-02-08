package router

import (
	"testing"
	"time"
)

func TestDashboardUTCDayRange_UsesUTCDay(t *testing.T) {
	cst := time.FixedZone("CST", 8*60*60)
	nowInCST := time.Date(2026, 2, 8, 1, 30, 0, 0, cst)

	since, nowUTC := dashboardUTCDayRange(nowInCST)

	if got, want := nowUTC, time.Date(2026, 2, 7, 17, 30, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("nowUTC=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if got, want := since, time.Date(2026, 2, 7, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("since=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestDashboardUTCDayRange_WhenNowAlreadyUTC(t *testing.T) {
	now := time.Date(2026, 2, 8, 21, 45, 0, 0, time.UTC)

	since, nowUTC := dashboardUTCDayRange(now)

	if !nowUTC.Equal(now) {
		t.Fatalf("nowUTC=%s want=%s", nowUTC.Format(time.RFC3339), now.Format(time.RFC3339))
	}
	if got, want := since, time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("since=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}
