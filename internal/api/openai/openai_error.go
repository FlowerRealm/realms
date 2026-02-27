package openai

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

func writeOpenAIError(w http.ResponseWriter, status int, errType string, message string) {
	if w == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"type":    strings.TrimSpace(errType),
			"message": strings.TrimSpace(message),
		},
	})
}

func isInvalidEncryptedContentUpstreamError(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	code := strings.TrimSpace(gjson.GetBytes(body, "error.code").String())
	if strings.EqualFold(code, "invalid_encrypted_content") {
		return true
	}
	typ := strings.TrimSpace(gjson.GetBytes(body, "error.type").String())
	msg := strings.TrimSpace(gjson.GetBytes(body, "error.message").String())
	if strings.EqualFold(typ, "invalid_request_error") && strings.Contains(strings.ToLower(msg), "encrypted") {
		return true
	}
	return false
}

