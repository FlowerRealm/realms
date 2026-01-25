package version

import (
	"strings"
	"testing"
)

func TestInfo_UsesEmbeddedVersionByDefault(t *testing.T) {
	oldVersion := Version
	oldCommit := Commit
	oldDate := Date
	t.Cleanup(func() {
		Version = oldVersion
		Commit = oldCommit
		Date = oldDate
	})

	Version = "dev"
	got := Info()

	want := strings.TrimSpace(embeddedVersion)
	if want == "" {
		t.Fatalf("embeddedVersion is empty")
	}
	if got.Version != want {
		t.Fatalf("Version mismatch: got=%q want=%q", got.Version, want)
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
