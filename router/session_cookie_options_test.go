package router

import (
	"net/http/httptest"
	"testing"
)

func TestRequestUsesHTTPS(t *testing.T) {
	tests := []struct {
		name       string
		targetURL  string
		remoteAddr string
		forwarded  string
		want       bool
	}{
		{
			name:      "plain http request",
			targetURL: "http://example.com/login",
			want:      false,
		},
		{
			name:      "https request",
			targetURL: "https://example.com/login",
			want:      true,
		},
		{
			name:       "loopback proxy may forward https",
			targetURL:  "http://example.com/login",
			remoteAddr: "127.0.0.1:12345",
			forwarded:  "https",
			want:       true,
		},
		{
			name:      "https origin on same host implies secure cookie",
			targetURL: "http://example.com/login",
			want:      true,
		},
		{
			name:       "private network client cannot spoof forwarded proto",
			targetURL:  "http://example.com/login",
			remoteAddr: "10.0.0.9:12345",
			forwarded:  "https",
			want:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.targetURL, nil)
			if tc.remoteAddr != "" {
				req.RemoteAddr = tc.remoteAddr
			}
			if tc.forwarded != "" {
				req.Header.Set("X-Forwarded-Proto", tc.forwarded)
				req.Header.Set("X-Forwarded-Host", "example.com")
			}
			if tc.name == "https origin on same host implies secure cookie" {
				req.Header.Set("Origin", "https://example.com")
			}
			if got := requestUsesHTTPS(req); got != tc.want {
				t.Fatalf("requestUsesHTTPS() = %v, want %v", got, tc.want)
			}
		})
	}
}
