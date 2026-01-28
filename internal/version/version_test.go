package version

import (
	"testing"
)

func TestInfo_UsesEmbeddedVersionByDefault(t *testing.T) {
	oldVersion := Version
	oldDate := Date
	t.Cleanup(func() {
		Version = oldVersion
		Date = oldDate
	})

	Version = "dev"
	got := Info()

	if got.Version != "dev" {
		t.Fatalf("Version mismatch: got=%q want=%q", got.Version, "dev")
	}
}

func TestInfo_VersionOverride(t *testing.T) {
	oldVersion := Version
	t.Cleanup(func() { Version = oldVersion })

	Version = "9.9.9"
	got := Info()
	if got.Version != "9.9.9" {
		t.Fatalf("Version mismatch: got=%q want=%q", got.Version, "9.9.9")
	}
}
