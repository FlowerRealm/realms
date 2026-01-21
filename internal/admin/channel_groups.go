package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"realms/internal/store"
)

func (s *Server) ChannelGroups(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	groups, err := s.st.ListChannelGroups(r.Context())
	if err != nil {
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}

	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}

	s.render(w, "admin_channel_groups", s.withFeatures(r.Context(), templateData{
		Title:         "分组 - Realms",
		Error:         errMsg,
		Notice:        notice,
		User:          u,
		IsRoot:        isRoot,
		CSRFToken:     csrf,
		ChannelGroups: groups,
	}))
}

func (s *Server) CreateChannelGroup(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	nameRaw := strings.TrimSpace(r.FormValue("name"))
	if nameRaw == "" {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "name 不能为空")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups?err="+url.QueryEscape("name 不能为空"), http.StatusFound)
		return
	}
	name, err := normalizeSingleGroup(nameRaw)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, "/admin/channel-groups?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}

	descRaw := strings.TrimSpace(r.FormValue("description"))
	var desc *string
	if descRaw != "" {
		v := descRaw
		desc = &v
	}

	priceMult := store.DefaultGroupPriceMultiplier
	if v := strings.TrimSpace(r.FormValue("price_multiplier")); v != "" {
		m, err := parseMultiplier(v)
		if err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "price_multiplier 不合法")
				return
			}
			http.Redirect(w, r, "/admin/channel-groups?err="+url.QueryEscape("price_multiplier 不合法"), http.StatusFound)
			return
		}
		priceMult = m
	}

	maxAttempts, err := parseInt(r.FormValue("max_attempts"))
	if err != nil {
		maxAttempts = 5
	}
	if maxAttempts <= 0 {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "max_attempts 不合法")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups?err="+url.QueryEscape("max_attempts 不合法"), http.StatusFound)
		return
	}

	status, err := parseInt(r.FormValue("status"))
	if err != nil {
		status = 1
	}
	if status != 0 && status != 1 {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "status 不合法")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups?err="+url.QueryEscape("status 不合法"), http.StatusFound)
		return
	}

	if _, err := s.st.CreateChannelGroup(r.Context(), name, desc, status, priceMult, maxAttempts); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "创建失败（可能分组已存在）")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups?err="+url.QueryEscape("创建失败（可能分组已存在）"), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已创建")
		return
	}
	http.Redirect(w, r, "/admin/channel-groups?msg="+url.QueryEscape("已创建"), http.StatusFound)
}

func (s *Server) UpdateChannelGroup(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	groupID, err := parseInt64(r.PathValue("group_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	g, err := s.st.GetChannelGroupByID(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "分组不存在", http.StatusNotFound)
			return
		}
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}

	status, err := parseInt(r.FormValue("status"))
	if err != nil {
		status = g.Status
	}
	if status != 0 && status != 1 {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "status 不合法")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups?err="+url.QueryEscape("status 不合法"), http.StatusFound)
		return
	}
	if isDefaultGroup(g.Name) && status != 1 {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "default 分组不允许禁用")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups?err="+url.QueryEscape("default 分组不允许禁用"), http.StatusFound)
		return
	}

	descRaw := strings.TrimSpace(r.FormValue("description"))
	var desc *string
	if descRaw != "" {
		v := descRaw
		desc = &v
	}

	priceMult := g.PriceMultiplier
	if v := strings.TrimSpace(r.FormValue("price_multiplier")); v != "" {
		m, err := parseMultiplier(v)
		if err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "price_multiplier 不合法")
				return
			}
			http.Redirect(w, r, "/admin/channel-groups?err="+url.QueryEscape("price_multiplier 不合法"), http.StatusFound)
			return
		}
		priceMult = m
	}

	maxAttempts, err := parseInt(r.FormValue("max_attempts"))
	if err != nil {
		maxAttempts = g.MaxAttempts
	}
	if maxAttempts <= 0 {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "max_attempts 不合法")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups?err="+url.QueryEscape("max_attempts 不合法"), http.StatusFound)
		return
	}

	if err := s.st.UpdateChannelGroup(r.Context(), g.ID, desc, status, priceMult, maxAttempts); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "保存失败")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups?err="+url.QueryEscape("保存失败"), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已保存")
		return
	}
	http.Redirect(w, r, "/admin/channel-groups?msg="+url.QueryEscape("已保存"), http.StatusFound)
}

func (s *Server) DeleteChannelGroup(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	groupID, err := parseInt64(r.PathValue("group_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	g, err := s.st.GetChannelGroupByID(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "分组不存在", http.StatusNotFound)
			return
		}
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}
	if isDefaultGroup(g.Name) {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "default 分组不允许删除")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups?err="+url.QueryEscape("default 分组不允许删除"), http.StatusFound)
		return
	}

	sum, err := s.st.ForceDeleteChannelGroup(r.Context(), g.ID)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "删除失败")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups?err="+url.QueryEscape("删除失败"), http.StatusFound)
		return
	}
	if isAjax(r) {
		msg := "已删除"
		if sum.UsersUnbound > 0 || sum.ChannelsUpdated > 0 {
			msg += "（已解绑 users=" + strconv.FormatInt(sum.UsersUnbound, 10)
			if sum.ChannelsUpdated > 0 {
				msg += ", channels=" + strconv.FormatInt(sum.ChannelsUpdated, 10)
			}
			if sum.ChannelsDisabled > 0 {
				msg += ", disabled=" + strconv.FormatInt(sum.ChannelsDisabled, 10)
			}
			msg += "）"
		}
		ajaxOK(w, msg)
		return
	}
	notice := "已删除"
	if sum.UsersUnbound > 0 || sum.ChannelsUpdated > 0 {
		notice += "（已解绑 users=" + strconv.FormatInt(sum.UsersUnbound, 10)
		if sum.ChannelsUpdated > 0 {
			notice += ", channels=" + strconv.FormatInt(sum.ChannelsUpdated, 10)
		}
		if sum.ChannelsDisabled > 0 {
			notice += ", disabled=" + strconv.FormatInt(sum.ChannelsDisabled, 10)
		}
		notice += "）"
	}
	http.Redirect(w, r, "/admin/channel-groups?msg="+url.QueryEscape(notice), http.StatusFound)
}
