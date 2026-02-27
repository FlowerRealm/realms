package router

import (
	"testing"
	"time"
)

func TestDashboardTodayRange_UsesLocalMidnight(t *testing.T) {
	cst := time.FixedZone("CST", 8*60*60)
	nowUTC := time.Date(2026, 2, 7, 17, 30, 0, 0, time.UTC) // 2026-02-08 01:30 in CST

	sinceUTC, untilUTC, sinceLocal, untilLocal, ok := parseDateRangeInLocation(nowUTC, "", "", cst)
	if !ok {
		t.Fatalf("parseDateRangeInLocation failed")
	}

	if got, want := untilUTC, nowUTC; !got.Equal(want) {
		t.Fatalf("untilUTC=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if got, want := sinceUTC, time.Date(2026, 2, 7, 16, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("sinceUTC=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if got, want := sinceLocal, time.Date(2026, 2, 8, 0, 0, 0, 0, cst); !got.Equal(want) {
		t.Fatalf("sinceLocal=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if !untilLocal.Equal(nowUTC.In(cst)) {
		t.Fatalf("untilLocal=%s want=%s", untilLocal.Format(time.RFC3339), nowUTC.In(cst).Format(time.RFC3339))
	}
}

func TestDashboardTodayRange_WhenUTC(t *testing.T) {
	nowUTC := time.Date(2026, 2, 8, 21, 45, 0, 0, time.UTC)

	sinceUTC, untilUTC, _, _, ok := parseDateRangeInLocation(nowUTC, "", "", time.UTC)
	if !ok {
		t.Fatalf("parseDateRangeInLocation failed")
	}

	if got, want := untilUTC, nowUTC; !got.Equal(want) {
		t.Fatalf("untilUTC=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if got, want := sinceUTC, time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("sinceUTC=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}
