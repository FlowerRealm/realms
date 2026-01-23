package web_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaseTemplates_ContainGitHubFooterAndGuard(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	cases := []struct {
		name string
		path string
	}{
		{name: "web_base", path: filepath.Join(repoRoot, "internal", "web", "templates", "base.html")},
		{name: "admin_base", path: filepath.Join(repoRoot, "internal", "admin", "templates", "base.html")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			s := string(b)

			if !strings.Contains(s, `id="rlmProjectFooter"`) {
				t.Fatalf("expected project footer id in %s", tc.path)
			}
			if !strings.Contains(s, "https://github.com/FlowerRealm/realms") {
				t.Fatalf("expected GitHub repo link in %s", tc.path)
			}
			if !strings.Contains(s, "MutationObserver") {
				t.Fatalf("expected MutationObserver guard in %s", tc.path)
			}
		})
	}
}
