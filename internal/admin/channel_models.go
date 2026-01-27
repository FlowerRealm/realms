package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"realms/internal/store"
)

type channelModelView struct {
	ID            int64
	ChannelID     int64
	PublicID      string
	UpstreamModel string
	Status        int
	CreatedAt     string
	UpdatedAt     string
}

func toChannelModelView(m store.ChannelModel, loc *time.Location) channelModelView {
	return channelModelView{
		ID:            m.ID,
		ChannelID:     m.ChannelID,
		PublicID:      m.PublicID,
		UpstreamModel: m.UpstreamModel,
		Status:        m.Status,
		CreatedAt:     formatTimeIn(m.CreatedAt, "2006-01-02 15:04", loc),
		UpdatedAt:     formatTimeIn(m.UpdatedAt, "2006-01-02 15:04", loc),
	}
}

func (s *Server) ChannelModels(w http.ResponseWriter, r *http.Request) {
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
	if _, err := s.st.GetUpstreamChannelByID(r.Context(), channelID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Channel 不存在", http.StatusNotFound)
			return
		}
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}

	q := r.URL.Query()
	q.Set("open_channel_settings", fmt.Sprintf("%d", channelID))

	target := "/admin/channels"
	if enc := q.Encode(); enc != "" {
		target += "?" + enc
	}
	target += "#models"

	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) CreateChannelModel(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	publicID := strings.TrimSpace(r.FormValue("public_id"))
	upstreamModel := strings.TrimSpace(r.FormValue("upstream_model"))
	status, err := parseInt(r.FormValue("status"))
	if err != nil {
		status = 1
	}

	if publicID == "" {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "public_id 不能为空")
			return
		}
		http.Redirect(w, r, "/admin/channels?open_channel_settings="+fmt.Sprintf("%d", channelID)+"&err="+url.QueryEscape("public_id 不能为空")+"#models", http.StatusFound)
		return
	}
	if upstreamModel == "" {
		upstreamModel = publicID
	}
	if status != 0 && status != 1 {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "status 不合法")
			return
		}
		http.Redirect(w, r, "/admin/channels?open_channel_settings="+fmt.Sprintf("%d", channelID)+"&err="+url.QueryEscape("status 不合法")+"#models", http.StatusFound)
		return
	}

	if _, err := s.st.GetUpstreamChannelByID(r.Context(), channelID); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "channel_id 不存在")
			return
		}
		http.Redirect(w, r, "/admin/channels?open_channel_settings="+fmt.Sprintf("%d", channelID)+"&err="+url.QueryEscape("channel_id 不存在")+"#models", http.StatusFound)
		return
	}
	if _, err := s.st.GetManagedModelByPublicID(r.Context(), publicID); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "public_id 不存在，请先在模型管理创建")
			return
		}
		http.Redirect(w, r, "/admin/channels?open_channel_settings="+fmt.Sprintf("%d", channelID)+"&err="+url.QueryEscape("public_id 不存在，请先在模型管理创建")+"#models", http.StatusFound)
		return
	}

	if _, err := s.st.CreateChannelModel(r.Context(), store.ChannelModelCreate{
		ChannelID:     channelID,
		PublicID:      publicID,
		UpstreamModel: upstreamModel,
		Status:        status,
	}); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "创建失败")
			return
		}
		http.Redirect(w, r, "/admin/channels?open_channel_settings="+fmt.Sprintf("%d", channelID)+"&err="+url.QueryEscape("创建失败")+"#models", http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已创建")
		return
	}
	http.Redirect(w, r, "/admin/channels?open_channel_settings="+fmt.Sprintf("%d", channelID)+"&msg="+url.QueryEscape("已创建")+"#models", http.StatusFound)
}

func (s *Server) ChannelModel(w http.ResponseWriter, r *http.Request) {
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
	bindingID, err := parseInt64(r.PathValue("binding_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	_, err = s.st.GetUpstreamChannelByID(r.Context(), channelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Channel 不存在", http.StatusNotFound)
			return
		}
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}
	cm, err := s.st.GetChannelModelByID(r.Context(), bindingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "绑定不存在", http.StatusNotFound)
			return
		}
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}
	if cm.ChannelID != channelID {
		http.Error(w, "绑定不存在", http.StatusNotFound)
		return
	}

	q := r.URL.Query()
	q.Set("open_channel_settings", fmt.Sprintf("%d", channelID))

	target := "/admin/channels"
	if enc := q.Encode(); enc != "" {
		target += "?" + enc
	}
	target += "#models"

	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) UpdateChannelModel(w http.ResponseWriter, r *http.Request) {
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
	bindingID, err := parseInt64(r.PathValue("binding_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	publicID := strings.TrimSpace(r.FormValue("public_id"))
	upstreamModel := strings.TrimSpace(r.FormValue("upstream_model"))
	status, err := parseInt(r.FormValue("status"))
	if err != nil {
		http.Error(w, "status 不合法", http.StatusBadRequest)
		return
	}

	if publicID == "" {
		http.Error(w, "public_id 不能为空", http.StatusBadRequest)
		return
	}
	if upstreamModel == "" {
		upstreamModel = publicID
	}
	if status != 0 && status != 1 {
		http.Error(w, "status 不合法", http.StatusBadRequest)
		return
	}

	cm, err := s.st.GetChannelModelByID(r.Context(), bindingID)
	if err != nil {
		http.Error(w, "绑定不存在", http.StatusNotFound)
		return
	}
	if cm.ChannelID != channelID {
		http.Error(w, "绑定不存在", http.StatusNotFound)
		return
	}
	if _, err := s.st.GetManagedModelByPublicID(r.Context(), publicID); err != nil {
		http.Error(w, "public_id 不存在，请先在模型管理创建", http.StatusBadRequest)
		return
	}

	if err := s.st.UpdateChannelModel(r.Context(), store.ChannelModelUpdate{
		ID:            bindingID,
		ChannelID:     channelID,
		PublicID:      publicID,
		UpstreamModel: upstreamModel,
		Status:        status,
	}); err != nil {
		http.Error(w, "更新失败", http.StatusInternalServerError)
		return
	}

	if isAjax(r) {
		ajaxOK(w, "已保存")
		return
	}
	http.Redirect(w, r, "/admin/channels?open_channel_settings="+fmt.Sprintf("%d", channelID)+"&msg="+url.QueryEscape("已保存")+"#models", http.StatusFound)
}

func (s *Server) DeleteChannelModel(w http.ResponseWriter, r *http.Request) {
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
	bindingID, err := parseInt64(r.PathValue("binding_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	cm, err := s.st.GetChannelModelByID(r.Context(), bindingID)
	if err != nil {
		http.Error(w, "绑定不存在", http.StatusNotFound)
		return
	}
	if cm.ChannelID != channelID {
		http.Error(w, "绑定不存在", http.StatusNotFound)
		return
	}

	if err := s.st.DeleteChannelModel(r.Context(), bindingID); err != nil {
		http.Error(w, "删除失败", http.StatusInternalServerError)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已删除")
		return
	}
	http.Redirect(w, r, "/admin/channels?open_channel_settings="+fmt.Sprintf("%d", channelID)+"&msg="+url.QueryEscape("已删除")+"#models", http.StatusFound)
}
