package codexoauth

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
)

func writeCallbackHTML(w http.ResponseWriter, status int, title string, message string, redirectURL string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	safeTitle := html.EscapeString(title)
	safeMessage := html.EscapeString(message)
	safeRedirect := html.EscapeString(redirectURL)

	linkHTML := ""
	scriptHTML := ""
	if redirectURL != "" {
		redirectPath := ""
		if u, err := url.Parse(redirectURL); err == nil && u != nil {
			redirectPath = u.EscapedPath()
			if u.RawQuery != "" {
				redirectPath += "?" + u.RawQuery
			}
			if u.Fragment != "" {
				redirectPath += "#" + u.Fragment
			}
		}
		linkHTML = fmt.Sprintf(`<p><a href="%s">返回管理后台</a></p>`, safeRedirect)
		scriptHTML = fmt.Sprintf(`<script>
(function () {
  var redirectURL = %q;
  var redirectPath = %q;
  try { window.name = "realms_codex_oauth_popup"; } catch (e) {}
  try {
    if (redirectURL && window.opener && !window.opener.closed) {
      try { window.opener.postMessage({ type: "realms_codex_oauth_callback", redirectURL: redirectURL, redirectPath: redirectPath }, "*"); } catch (e) {}
    }
  } catch (e) {}

  setTimeout(function () {
    var hasOpener = false;
    try { hasOpener = !!(window.opener && !window.opener.closed); } catch (e) { hasOpener = false; }
    if (!hasOpener) {
      // OpenAI 的 OAuth 页面会设置 COOP=same-origin，导致 window.opener 被浏览器清空。
      // 在这种情况下，让窗口跳转回管理后台，由管理后台页面通过 localStorage 广播刷新原页面。
      try { window.location.href = redirectURL; } catch (e) {}
      return;
    }

    try { window.close(); } catch (e) {}
  }, 800);
})();
</script>`, redirectURL, redirectPath)
	}

	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>%s</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Fira+Code:wght@300;400;500;600;700&family=Noto+Sans+SC:wght@300;400;500;700&display=swap" rel="stylesheet">
  <style>
    :root { color-scheme: light dark; }
    body { font-family: 'Fira Code', 'Noto Sans SC', monospace, ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial; margin: 0; padding: 24px; }
    .card { max-width: 720px; margin: 0 auto; padding: 18px 20px; border: 1px solid rgba(127,127,127,.25); border-radius: 12px; }
    h1 { font-size: 18px; margin: 0 0 8px; }
    p { margin: 8px 0; line-height: 1.5; }
    a { text-decoration: none; }
  </style>
</head>
<body>
  <div class="card">
    <h1>%s</h1>
    <p>%s</p>
    %s
    <p>你也可以直接关闭此窗口。</p>
  </div>
  %s
</body>
</html>`, safeTitle, safeTitle, safeMessage, linkHTML, scriptHTML)
}
