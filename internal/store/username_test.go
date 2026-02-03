package store

import (
	"strings"
	"testing"
)

func TestNormalizeUsername(t *testing.T) {
	t.Run("trim_and_keep_case", func(t *testing.T) {
		got, err := NormalizeUsername("  Alice01  ")
		if err != nil {
			t.Fatalf("NormalizeUsername: %v", err)
		}
		if got != "Alice01" {
			t.Fatalf("unexpected username: %q", got)
		}
	})

	t.Run("reject_empty", func(t *testing.T) {
		if _, err := NormalizeUsername("   "); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("reject_special_chars", func(t *testing.T) {
		for _, in := range []string{"a_b", "a-b", "a.b", "a b", "a@b"} {
			if _, err := NormalizeUsername(in); err == nil {
				t.Fatalf("expected error for %q", in)
			}
		}
	})

	t.Run("reject_non_ascii", func(t *testing.T) {
		if _, err := NormalizeUsername("中文"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("reject_too_long", func(t *testing.T) {
		in := strings.Repeat("a", 65)
		if _, err := NormalizeUsername(in); err == nil {
			t.Fatalf("expected error")
		}
	})
}
