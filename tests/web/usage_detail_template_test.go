package web_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUsageTemplates_ContainExpandableDetails(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	cases := []struct {
		name     string
		path     string
		contains []string
	}{
		{
			name: "web_usage",
			path: filepath.Join(repoRoot, "internal", "web", "templates", "usage.html"),
			contains: []string{
				`id="rlmUsageEvents"`,
				`data-bs-target="#rlmUsageDetail{{.ID}}"`,
				`id="rlmUsageDetail{{.ID}}"`,
				`{{.ErrorMessage}}`,
				`onclick="event.stopPropagation();"`,
			},
		},
		{
			name: "admin_usage",
			path: filepath.Join(repoRoot, "internal", "admin", "templates", "usage.html"),
			contains: []string{
				`id="rlmAdminUsageEvents"`,
				`data-bs-target="#rlmAdminUsageDetail{{.ID}}"`,
				`id="rlmAdminUsageDetail{{.ID}}"`,
				`{{.ErrorMessage}}`,
				`onclick="event.stopPropagation();"`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			s := string(b)
			for _, want := range tc.contains {
				if !strings.Contains(s, want) {
					t.Fatalf("expected %q in %s", want, tc.path)
				}
			}
		})
	}
}
