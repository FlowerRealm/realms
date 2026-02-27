package openai

import (
	"net/http"
	"testing"
	"time"
)

func TestCodexQuotaPatchFromResponseHeaders_WindowMinutesNormalizeTo5hAnd7d(t *testing.T) {
	now := time.Date(2026, 2, 27, 0, 0, 0, 0, time.UTC)
	h := make(http.Header)
	h.Set("x-codex-primary-used-percent", "88")
	h.Set("x-codex-primary-reset-after-seconds", "604800")
	h.Set("x-codex-primary-window-minutes", "10080")
	h.Set("x-codex-secondary-used-percent", "12")
	h.Set("x-codex-secondary-reset-after-seconds", "3600")
	h.Set("x-codex-secondary-window-minutes", "300")

	patch, ok := codexQuotaPatchFromResponseHeaders(h, now)
	if !ok {
		t.Fatal("expected ok=true")
	}

	// UI semantics: quota_primary = 5h window, quota_secondary = 7d window.
	if patch.PrimaryUsedPercent == nil || *patch.PrimaryUsedPercent != 12 {
		t.Fatalf("PrimaryUsedPercent=%v, want 12", patch.PrimaryUsedPercent)
	}
	if patch.SecondaryUsedPercent == nil || *patch.SecondaryUsedPercent != 88 {
		t.Fatalf("SecondaryUsedPercent=%v, want 88", patch.SecondaryUsedPercent)
	}

	if patch.PrimaryResetAt == nil || !patch.PrimaryResetAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("PrimaryResetAt=%v, want %v", patch.PrimaryResetAt, now.Add(time.Hour))
	}
	if patch.SecondaryResetAt == nil || !patch.SecondaryResetAt.Equal(now.Add(7*24*time.Hour)) {
		t.Fatalf("SecondaryResetAt=%v, want %v", patch.SecondaryResetAt, now.Add(7*24*time.Hour))
	}
}

func TestCodexQuotaPatchFromResponseHeaders_NoWindowMinutesFallsBackToPrimaryAndSecondary(t *testing.T) {
	now := time.Date(2026, 2, 27, 1, 2, 3, 0, time.UTC)
	h := make(http.Header)
	h.Set("x-codex-primary-used-percent", "34")
	h.Set("x-codex-primary-reset-after-seconds", "10")
	h.Set("x-codex-secondary-used-percent", "90")
	h.Set("x-codex-secondary-reset-after-seconds", "20")

	patch, ok := codexQuotaPatchFromResponseHeaders(h, now)
	if !ok {
		t.Fatal("expected ok=true")
	}

	// Legacy UI semantics: primary=5h, secondary=7d.
	if patch.PrimaryUsedPercent == nil || *patch.PrimaryUsedPercent != 34 {
		t.Fatalf("PrimaryUsedPercent=%v, want 34", patch.PrimaryUsedPercent)
	}
	if patch.SecondaryUsedPercent == nil || *patch.SecondaryUsedPercent != 90 {
		t.Fatalf("SecondaryUsedPercent=%v, want 90", patch.SecondaryUsedPercent)
	}
}

func TestCodexQuotaPatchFromResponseHeaders_ClampsPercentAndNegativeReset(t *testing.T) {
	now := time.Date(2026, 2, 27, 0, 0, 0, 0, time.UTC)
	h := make(http.Header)
	h.Set("x-codex-primary-used-percent", "101.2")
	h.Set("x-codex-primary-reset-after-seconds", "-3")

	patch, ok := codexQuotaPatchFromResponseHeaders(h, now)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if patch.PrimaryUsedPercent == nil || *patch.PrimaryUsedPercent != 100 {
		t.Fatalf("PrimaryUsedPercent=%v, want 100", patch.PrimaryUsedPercent)
	}
	if patch.PrimaryResetAt == nil || !patch.PrimaryResetAt.Equal(now) {
		t.Fatalf("PrimaryResetAt=%v, want %v", patch.PrimaryResetAt, now)
	}
}
