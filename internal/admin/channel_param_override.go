package admin

import (
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) UpdateChannelParamOverride(w http.ResponseWriter, r *http.Request) {
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
	paramOverride := strings.TrimSpace(r.FormValue("param_override"))

	if err := s.st.UpdateUpstreamChannelParamOverride(r.Context(), channelID, paramOverride); err != nil {
		status := http.StatusInternalServerError
		msg := "保存失败"
		if strings.Contains(err.Error(), "param_override 不是有效 JSON") {
			status = http.StatusBadRequest
			msg = "param_override 不是有效 JSON"
		}
		if isAjax(r) {
			ajaxError(w, status, msg)
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape(msg), http.StatusFound)
		return
	}

	if isAjax(r) {
		ajaxOK(w, "参数改写已保存")
		return
	}
	http.Redirect(w, r, returnTo+"?msg="+url.QueryEscape("参数改写已保存"), http.StatusFound)
}
