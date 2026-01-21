package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"realms/internal/store"
)

func (s *Server) ChannelGroupDetail(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
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

	breadcrumb, err := s.groupBreadcrumb(r.Context(), g.ID)
	if err != nil {
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}
	members, err := s.st.ListChannelGroupMembers(r.Context(), g.ID)
	if err != nil {
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}
	channelGroups, err := s.st.ListChannelGroups(r.Context())
	if err != nil {
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}
	channels, err := s.st.ListUpstreamChannels(r.Context())
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

	s.render(w, "admin_channel_group_detail", s.withFeatures(r.Context(), templateData{
		Title:               "渠道组 - Realms",
		Error:               errMsg,
		Notice:              notice,
		User:                u,
		IsRoot:              isRoot,
		CSRFToken:           csrf,
		CurrentGroup:        &g,
		GroupBreadcrumb:     breadcrumb,
		ChannelGroupMembers: members,
		ChannelGroups:       channelGroups,
		Channels:            channels,
	}))
}

func (s *Server) groupBreadcrumb(ctx context.Context, groupID int64) ([]store.ChannelGroup, error) {
	if groupID == 0 {
		return nil, errors.New("groupID 不能为空")
	}
	var chain []store.ChannelGroup
	cur := groupID
	for i := 0; i < 32; i++ {
		g, err := s.st.GetChannelGroupByID(ctx, cur)
		if err != nil {
			return nil, err
		}
		chain = append(chain, g)
		if strings.TrimSpace(g.Name) == store.DefaultGroupName {
			break
		}
		parentID, ok, err := s.st.GetChannelGroupParentID(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		if !ok || parentID == 0 {
			break
		}
		cur = parentID
	}
	// 反转为 root -> leaf
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

func (s *Server) CreateChildChannelGroup(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	parentID, err := parseInt64(r.PathValue("group_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
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
		http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?err="+url.QueryEscape("name 不能为空"), http.StatusFound)
		return
	}
	name, err := normalizeSingleGroup(nameRaw)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?err="+url.QueryEscape(err.Error()), http.StatusFound)
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
			http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?err="+url.QueryEscape("price_multiplier 不合法"), http.StatusFound)
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
		http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?err="+url.QueryEscape("max_attempts 不合法"), http.StatusFound)
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
		http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?err="+url.QueryEscape("status 不合法"), http.StatusFound)
		return
	}

	id, err := s.st.CreateChannelGroup(r.Context(), name, desc, status, priceMult, maxAttempts)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "创建失败（可能分组已存在）")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?err="+url.QueryEscape("创建失败（可能分组已存在）"), http.StatusFound)
		return
	}
	if err := s.st.AddChannelGroupMemberGroup(r.Context(), parentID, id, 0, false); err != nil {
		_ = s.st.DeleteChannelGroup(r.Context(), id)
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}

	if isAjax(r) {
		ajaxOK(w, "已创建")
		return
	}
	http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?msg="+url.QueryEscape("已创建"), http.StatusFound)
}

func (s *Server) AddChannelGroupChannelMember(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	parentID, err := parseInt64(r.PathValue("group_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	channelID, err := parseInt64(strings.TrimSpace(r.FormValue("channel_id")))
	if err != nil || channelID == 0 {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "channel_id 不合法")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?err="+url.QueryEscape("channel_id 不合法"), http.StatusFound)
		return
	}
	ch, err := s.st.GetUpstreamChannelByID(r.Context(), channelID)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusNotFound, "channel 不存在")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?err="+url.QueryEscape("channel 不存在"), http.StatusFound)
		return
	}
	if err := s.st.AddChannelGroupMemberChannel(r.Context(), parentID, channelID, ch.Priority, ch.Promotion); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已添加")
		return
	}
	http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?msg="+url.QueryEscape("已添加"), http.StatusFound)
}

func (s *Server) DeleteChannelGroupGroupMember(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	parentID, err := parseInt64(r.PathValue("group_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	childID, err := parseInt64(r.PathValue("child_group_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := s.st.RemoveChannelGroupMemberGroup(r.Context(), parentID, childID); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "删除失败")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?err="+url.QueryEscape("删除失败"), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已移除")
		return
	}
	http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?msg="+url.QueryEscape("已移除"), http.StatusFound)
}

func (s *Server) DeleteChannelGroupChannelMember(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	parentID, err := parseInt64(r.PathValue("group_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	channelID, err := parseInt64(r.PathValue("channel_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := s.st.RemoveChannelGroupMemberChannel(r.Context(), parentID, channelID); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "删除失败")
			return
		}
		http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?err="+url.QueryEscape("删除失败"), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已移除")
		return
	}
	http.Redirect(w, r, "/admin/channel-groups/"+strconv.FormatInt(parentID, 10)+"?msg="+url.QueryEscape("已移除"), http.StatusFound)
}

func (s *Server) ReorderChannelGroupMembers(w http.ResponseWriter, r *http.Request) {
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
	var ids []int64
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		http.Error(w, "无效的 JSON 数据", http.StatusBadRequest)
		return
	}
	if len(ids) > 0 {
		if err := s.st.ReorderChannelGroupMembers(r.Context(), groupID, ids); err != nil {
			http.Error(w, "排序保存失败", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}
