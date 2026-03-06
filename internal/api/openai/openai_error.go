package openai

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

func writeOpenAIError(w http.ResponseWriter, status int, errType string, message string) {
	writeOpenAIErrorWithRetryAfter(w, status, errType, message, 0)
}

func writeOpenAIErrorWithRetryAfter(w http.ResponseWriter, status int, errType string, message string, retryAfterSeconds int) {
	if w == nil {
		return
	}
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	if retryAfterSeconds > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
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
