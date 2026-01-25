package admin

import (
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) UpdateChannelModelSuffixPreserve(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	channelID, err := parseInt64(r.PathValue("channel_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "表单解析失败")
			return
		}
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	returnTo := safeAdminReturnTo(r.FormValue("return_to"), "/admin/channels")
	raw := strings.TrimSpace(r.FormValue("model_suffix_preserve"))

	if err := s.st.UpdateUpstreamChannelModelSuffixPreserve(r.Context(), channelID, raw); err != nil {
		status := http.StatusInternalServerError
		msg := "保存失败"
		if strings.Contains(err.Error(), "不是有效 JSON 数组") || strings.Contains(err.Error(), "数组元素不能为空") {
			status = http.StatusBadRequest
			msg = "model_suffix_preserve 不是有效 JSON 数组"
		}
		if isAjax(r) {
			ajaxError(w, status, msg)
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape(msg), http.StatusFound)
		return
	}

	if isAjax(r) {
		ajaxOK(w, "模型后缀保护名单已保存")
		return
	}
	http.Redirect(w, r, returnTo+"?msg="+url.QueryEscape("模型后缀保护名单已保存"), http.StatusFound)
}
