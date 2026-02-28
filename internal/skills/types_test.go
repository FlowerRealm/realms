package skills

import "testing"

func TestIsSafeID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"a", true},
		{"my-skill_1", true},
		{"", false},
		{"  ", false},
		{"../x", false},
		{"x/../y", false},
		{"x/y", false},
		{"x\\y", false},
		{"..", false},
		{".hidden", false}, // doesn't match regex (must start alnum)
	}
	for _, c := range cases {
		if got := IsSafeID(c.id); got != c.want {
			t.Fatalf("IsSafeID(%q)=%v want=%v", c.id, got, c.want)
		}
	}
}

func TestParseStoreV1JSON_EmptyOK(t *testing.T) {
	s, err := ParseStoreV1JSON("")
	if err != nil {
		t.Fatalf("ParseStoreV1JSON(empty) err=%v", err)
	}
	if s.Version != 1 {
		t.Fatalf("version=%d", s.Version)
	}
	if len(s.Skills) != 0 {
		t.Fatalf("skills=%v", s.Skills)
	}
}

func TestParseStoreV1JSON_Invalid(t *testing.T) {
	_, err := ParseStoreV1JSON(`{"version":1,"skills":{"../x":{"id":"../x","title":"t","prompt":"p"}}}`)
	if err == nil {
		t.Fatalf("expected error")
	}
}
