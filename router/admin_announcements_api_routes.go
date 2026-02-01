package router

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"realms/internal/store"
)

const (
	adminAnnouncementTitleMaxLen = 200
	adminAnnouncementBodyMaxLen  = 8000
)

type adminAnnouncementView struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Status    int    `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func setAdminAnnouncementAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/announcements", adminListAnnouncementsHandler(opts))
	r.POST("/announcements", adminCreateAnnouncementHandler(opts))
	r.PUT("/announcements/:announcement_id", adminUpdateAnnouncementStatusHandler(opts))
	r.DELETE("/announcements/:announcement_id", adminDeleteAnnouncementHandler(opts))
}

func adminAnnouncementsFeatureDisabled(c *gin.Context, opts Options) bool {
	if c == nil || opts.Store == nil {
		return false
	}
	if opts.Store.FeatureDisabledEffective(c.Request.Context(), opts.SelfMode, store.SettingFeatureDisableAdminAnnouncements) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
		return true
	}
	return false
}

func adminListAnnouncementsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminAnnouncementsFeatureDisabled(c, opts) {
			return
		}

		items, err := opts.Store.ListAnnouncements(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询公告失败"})
			return
		}

		out := make([]adminAnnouncementView, 0, len(items))
		for _, a := range items {
			title := a.Title
			if len([]rune(title)) > adminAnnouncementTitleMaxLen {
				title = string([]rune(title)[:adminAnnouncementTitleMaxLen]) + "..."
			}
			out = append(out, adminAnnouncementView{
				ID:        a.ID,
				Title:     title,
				Body:      a.Body,
				Status:    a.Status,
				CreatedAt: a.CreatedAt.Format("2006-01-02 15:04"),
				UpdatedAt: a.UpdatedAt.Format("2006-01-02 15:04"),
			})
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func adminCreateAnnouncementHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Title  string `json:"title"`
		Body   string `json:"body"`
		Status int    `json:"status"`
	}

	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminAnnouncementsFeatureDisabled(c, opts) {
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		title := strings.TrimSpace(req.Title)
		body := strings.TrimSpace(req.Body)
		if title == "" || body == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "标题与内容不能为空"})
			return
		}
		if len([]rune(title)) > adminAnnouncementTitleMaxLen {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "标题过长"})
			return
		}
		if len([]rune(body)) > adminAnnouncementBodyMaxLen {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "内容过长"})
			return
		}

		status := req.Status
		if status != store.AnnouncementStatusDraft && status != store.AnnouncementStatusPublished {
			status = store.AnnouncementStatusPublished
		}

		id, err := opts.Store.CreateAnnouncement(c.Request.Context(), title, body, status)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已创建", "data": gin.H{"id": id}})
	}
}

func adminUpdateAnnouncementStatusHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Status int `json:"status"`
	}

	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminAnnouncementsFeatureDisabled(c, opts) {
			return
		}
		announcementID, err := strconv.ParseInt(strings.TrimSpace(c.Param("announcement_id")), 10, 64)
		if err != nil || announcementID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "announcement_id 不合法"})
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		status := req.Status
		if status != store.AnnouncementStatusDraft && status != store.AnnouncementStatusPublished {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "status 不合法"})
			return
		}

		if err := opts.Store.UpdateAnnouncementStatus(c.Request.Context(), announcementID, status); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func adminDeleteAnnouncementHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminAnnouncementsFeatureDisabled(c, opts) {
			return
		}

		announcementID, err := strconv.ParseInt(strings.TrimSpace(c.Param("announcement_id")), 10, 64)
		if err != nil || announcementID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "announcement_id 不合法"})
			return
		}

		if err := opts.Store.DeleteAnnouncement(c.Request.Context(), announcementID); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
	}
}
