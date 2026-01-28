package middleware

import (
	"net/http"
	"strconv"
	"strings"
)

// StripWebQuery 将 Web(SSR) 页面上的 query 参数“自动略去”：
// - 对 GET/HEAD 且存在 query 的请求做 302 跳转到无 query 的等价 URL
// - 对部分历史 query（msg/err/next/usage 过滤/分页/tickets 状态）做 cookie/path 的兼容迁移
//
// 注意：协议性端点（如 OAuth authorize）依赖 query，应在此中间件中排除。
func StripWebQuery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r == nil || r.URL == nil || strings.TrimSpace(r.URL.RawQuery) == "" {
			next.ServeHTTP(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead:
		default:
			next.ServeHTTP(w, r)
			return
		}

		// OAuth authorize 属于协议端点，Query 是必需的（response_type/client_id/...）。
		if strings.HasPrefix(r.URL.Path, "/oauth/authorize") {
			next.ServeHTTP(w, r)
			return
		}

		q := r.URL.Query()

		if v := strings.TrimSpace(q.Get("msg")); v != "" {
			SetFlashNotice(w, r, v)
		}
		if v := strings.TrimSpace(q.Get("err")); v != "" {
			SetFlashError(w, r, v)
		}
		if v := strings.TrimSpace(q.Get("next")); v != "" {
			SetNextPathCookie(w, r, v)
		}

		target := strings.TrimSpace(r.URL.Path)

		// /usage: 将 start/end/limit 写入 cookie，并把 before_id/after_id 迁移到 path。
		if target == "/usage" {
			start := strings.TrimSpace(q.Get("start"))
			end := strings.TrimSpace(q.Get("end"))
			limitRaw := strings.TrimSpace(q.Get("limit"))
			limit := 0
			if limitRaw != "" {
				if n, err := strconv.Atoi(limitRaw); err == nil {
					limit = n
				}
			}
			if start != "" || end != "" || limit > 0 {
				SetUsageFilterCookies(w, r, start, end, limit)
			}

			beforeRaw := strings.TrimSpace(q.Get("before_id"))
			afterRaw := strings.TrimSpace(q.Get("after_id"))
			if beforeRaw != "" && afterRaw != "" {
				SetFlashError(w, r, "before_id 与 after_id 不能同时使用")
			} else if beforeRaw != "" {
				if id, err := strconv.ParseInt(beforeRaw, 10, 64); err == nil && id > 0 {
					target = "/usage/before/" + strconv.FormatInt(id, 10)
				}
			} else if afterRaw != "" {
				if id, err := strconv.ParseInt(afterRaw, 10, 64); err == nil && id > 0 {
					target = "/usage/after/" + strconv.FormatInt(id, 10)
				}
			}
		}

		// /tickets: ?status=open|closed -> /tickets/open|/tickets/closed
		if target == "/tickets" {
			switch strings.TrimSpace(q.Get("status")) {
			case "open":
				target = "/tickets/open"
			case "closed":
				target = "/tickets/closed"
			default:
			}
		}

		if target == "" || !strings.HasPrefix(target, "/") {
			target = "/"
		}
		http.Redirect(w, r, target, http.StatusFound)
	})
}
