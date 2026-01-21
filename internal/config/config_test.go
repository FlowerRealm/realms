package config

import (
	"strings"
	"testing"
)

func TestNormalizeHTTPBaseURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		in         string
		label      string
		want       string
		wantErrSub string
	}{
		{name: "empty ok", in: "", label: "site_base_url", want: ""},
		{name: "trim ok", in: " https://example.com/ ", label: "site_base_url", want: "https://example.com"},
		{name: "trim right slash ok", in: "https://example.com/", label: "site_base_url", want: "https://example.com"},
		{name: "path ok", in: "https://example.com/realms/", label: "site_base_url", want: "https://example.com/realms"},
		{name: "invalid scheme", in: "ftp://example.com", label: "site_base_url", wantErrSub: "site_base_url 仅支持 http/https"},
		{name: "missing host", in: "https://", label: "site_base_url", wantErrSub: "site_base_url host 不能为空"},
		{name: "parse error", in: "://bad", label: "site_base_url", wantErrSub: "解析 site_base_url 失败"},
		{name: "no label scheme", in: "ftp://example.com", label: "", wantErrSub: "base_url 仅支持 http/https"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := NormalizeHTTPBaseURL(tc.in, tc.label)
			if tc.wantErrSub != "" {
				if err == nil {
					t.Fatalf("NormalizeHTTPBaseURL(%q, %q) expected error, got nil", tc.in, tc.label)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("NormalizeHTTPBaseURL(%q, %q) error = %q, want contains %q", tc.in, tc.label, err.Error(), tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeHTTPBaseURL(%q, %q) unexpected error: %v", tc.in, tc.label, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeHTTPBaseURL(%q, %q) = %q, want %q", tc.in, tc.label, got, tc.want)
			}
		})
	}
}

func TestApplyEnvOverrides_SelfModeEnable(t *testing.T) {
	t.Setenv("REALMS_SELF_MODE_ENABLE", "true")

	cfg := defaultConfig()
	applyEnvOverrides(&cfg)

	if !cfg.SelfMode.Enable {
		t.Fatalf("expected cfg.SelfMode.Enable=true")
	}
}
