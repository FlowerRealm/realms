package security

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestDeriveBaseURLFromRequest_IgnoresForwardedWhenUntrusted(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.com/", nil)
	r.RemoteAddr = "203.0.113.10:1234"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "evil.example.com")

	if got := DeriveBaseURLFromRequest(r, true, nil); got != "http://example.com" {
		t.Fatalf("expected base url to ignore forwarded headers, got %q", got)
	}
}

func TestDeriveBaseURLFromRequest_UsesForwardedWhenTrusted(t *testing.T) {
	r := httptest.NewRequest("GET", "http://internal.local/", nil)
	r.RemoteAddr = "10.1.2.3:1234"
	r.Header.Set("X-Forwarded-Proto", "https, http")
	r.Header.Set("X-Forwarded-Host", "pay.example.com, internal.local")

	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	if got := DeriveBaseURLFromRequest(r, true, trusted); got != "https://pay.example.com" {
		t.Fatalf("expected base url to use forwarded headers, got %q", got)
	}
}

func TestDeriveBaseURLFromRequest_RejectsInvalidForwardedValues(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.com/", nil)
	r.RemoteAddr = "10.1.2.3:1234"
	r.Header.Set("X-Forwarded-Proto", "ftp")
	r.Header.Set("X-Forwarded-Host", "evil.example.com/path")

	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	if got := DeriveBaseURLFromRequest(r, true, trusted); got != "http://example.com" {
		t.Fatalf("expected invalid forwarded values to be ignored, got %q", got)
	}
}
