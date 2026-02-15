package router

import (
	"expvar"
	"net"
	"net/http"
	"net/http/pprof"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func parseCIDRs(raw string) []*net.IPNet {
	var out []*net.IPNet
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(part)
		if err != nil || ipnet == nil {
			continue
		}
		out = append(out, ipnet)
	}
	return out
}

func requestRemoteIP(r *http.Request) net.IP {
	if r == nil {
		return nil
	}
	host := strings.TrimSpace(r.RemoteAddr)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return net.ParseIP(host)
}

func parseTrustedProxyCIDRs(raw string) []netip.Prefix {
	var out []netip.Prefix
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pfx, err := netip.ParsePrefix(part)
		if err != nil {
			continue
		}
		out = append(out, pfx)
	}
	return out
}

func isTrustedProxyRequest(r *http.Request, trustedProxies []netip.Prefix) bool {
	if r == nil || len(trustedProxies) == 0 {
		return false
	}
	host := strings.TrimSpace(r.RemoteAddr)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
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

func requestClientIP(r *http.Request) net.IP {
	if r == nil {
		return nil
	}

	ip := requestRemoteIP(r)

	trustProxyHeaders := false
	if v := strings.TrimSpace(os.Getenv("REALMS_TRUST_PROXY_HEADERS")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			trustProxyHeaders = b
		}
	}
	if !trustProxyHeaders {
		return ip
	}

	trustedProxies := parseTrustedProxyCIDRs(os.Getenv("REALMS_TRUSTED_PROXY_CIDRS"))
	if !isTrustedProxyRequest(r, trustedProxies) {
		return ip
	}

	xff := firstForwardedToken(r.Header.Get("X-Forwarded-For"))
	if xff == "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(xff); err == nil {
		xff = host
	}
	xffIP := net.ParseIP(strings.TrimSpace(xff))
	if xffIP == nil {
		return ip
	}
	return xffIP
}

func debugRoutesGuard() gin.HandlerFunc {
	allowCIDRs := parseCIDRs(os.Getenv("REALMS_DEBUG_ROUTES_ALLOW_CIDRS"))
	// Always allow localhost by default.
	allowCIDRs = append(allowCIDRs, parseCIDRs("127.0.0.1/32,::1/128")...)
	token := strings.TrimSpace(os.Getenv("REALMS_DEBUG_ROUTES_TOKEN"))

	return func(c *gin.Context) {
		if token != "" && c.GetHeader("X-Realms-Debug-Token") == token {
			c.Next()
			return
		}
		ip := requestClientIP(c.Request)
		for _, cidr := range allowCIDRs {
			if ip != nil && cidr.Contains(ip) {
				c.Next()
				return
			}
		}
		c.AbortWithStatus(403)
	}
}

func setSystemRoutes(r *gin.Engine, opts Options) {
	r.GET("/healthz", wrapHTTPFunc(opts.Healthz))

	r.GET("/assets/realms_icon.svg", wrapHTTPFunc(opts.RealmsIconSVG))
	r.HEAD("/assets/realms_icon.svg", wrapHTTPFunc(opts.RealmsIconSVG))

	r.GET("/favicon.ico", wrapHTTPFunc(opts.FaviconICO))
	r.HEAD("/favicon.ico", wrapHTTPFunc(opts.FaviconICO))

	// Debug routes are explicitly opt-in to avoid exposing pprof/vars to the public internet.
	// Enable for trusted environments only.
	if strings.TrimSpace(os.Getenv("REALMS_DEBUG_ROUTES")) != "" {
		guard := debugRoutesGuard()
		r.GET("/debug/vars", guard, gin.WrapH(expvar.Handler()))

		pp := r.Group("/debug/pprof", guard)
		pp.GET("/", gin.WrapF(pprof.Index))
		pp.GET("/cmdline", gin.WrapF(pprof.Cmdline))
		pp.GET("/profile", gin.WrapF(pprof.Profile))
		pp.GET("/symbol", gin.WrapF(pprof.Symbol))
		pp.POST("/symbol", gin.WrapF(pprof.Symbol))
		pp.GET("/trace", gin.WrapF(pprof.Trace))
		pp.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))
		pp.GET("/block", gin.WrapH(pprof.Handler("block")))
		pp.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
		pp.GET("/heap", gin.WrapH(pprof.Handler("heap")))
		pp.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))
		pp.GET("/threadcreate", gin.WrapH(pprof.Handler("threadcreate")))
	}
}
