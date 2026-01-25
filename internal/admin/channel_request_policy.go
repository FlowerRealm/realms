package admin

import (
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) UpdateChannelRequestPolicy(w http.ResponseWriter, r *http.Request) {
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

	allowServiceTier := strings.TrimSpace(r.FormValue("allow_service_tier")) == "1"
	disableStore := strings.TrimSpace(r.FormValue("disable_store")) == "1"
	allowSafetyIdentifier := strings.TrimSpace(r.FormValue("allow_safety_identifier")) == "1"

	if err := s.st.UpdateUpstreamChannelRequestPolicy(r.Context(), channelID, allowServiceTier, disableStore, allowSafetyIdentifier); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "保存失败")
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("保存失败"), http.StatusFound)
		return
	}

	if isAjax(r) {
		ajaxOK(w, "请求字段策略已保存")
		return
	}
	http.Redirect(w, r, returnTo+"?msg="+url.QueryEscape("请求字段策略已保存"), http.StatusFound)
}
