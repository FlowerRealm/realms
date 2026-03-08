package upstream

import (
	"net/http"
	"strings"
)

var codexOAuthAllowedHeaders = map[string]bool{
	"accept-language": true,
	"content-type":    true,
	"conversation_id": true,
	"user-agent":      true,
	"originator":      true,
	"session_id":      true,
}

func copyCodexOAuthWhitelistedHeaders(dst http.Header, src http.Header) {
	if dst == nil || src == nil {
		return
	}
	for key, values := range src {
		lowerKey := strings.ToLower(key)
		if !codexOAuthAllowedHeaders[lowerKey] {
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}

func isCodexCLIUserAgent(userAgent string) bool {
	userAgent = strings.ToLower(strings.TrimSpace(userAgent))
	return strings.Contains(userAgent, "codex_cli_rs") || strings.Contains(userAgent, "codex_vscode")
}
