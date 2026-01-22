package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvFile_StripsQuotes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO='bar'\nBAR=\"baz\"\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Unsetenv("FOO")
		_ = os.Unsetenv("BAR")
	})

	loaded, err := loadDotEnvFile(path)
	if err != nil {
		t.Fatalf("loadDotEnvFile: %v", err)
	}
	if !loaded {
		t.Fatalf("expected loaded=true")
	}
	if got := os.Getenv("FOO"); got != "bar" {
		t.Fatalf("FOO=%q, want %q", got, "bar")
	}
	if got := os.Getenv("BAR"); got != "baz" {
		t.Fatalf("BAR=%q, want %q", got, "baz")
	}
}

func TestLoadDotEnvFile_OverridesExistingEnv(t *testing.T) {
	t.Setenv("FOO", "existing")

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=bar\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	loaded, err := loadDotEnvFile(path)
	if err != nil {
		t.Fatalf("loadDotEnvFile: %v", err)
	}
	if !loaded {
		t.Fatalf("expected loaded=true")
	}
	if got := os.Getenv("FOO"); got != "bar" {
		t.Fatalf("FOO=%q, want %q", got, "bar")
	}
}

func TestLoadDotEnvFile_InvalidLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("BADLINE\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	loaded, err := loadDotEnvFile(path)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !loaded {
		t.Fatalf("expected loaded=true for existing file")
	}
}
