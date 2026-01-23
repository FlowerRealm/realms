package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) PromoteChannel5Min(w http.ResponseWriter, r *http.Request) {
	_, _, isRoot, err := s.currentUser(r)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusUnauthorized, "未登录")
			return
		}
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !isRoot {
		if isAjax(r) {
			ajaxError(w, http.StatusForbidden, "无权限")
			return
		}
		http.Error(w, "无权限", http.StatusForbidden)
		return
	}
	if s.sched == nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "调度器未初始化")
			return
		}
		http.Error(w, "调度器未初始化", http.StatusInternalServerError)
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

	channelID, err := parseInt64(strings.TrimSpace(r.PathValue("channel_id")))
	if err != nil || channelID <= 0 {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "channel_id 不合法")
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("channel_id 不合法"), http.StatusFound)
		return
	}
	ch, err := s.st.GetUpstreamChannelByID(r.Context(), channelID)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusNotFound, "channel 不存在")
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("channel 不存在"), http.StatusFound)
		return
	}

	until := s.sched.ForceChannelFor(ch.ID, 5*time.Minute)
	s.sched.ClearChannelBan(ch.ID)

	loc, _ := s.adminTimeLocation(r.Context())
	msg := fmt.Sprintf("已设置 %s 为 5 分钟最高优先级（到期 %s）", ch.Name, formatTimeIn(until, "15:04:05", loc))
	if isAjax(r) {
		ajaxOK(w, msg)
		return
	}
	http.Redirect(w, r, returnTo+"?msg="+url.QueryEscape(msg), http.StatusFound)
}

