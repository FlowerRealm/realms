package skills

import "testing"

func TestParseFrontmatter_Roundtrip(t *testing.T) {
	md := renderSkillMarkdown("my-skill", "only when asked", "do it\nnow")
	meta, body, ok := parseFrontmatter(md)
	if !ok {
		t.Fatalf("expected ok")
	}
	if stringFromMeta(meta, "name") != "my-skill" {
		t.Fatalf("name=%q", stringFromMeta(meta, "name"))
	}
	if stringFromMeta(meta, "description") != "only when asked" {
		t.Fatalf("description=%q", stringFromMeta(meta, "description"))
	}
	if body != "do it\nnow" {
		t.Fatalf("body=%q", body)
	}
}
