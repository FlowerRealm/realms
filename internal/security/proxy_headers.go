package security

import (
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// DeriveBaseURLFromRequest 基于请求推断对外 Base URL（用于页面展示、支付回调/返回地址、OAuth 回跳等）。
//
// 安全约束：
// - 仅当 trustProxyHeaders=true 且请求来源命中 trustedProxies 时，才信任 X-Forwarded-*。
// - X-Forwarded-Proto 仅允许 http/https；X-Forwarded-Host 仅允许纯 host[:port]（不允许路径等）。
func DeriveBaseURLFromRequest(r *http.Request, trustProxyHeaders bool, trustedProxies []netip.Prefix) string {
	if r == nil {
		return ""
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	host := strings.TrimSpace(r.Host)
	if host == "" && r.URL != nil {
		host = strings.TrimSpace(r.URL.Host)
	}

	if trustProxyHeaders && isTrustedProxyRequest(r, trustedProxies) {
		if proto, ok := forwardedProto(r.Header.Get("X-Forwarded-Proto")); ok {
			scheme = proto
		}
		if h, ok := forwardedHost(r.Header.Get("X-Forwarded-Host")); ok {
			host = h
		}
	}

	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func isTrustedProxyRequest(r *http.Request, trustedProxies []netip.Prefix) bool {
	if r == nil {
		return false
	}
	if len(trustedProxies) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	for _, pfx := range trustedProxies {
		if pfx.Contains(ip) {
			return true
		}
	}
	return false
}

func forwardedProto(raw string) (string, bool) {
	v := strings.ToLower(firstForwardedToken(raw))
	switch v {
	case "http", "https":
		return v, true
	default:
		return "", false
	}
}

func forwardedHost(raw string) (string, bool) {
	v := firstForwardedToken(raw)
	if v == "" {
		return "", false
	}
	if strings.ContainsAny(v, " \t\r\n") {
		return "", false
	}
	if strings.ContainsAny(v, "/\\") {
		return "", false
	}

	u, err := url.Parse("http://" + v)
	if err != nil {
		return "", false
	}
	if u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	if !strings.EqualFold(u.Host, v) {
		return "", false
	}
	return v, true
}

func firstForwardedToken(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if idx := strings.IndexByte(v, ','); idx >= 0 {
		v = v[:idx]
	}
	return strings.TrimSpace(v)
}
