package openai

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"realms/internal/store"
)

const (
	// maxMinutesFor5hWindow provides some buffer for the 5-hour (300 minutes) window.
	// Codex upstream may not always return exactly 300, so we treat anything <= 6h as "5h window".
	maxMinutesFor5hWindow = 360
	// minMinutesFor7dWindow is a lower bound for the 7-day (10080 minutes) window.
	// We use a slightly smaller threshold to avoid false negatives caused by upstream variations.
	minMinutesFor7dWindow = 10000
)

type codexQuotaWindow struct {
	usedPercent   *int
	resetAt       *time.Time
	windowMinutes *int
}

func parseCodexQuotaWindow(headers http.Header, prefix string, now time.Time) codexQuotaWindow {
	parseIntPtr := func(v string) *int {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil
		}
		i, err := strconv.Atoi(v)
		if err != nil {
			return nil
		}
		return &i
	}

	parsePercent := func(v string) *int {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		p := int(math.Round(f))
		if p < 0 {
			p = 0
		}
		if p > 100 {
			p = 100
		}
		return &p
	}

	resetAtFromSeconds := func(sec *int) *time.Time {
		if sec == nil {
			return nil
		}
		s := *sec
		if s < 0 {
			s = 0
		}
		t := now.Add(time.Duration(s) * time.Second).UTC()
		return &t
	}

	used := parsePercent(headers.Get("x-codex-" + prefix + "-used-percent"))
	resetAfter := parseIntPtr(headers.Get("x-codex-" + prefix + "-reset-after-seconds"))
	windowMins := parseIntPtr(headers.Get("x-codex-" + prefix + "-window-minutes"))

	return codexQuotaWindow{
		usedPercent:   used,
		resetAt:       resetAtFromSeconds(resetAfter),
		windowMinutes: windowMins,
	}
}

func isCodex5hWindow(windowMinutes *int) bool {
	if windowMinutes == nil {
		return false
	}
	return *windowMinutes > 0 && *windowMinutes <= maxMinutesFor5hWindow
}

func isCodex7dWindow(windowMinutes *int) bool {
	if windowMinutes == nil {
		return false
	}
	return *windowMinutes >= minMinutesFor7dWindow
}

func pickCodexWindows(primary, secondary codexQuotaWindow) (fiveH codexQuotaWindow, sevenD codexQuotaWindow, ok bool) {
	hasPrimaryWindow := primary.windowMinutes != nil
	hasSecondaryWindow := secondary.windowMinutes != nil

	switch {
	case hasPrimaryWindow && hasSecondaryWindow && *primary.windowMinutes > 0 && *secondary.windowMinutes > 0 && *primary.windowMinutes != *secondary.windowMinutes:
		// Both known: smaller is 5h, larger is 7d
		if *primary.windowMinutes < *secondary.windowMinutes {
			return primary, secondary, true
		}
		return secondary, primary, true
	case isCodex5hWindow(primary.windowMinutes):
		fiveH = primary
	case isCodex5hWindow(secondary.windowMinutes):
		fiveH = secondary
	}

	switch {
	case isCodex7dWindow(primary.windowMinutes):
		sevenD = primary
	case isCodex7dWindow(secondary.windowMinutes):
		sevenD = secondary
	}

	// No window-minutes: keep legacy UI semantics (primary=5h, secondary=7d)
	if fiveH.usedPercent == nil && fiveH.resetAt == nil && fiveH.windowMinutes == nil {
		fiveH = primary
	}
	if sevenD.usedPercent == nil && sevenD.resetAt == nil && sevenD.windowMinutes == nil {
		sevenD = secondary
	}

	if fiveH.usedPercent == nil && fiveH.resetAt == nil && sevenD.usedPercent == nil && sevenD.resetAt == nil {
		return codexQuotaWindow{}, codexQuotaWindow{}, false
	}
	return fiveH, sevenD, true
}

func codexQuotaPatchFromResponseHeaders(headers http.Header, now time.Time) (store.CodexOAuthQuotaPatch, bool) {
	if headers == nil {
		return store.CodexOAuthQuotaPatch{}, false
	}
	if now.IsZero() {
		now = time.Now()
	}

	primary := parseCodexQuotaWindow(headers, "primary", now)
	secondary := parseCodexQuotaWindow(headers, "secondary", now)

	fiveH, sevenD, ok := pickCodexWindows(primary, secondary)
	if !ok {
		return store.CodexOAuthQuotaPatch{}, false
	}

	patch := store.CodexOAuthQuotaPatch{
		PrimaryUsedPercent:   fiveH.usedPercent,
		PrimaryResetAt:       fiveH.resetAt,
		SecondaryUsedPercent: sevenD.usedPercent,
		SecondaryResetAt:     sevenD.resetAt,
	}

	return patch, true
}
