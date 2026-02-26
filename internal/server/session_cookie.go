package server

const SessionCookieName = "realms_session"

// SessionCookieNameForPersonalMode 返回 Web 会话 cookie 名。
//
// 说明：浏览器 cookie 不区分端口（仅按域名 + Path），因此在同一 host 上同时运行两套 Realms（例如本地 8080 business + Docker 7080 personal）时，
// 若 cookie 名相同会互相覆盖/清理，导致“一个窗口登录另一个窗口掉线”。
//
// 约定：personal 模式使用独立 cookie 名，避免与 business 模式冲突。
func SessionCookieNameForPersonalMode(personalMode bool) string {
	if personalMode {
		return SessionCookieName + "_personal"
	}
	return SessionCookieName
}
