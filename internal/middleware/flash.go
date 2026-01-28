package middleware

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	flashNoticeCookieName = "rlm_flash_notice"
	flashErrorCookieName  = "rlm_flash_error"
	nextPathCookieName    = "rlm_next"

	usageStartCookieName = "rlm_usage_start"
	usageEndCookieName   = "rlm_usage_end"
	usageLimitCookieName = "rlm_usage_limit"
)

type flashContextKey struct{}

type flashData struct {
	Notice string
	Error  string
}

func FlashNotice(ctx context.Context) string {
	v := ctx.Value(flashContextKey{})
	fd, ok := v.(flashData)
	if !ok {
		return ""
	}
	return fd.Notice
}

func FlashError(ctx context.Context) string {
	v := ctx.Value(flashContextKey{})
	fd, ok := v.(flashData)
	if !ok {
		return ""
	}
	return fd.Error
}

func NextPathFromCookie(r *http.Request) string {
	if r == nil {
		return ""
	}
	c, err := r.Cookie(nextPathCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(c.Value)
}

func ClearNextPathCookie(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, r, nextPathCookieName)
}

func SetNextPathCookie(w http.ResponseWriter, r *http.Request, nextPath string) {
	nextPath = strings.TrimSpace(nextPath)
	if nextPath == "" {
		return
	}
	if !strings.HasPrefix(nextPath, "/") || strings.HasPrefix(nextPath, "//") || strings.Contains(nextPath, "\\") {
		return
	}
	u, err := url.Parse(nextPath)
	if err != nil || u.IsAbs() || u.Host != "" || u.Scheme != "" {
		return
	}
	nextPath = strings.TrimSpace(u.Path)
	if nextPath == "" || !strings.HasPrefix(nextPath, "/") || strings.HasPrefix(nextPath, "//") {
		return
	}
	setCookie(w, r, nextPathCookieName, nextPath, 10*time.Minute)
}

func SetFlashNotice(w http.ResponseWriter, r *http.Request, msg string) {
	setFlash(w, r, flashNoticeCookieName, msg)
}

func SetFlashError(w http.ResponseWriter, r *http.Request, msg string) {
	setFlash(w, r, flashErrorCookieName, msg)
}

func setFlash(w http.ResponseWriter, r *http.Request, cookieName string, msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	if len(msg) > 500 {
		msg = msg[:500] + "..."
	}
	enc := base64.RawURLEncoding.EncodeToString([]byte(msg))
	setCookie(w, r, cookieName, enc, 2*time.Minute)
}

func FlashFromCookies(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r == nil {
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead:
		default:
			next.ServeHTTP(w, r)
			return
		}
		notice := popFlash(w, r, flashNoticeCookieName)
		errMsg := popFlash(w, r, flashErrorCookieName)
		ctx := context.WithValue(r.Context(), flashContextKey{}, flashData{
			Notice: notice,
			Error:  errMsg,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func popFlash(w http.ResponseWriter, r *http.Request, cookieName string) string {
	if r == nil {
		return ""
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	clearCookie(w, r, cookieName)
	raw := strings.TrimSpace(c.Value)
	if raw == "" {
		return ""
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func clearCookie(w http.ResponseWriter, r *http.Request, name string) {
	if w == nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   r != nil && r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

func setCookie(w http.ResponseWriter, r *http.Request, name string, value string, ttl time.Duration) {
	if w == nil {
		return
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		Expires:  time.Now().Add(ttl),
		HttpOnly: true,
		Secure:   r != nil && r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

func UsageStartFromCookie(r *http.Request) string {
	return cookieValueTrimmed(r, usageStartCookieName)
}

func UsageEndFromCookie(r *http.Request) string {
	return cookieValueTrimmed(r, usageEndCookieName)
}

func UsageLimitFromCookie(r *http.Request) string {
	return cookieValueTrimmed(r, usageLimitCookieName)
}

func SetUsageFilterCookies(w http.ResponseWriter, r *http.Request, start string, end string, limit int) {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	if start != "" {
		setCookie(w, r, usageStartCookieName, start, 30*24*time.Hour)
	}
	if end != "" {
		setCookie(w, r, usageEndCookieName, end, 30*24*time.Hour)
	}
	if limit > 0 {
		setCookie(w, r, usageLimitCookieName, strconv.Itoa(limit), 30*24*time.Hour)
	}
}

func ClearUsageFilterCookies(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, r, usageStartCookieName)
	clearCookie(w, r, usageEndCookieName)
	clearCookie(w, r, usageLimitCookieName)
}

func cookieValueTrimmed(r *http.Request, name string) string {
	if r == nil {
		return ""
	}
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(c.Value)
}
