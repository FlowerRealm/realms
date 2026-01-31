package router

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type announcementListItemAPI struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	Read      bool   `json:"read"`
}

type announcementsListAPIResponse struct {
	UnreadCount int64                    `json:"unread_count"`
	Items       []announcementListItemAPI `json:"items"`
}

type announcementDetailAPIResponse struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

func setAnnouncementAPIRoutes(r gin.IRoutes, opts Options) {
	authn := requireUserSession(opts)

	r.GET("/announcements", authn, announcementsListHandler(opts))
	r.GET("/announcements/:announcement_id", authn, announcementDetailHandler(opts))
	r.POST("/announcements/:announcement_id/read", authn, announcementMarkReadHandler(opts))
}

func announcementsListHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		limit := 200
		if v := strings.TrimSpace(c.Query("limit")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
				limit = n
			}
		}

		rows, err := opts.Store.ListPublishedAnnouncementsWithRead(c.Request.Context(), userID, limit)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询公告失败"})
			return
		}
		unread, err := opts.Store.CountUnreadAnnouncements(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询公告失败"})
			return
		}

		items := make([]announcementListItemAPI, 0, len(rows))
		for _, a := range rows {
			items = append(items, announcementListItemAPI{
				ID:        a.ID,
				Title:     a.Title,
				CreatedAt: a.CreatedAt.Format("2006-01-02 15:04"),
				Read:      a.ReadAt != nil,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": announcementsListAPIResponse{
				UnreadCount: unread,
				Items:       items,
			},
		})
	}
}

func announcementDetailHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		idStr := strings.TrimSpace(c.Param("announcement_id"))
		announcementID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || announcementID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}

		a, err := opts.Store.GetPublishedAnnouncementByID(c.Request.Context(), announcementID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "公告不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询公告失败"})
			return
		}

		_ = opts.Store.MarkAnnouncementRead(c.Request.Context(), userID, announcementID)

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": announcementDetailAPIResponse{
				ID:        a.ID,
				Title:     a.Title,
				Body:      a.Body,
				CreatedAt: a.CreatedAt.Format("2006-01-02 15:04"),
			},
		})
	}
}

func announcementMarkReadHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		idStr := strings.TrimSpace(c.Param("announcement_id"))
		announcementID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || announcementID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}

		// best-effort 校验公告存在且为已发布；避免将任意 id 写入已读表。
		if _, err := opts.Store.GetPublishedAnnouncementByID(c.Request.Context(), announcementID); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "公告不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询公告失败"})
			return
		}

		if err := opts.Store.MarkAnnouncementRead(c.Request.Context(), userID, announcementID); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "标记已读失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}
