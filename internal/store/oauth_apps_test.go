package store

import "testing"

func TestNormalizeOAuthScope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: ""},
		{in: "  ", want: ""},
		{in: "read", want: "read"},
		{in: "Read WRITE", want: "read write"},
		{in: "write read read", want: "read write"},
		{in: "a b  c", want: "a b c"},
	}
	for _, tc := range cases {
		got, err := NormalizeOAuthScope(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("NormalizeOAuthScope(%q) expected error, got nil", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NormalizeOAuthScope(%q) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("NormalizeOAuthScope(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	if _, err := NormalizeOAuthScope(string(make([]byte, 3000))); err == nil {
		t.Fatalf("expected error for overly long scope")
	}
}

func TestNormalizeOAuthRedirectURI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "https://example.com/oauth/callback", want: "https://example.com/oauth/callback"},
		{in: "HTTPS://Example.com/oauth/callback", want: "https://example.com/oauth/callback"},
		{in: "https://example.com/oauth/callback?x=1", want: "https://example.com/oauth/callback?x=1"},
		{in: "/oauth/callback", wantErr: true},
		{in: "ftp://example.com/cb", wantErr: true},
		{in: "https://example.com/cb#frag", wantErr: true},
		{in: "https://user@example.com/cb", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tc := range cases {
		got, err := NormalizeOAuthRedirectURI(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("NormalizeOAuthRedirectURI(%q) expected error, got nil", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NormalizeOAuthRedirectURI(%q) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("NormalizeOAuthRedirectURI(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
