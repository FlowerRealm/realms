// Announcements 提供公告管理入口：创建/发布/撤回/删除公告（仅 root）。
package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"realms/internal/store"
)

const (
	announcementTitleMaxLen = 200
	announcementBodyMaxLen  = 8000
)

type adminAnnouncementView struct {
	ID        int64
	Title     string
	Status    int
	CreatedAt string
	UpdatedAt string
}

func (s *Server) Announcements(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	loc, _ := s.adminTimeLocation(r.Context())
	items, err := s.st.ListAnnouncements(r.Context())
	if err != nil {
		http.Error(w, "查询公告失败", http.StatusInternalServerError)
		return
	}

	views := make([]adminAnnouncementView, 0, len(items))
	for _, a := range items {
		title := a.Title
		if len([]rune(title)) > announcementTitleMaxLen {
			title = string([]rune(title)[:announcementTitleMaxLen]) + "..."
		}
		views = append(views, adminAnnouncementView{
			ID:        a.ID,
			Title:     title,
			Status:    a.Status,
			CreatedAt: formatTimeIn(a.CreatedAt, "2006-01-02 15:04", loc),
			UpdatedAt: formatTimeIn(a.UpdatedAt, "2006-01-02 15:04", loc),
		})
	}

	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}

	s.render(w, "admin_announcements", s.withFeatures(r.Context(), templateData{
		Title:         "公告 - Realms",
		Error:         errMsg,
		Notice:        notice,
		User:          u,
		IsRoot:        isRoot,
		CSRFToken:     csrf,
		Announcements: views,
	}))
}

func (s *Server) CreateAnnouncement(w http.ResponseWriter, r *http.Request) {
	if _, _, _, err := s.currentUser(r); err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	if title == "" || body == "" {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "标题与内容不能为空")
			return
		}
		http.Redirect(w, r, "/admin/announcements?err="+url.QueryEscape("标题与内容不能为空"), http.StatusFound)
		return
	}
	if len([]rune(title)) > announcementTitleMaxLen {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "标题过长")
			return
		}
		http.Redirect(w, r, "/admin/announcements?err="+url.QueryEscape("标题过长"), http.StatusFound)
		return
	}
	if len([]rune(body)) > announcementBodyMaxLen {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "内容过长")
			return
		}
		http.Redirect(w, r, "/admin/announcements?err="+url.QueryEscape("内容过长"), http.StatusFound)
		return
	}

	status, err := parseInt(r.FormValue("status"))
	if err != nil || (status != store.AnnouncementStatusDraft && status != store.AnnouncementStatusPublished) {
		status = store.AnnouncementStatusPublished
	}

	if _, err := s.st.CreateAnnouncement(r.Context(), title, body, status); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "创建失败")
			return
		}
		http.Redirect(w, r, "/admin/announcements?err="+url.QueryEscape("创建失败"), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已创建")
		return
	}
	http.Redirect(w, r, "/admin/announcements?msg="+url.QueryEscape("已创建"), http.StatusFound)
}

func (s *Server) UpdateAnnouncementStatus(w http.ResponseWriter, r *http.Request) {
	if _, _, _, err := s.currentUser(r); err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	announcementID, err := parseInt64(strings.TrimSpace(r.PathValue("announcement_id")))
	if err != nil || announcementID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	status, err := parseInt(r.FormValue("status"))
	if err != nil || (status != store.AnnouncementStatusDraft && status != store.AnnouncementStatusPublished) {
		http.Error(w, "status 不合法", http.StatusBadRequest)
		return
	}
	if err := s.st.UpdateAnnouncementStatus(r.Context(), announcementID, status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "保存失败")
			return
		}
		http.Redirect(w, r, "/admin/announcements?err="+url.QueryEscape("保存失败"), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已保存")
		return
	}
	http.Redirect(w, r, "/admin/announcements?msg="+url.QueryEscape("已保存"), http.StatusFound)
}

func (s *Server) DeleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	if _, _, _, err := s.currentUser(r); err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	announcementID, err := parseInt64(strings.TrimSpace(r.PathValue("announcement_id")))
	if err != nil || announcementID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := s.st.DeleteAnnouncement(r.Context(), announcementID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "删除失败")
			return
		}
		http.Redirect(w, r, "/admin/announcements?err="+url.QueryEscape("删除失败"), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已删除")
		return
	}
	http.Redirect(w, r, "/admin/announcements?msg="+url.QueryEscape("已删除"), http.StatusFound)
}
