// Announcements 提供公告列表/详情与已读标记（用户侧只读）。
package web

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"realms/internal/auth"
)

type AnnouncementListItemView struct {
	ID        int64
	Title     string
	CreatedAt string
	Read      bool
}

type AnnouncementDetailView struct {
	ID        int64
	Title     string
	Body      string
	CreatedAt string
}

func (s *Server) AnnouncementsPage(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	rows, err := s.store.ListPublishedAnnouncementsWithRead(r.Context(), p.UserID, 200)
	if err != nil {
		http.Error(w, "查询公告失败", http.StatusInternalServerError)
		return
	}
	unread, err := s.store.CountUnreadAnnouncements(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "查询公告失败", http.StatusInternalServerError)
		return
	}

	items := make([]AnnouncementListItemView, 0, len(rows))
	for _, a := range rows {
		items = append(items, AnnouncementListItemView{
			ID:        a.ID,
			Title:     a.Title,
			CreatedAt: a.CreatedAt.Format("2006-01-02 15:04"),
			Read:      a.ReadAt != nil,
		})
	}

	s.Render(w, "page_announcements", s.withFeatures(r.Context(), TemplateData{
		Title:                    "公告 - Realms",
		User:                     userViewFromUser(u),
		CSRFToken:                csrfToken(p),
		Announcements:            items,
		UnreadAnnouncementsCount: int(unread),
	}))
}

func (s *Server) AnnouncementDetailPage(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	announcementID, err := parseInt64(strings.TrimSpace(r.PathValue("announcement_id")))
	if err != nil || announcementID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	a, err := s.store.GetPublishedAnnouncementByID(r.Context(), announcementID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "查询公告失败", http.StatusInternalServerError)
		return
	}

	_ = s.store.MarkAnnouncementRead(r.Context(), p.UserID, announcementID) // 幂等：忽略重复已读

	unread, err := s.store.CountUnreadAnnouncements(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "查询公告失败", http.StatusInternalServerError)
		return
	}

	s.Render(w, "page_announcement", s.withFeatures(r.Context(), TemplateData{
		Title:     a.Title + " - 公告 - Realms",
		User:      userViewFromUser(u),
		CSRFToken: csrfToken(p),
		Announcement: &AnnouncementDetailView{
			ID:        a.ID,
			Title:     a.Title,
			Body:      a.Body,
			CreatedAt: a.CreatedAt.Format("2006-01-02 15:04"),
		},
		UnreadAnnouncementsCount: int(unread),
	}))
}

func (s *Server) AnnouncementMarkRead(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	announcementID, err := parseInt64(strings.TrimSpace(r.PathValue("announcement_id")))
	if err != nil || announcementID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	// best-effort 校验公告存在且为已发布；避免将任意 id 写入已读表。
	if _, err := s.store.GetPublishedAnnouncementByID(r.Context(), announcementID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "查询公告失败", http.StatusInternalServerError)
		return
	}

	if err := s.store.MarkAnnouncementRead(r.Context(), p.UserID, announcementID); err != nil {
		http.Error(w, "标记已读失败", http.StatusInternalServerError)
		return
	}

	target := strings.TrimSpace(r.FormValue("redirect"))
	if target == "" {
		target = "/dashboard"
	}
	if !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") || strings.Contains(target, "://") {
		target = "/dashboard"
	}
	// 避免意外把查询串中的中文暴露在 Location；保持与现有页面一致用 QueryEscape。
	target = (&url.URL{Path: target}).String()
	http.Redirect(w, r, target, http.StatusFound)
}
