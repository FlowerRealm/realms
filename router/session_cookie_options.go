package router

import (
	"net/http"
	"net/netip"
	"strings"

	"github.com/gin-contrib/sessions"

	"realms/internal/security"
)

var trustedSessionCookieProxyCIDRs = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.1/32"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
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
	baseURL := security.DeriveBaseURLFromRequest(r, true, trustedSessionCookieProxyCIDRs)
	return strings.HasPrefix(baseURL, "https://")
}
