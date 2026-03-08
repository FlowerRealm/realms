package config

import "testing"

func TestNormalizeAndValidate_RejectsPersonalMode(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = ModePersonal

	if _, err := normalizeAndValidate(cfg); err == nil {
		t.Fatalf("expected personal mode to be rejected")
	}
}
