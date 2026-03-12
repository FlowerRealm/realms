package router

import (
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/gin-contrib/sessions"

	"realms/internal/security"
)

var trustedSessionCookieProxyCIDRs = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.1/32"),
	netip.MustParsePrefix("::1/128"),
}

func applySessionCookieOptions(sess sessions.Session, r *http.Request) {
	if sess == nil {
		return
	}
	sess.Options(sessions.Options{
		Path:     "/",
		MaxAge:   2592000, // 30 days
		HttpOnly: true,
		Secure:   requestUsesHTTPS(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func requestUsesHTTPS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if headerURLUsesHTTPS(r, "Origin") || headerURLUsesHTTPS(r, "Referer") {
		return true
	}
	baseURL := security.DeriveBaseURLFromRequest(r, true, trustedSessionCookieProxyCIDRs)
	return strings.HasPrefix(baseURL, "https://")
}

func headerURLUsesHTTPS(r *http.Request, headerName string) bool {
	if r == nil {
		return false
	}
	raw := strings.TrimSpace(r.Header.Get(headerName))
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || strings.TrimSpace(u.Host) == "" {
		return false
	}
	reqHost := strings.TrimSpace(r.Host)
	if reqHost == "" && r.URL != nil {
		reqHost = strings.TrimSpace(r.URL.Host)
	}
	return reqHost != "" && strings.EqualFold(u.Host, reqHost)
}
