package admin

import (
	"net/http"
	"net/url"
	"strings"
)

func parseOptionalLimitInt(raw string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	n, err := parseInt(raw)
	if err != nil {
		return nil, err
	}
	if n <= 0 {
		return nil, nil
	}
	v := n
	return &v, nil
}

func (s *Server) UpdateChannelLimits(w http.ResponseWriter, r *http.Request) {
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

	limitSessionsRaw := strings.TrimSpace(r.FormValue("limit_sessions"))
	// 兼容旧字段名：limit_cc
	if limitSessionsRaw == "" {
		limitSessionsRaw = r.FormValue("limit_cc")
	}
	limitSessions, err := parseOptionalLimitInt(limitSessionsRaw)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "limit_sessions 不合法")
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("limit_sessions 不合法"), http.StatusFound)
		return
	}
	limitRPM, err := parseOptionalLimitInt(r.FormValue("limit_rpm"))
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "limit_rpm 不合法")
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("limit_rpm 不合法"), http.StatusFound)
		return
	}
	limitTPM, err := parseOptionalLimitInt(r.FormValue("limit_tpm"))
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "limit_tpm 不合法")
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("limit_tpm 不合法"), http.StatusFound)
		return
	}

	if err := s.st.UpdateUpstreamChannelLimits(r.Context(), channelID, limitSessions, limitRPM, limitTPM); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "保存失败")
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("保存失败"), http.StatusFound)
		return
	}

	if isAjax(r) {
		ajaxOK(w, "限额已保存")
		return
	}
	http.Redirect(w, r, returnTo+"?msg="+url.QueryEscape("限额已保存"), http.StatusFound)
}
