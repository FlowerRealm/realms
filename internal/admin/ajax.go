// ajax.go 提供管理后台渐进式 AJAX 表单的请求识别与 JSON 响应封装。
package admin

import (
	"encoding/json"
	"net/http"
	"strings"
)

type ajaxResponse struct {
	OK     bool   `json:"ok"`
	Notice string `json:"notice,omitempty"`
	Error  string `json:"error,omitempty"`
}

const maxAjaxMessageLen = 160

func isAjax(r *http.Request) bool {
	if strings.TrimSpace(r.Header.Get("X-Realms-Ajax")) == "1" {
		return true
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(accept, "application/json")
}

func normalizeAjaxMessage(raw string) string {
	msg := strings.TrimSpace(raw)
	msg = strings.ReplaceAll(msg, "\r", " ")
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > maxAjaxMessageLen {
		msg = msg[:maxAjaxMessageLen] + "..."
	}
	return msg
}

func writeAjax(w http.ResponseWriter, status int, resp ajaxResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func ajaxOK(w http.ResponseWriter, notice string) {
	writeAjax(w, http.StatusOK, ajaxResponse{OK: true, Notice: normalizeAjaxMessage(notice)})
}

func ajaxError(w http.ResponseWriter, status int, errMsg string) {
	msg := normalizeAjaxMessage(errMsg)
	if msg == "" {
		msg = "请求失败"
	}
	writeAjax(w, status, ajaxResponse{OK: false, Error: msg})
}
