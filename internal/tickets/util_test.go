package tickets

import "testing"

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1024, "1.00 KB"},
		{1024 * 1024, "1.00 MB"},
	}
	for _, c := range cases {
		got := FormatBytes(c.in)
		if got != c.want {
			t.Fatalf("FormatBytes(%d)=%q, want %q", c.in, got, c.want)
		}
	}
}
