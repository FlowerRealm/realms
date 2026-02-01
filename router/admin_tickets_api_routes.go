package router

import (
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"realms/internal/store"
	ticketspkg "realms/internal/tickets"
)

const adminTicketBodyMaxLen = 8000

type adminTicketListItemView struct {
	ID            int64  `json:"id"`
	UserEmail     string `json:"user_email"`
	Subject       string `json:"subject"`
	StatusText    string `json:"status_text"`
	StatusBadge   string `json:"status_badge"`
	LastMessageAt string `json:"last_message_at"`
	CreatedAt     string `json:"created_at"`
}

type adminTicketDetailView struct {
	ID            int64  `json:"id"`
	UserEmail     string `json:"user_email"`
	Subject       string `json:"subject"`
	StatusText    string `json:"status_text"`
	StatusBadge   string `json:"status_badge"`
	LastMessageAt string `json:"last_message_at"`
	CreatedAt     string `json:"created_at"`
	ClosedAt      string `json:"closed_at"`
	CanReply      bool   `json:"can_reply"`
	Closed        bool   `json:"closed"`
}

type adminTicketAttachmentView struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Size      string `json:"size"`
	ExpiresAt string `json:"expires_at"`
	URL       string `json:"url"`
}

type adminTicketMessageView struct {
	ID          int64                       `json:"id"`
	Actor       string                      `json:"actor"`
	ActorMeta   string                      `json:"actor_meta"`
	Body        string                      `json:"body"`
	CreatedAt   string                      `json:"created_at"`
	Attachments []adminTicketAttachmentView `json:"attachments,omitempty"`
}

type adminTicketDetailAPIResponse struct {
	Ticket   adminTicketDetailView    `json:"ticket"`
	Messages []adminTicketMessageView `json:"messages"`
}

func setAdminTicketAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/tickets", adminTicketsListHandler(opts))
	r.GET("/tickets/:ticket_id", adminTicketDetailHandler(opts))
	r.POST("/tickets/:ticket_id/reply", adminTicketReplyHandler(opts))
	r.POST("/tickets/:ticket_id/close", adminTicketCloseHandler(opts))
	r.POST("/tickets/:ticket_id/reopen", adminTicketReopenHandler(opts))
	r.GET("/tickets/:ticket_id/attachments/:attachment_id", adminTicketAttachmentDownloadHandler(opts))
}

func adminTicketsFeatureDisabled(c *gin.Context, opts Options) bool {
	if c == nil || opts.Store == nil {
		return false
	}
	if opts.Store.FeatureDisabledEffective(c.Request.Context(), opts.SelfMode, store.SettingFeatureDisableTickets) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
		return true
	}
	return false
}

func adminTicketStatusView(status int) (string, string) {
	switch status {
	case store.TicketStatusOpen:
		return "打开", "bg-success"
	case store.TicketStatusClosed:
		return "已关闭", "bg-secondary"
	default:
		return "未知", "bg-light text-secondary border"
	}
}

func adminTicketActorView(actorType string, email *string, username *string) (string, string) {
	switch actorType {
	case store.TicketActorTypeAdmin:
		if username != nil && strings.TrimSpace(*username) != "" {
			return "管理员", "@" + strings.TrimSpace(*username)
		}
		if email != nil && strings.TrimSpace(*email) != "" {
			return "管理员", strings.TrimSpace(*email)
		}
		return "管理员", ""
	case store.TicketActorTypeUser:
		if username != nil && strings.TrimSpace(*username) != "" {
			return "用户", "@" + strings.TrimSpace(*username)
		}
		if email != nil && strings.TrimSpace(*email) != "" {
			return "用户", strings.TrimSpace(*email)
		}
		return "用户", ""
	default:
		return "系统", ""
	}
}

func adminTicketDetailFromStore(t store.TicketWithOwner, loc *time.Location) adminTicketDetailView {
	st, badge := adminTicketStatusView(t.Status)
	closedAt := ""
	if t.ClosedAt != nil {
		closedAt = formatTimeIn(*t.ClosedAt, "2006-01-02 15:04", loc)
	}
	return adminTicketDetailView{
		ID:            t.ID,
		UserEmail:     t.OwnerEmail,
		Subject:       t.Subject,
		StatusText:    st,
		StatusBadge:   badge,
		LastMessageAt: formatTimeIn(t.LastMessageAt, "2006-01-02 15:04", loc),
		CreatedAt:     formatTimeIn(t.CreatedAt, "2006-01-02 15:04", loc),
		ClosedAt:      closedAt,
		CanReply:      t.Status == store.TicketStatusOpen,
		Closed:        t.Status == store.TicketStatusClosed,
	}
}

func adminTicketsListHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminTicketsFeatureDisabled(c, opts) {
			return
		}

		loc, _ := adminTimeLocation(c.Request.Context(), opts)

		var statusPtr *int
		switch strings.TrimSpace(c.Query("status")) {
		case "open":
			v := store.TicketStatusOpen
			statusPtr = &v
		case "closed":
			v := store.TicketStatusClosed
			statusPtr = &v
		default:
		}

		rows, err := opts.Store.ListTicketsForAdmin(c.Request.Context(), statusPtr)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询工单失败"})
			return
		}

		items := make([]adminTicketListItemView, 0, len(rows))
		for _, t := range rows {
			st, badge := adminTicketStatusView(t.Status)
			items = append(items, adminTicketListItemView{
				ID:            t.ID,
				UserEmail:     t.OwnerEmail,
				Subject:       t.Subject,
				StatusText:    st,
				StatusBadge:   badge,
				LastMessageAt: formatTimeIn(t.LastMessageAt, "2006-01-02 15:04", loc),
				CreatedAt:     formatTimeIn(t.CreatedAt, "2006-01-02 15:04", loc),
			})
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": items})
	}
}

func adminTicketDetailHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminTicketsFeatureDisabled(c, opts) {
			return
		}

		loc, _ := adminTimeLocation(c.Request.Context(), opts)

		ticketID, err := strconv.ParseInt(strings.TrimSpace(c.Param("ticket_id")), 10, 64)
		if err != nil || ticketID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}

		tk, err := opts.Store.GetTicketWithOwnerByID(c.Request.Context(), ticketID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询工单失败"})
			return
		}

		msgs, err := opts.Store.ListTicketMessagesWithActors(c.Request.Context(), ticketID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询工单消息失败"})
			return
		}
		atts, err := opts.Store.ListTicketAttachmentsByTicketID(c.Request.Context(), ticketID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询工单附件失败"})
			return
		}
		attsByMsg := make(map[int64][]store.TicketAttachment)
		for _, a := range atts {
			attsByMsg[a.MessageID] = append(attsByMsg[a.MessageID], a)
		}

		dv := adminTicketDetailFromStore(tk, loc)
		mv := make([]adminTicketMessageView, 0, len(msgs))
		for _, m := range msgs {
			actor, meta := adminTicketActorView(m.ActorType, m.ActorEmail, m.ActorUsername)
			av := make([]adminTicketAttachmentView, 0, len(attsByMsg[m.ID]))
			for _, a := range attsByMsg[m.ID] {
				av = append(av, adminTicketAttachmentView{
					ID:        a.ID,
					Name:      a.OriginalName,
					Size:      ticketspkg.FormatBytes(a.SizeBytes),
					ExpiresAt: formatTimeIn(a.ExpiresAt, "2006-01-02 15:04", loc),
					URL:       fmt.Sprintf("/api/admin/tickets/%d/attachments/%d", ticketID, a.ID),
				})
			}
			mv = append(mv, adminTicketMessageView{
				ID:          m.ID,
				Actor:       actor,
				ActorMeta:   meta,
				Body:        m.Body,
				CreatedAt:   formatTimeIn(m.CreatedAt, "2006-01-02 15:04", loc),
				Attachments: av,
			})
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": adminTicketDetailAPIResponse{Ticket: dv, Messages: mv}})
	}
}

func adminTicketReplyHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminTicketsFeatureDisabled(c, opts) {
			return
		}
		if opts.TicketStorage == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "ticket storage 未初始化"})
			return
		}

		ticketID, err := strconv.ParseInt(strings.TrimSpace(c.Param("ticket_id")), 10, 64)
		if err != nil || ticketID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}

		tk, err := opts.Store.GetTicketWithOwnerByID(c.Request.Context(), ticketID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询工单失败"})
			return
		}
		if tk.Status != store.TicketStatusOpen {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "工单已关闭，请先恢复"})
			return
		}

		if err := c.Request.ParseMultipartForm(ticketMultipartMemory); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": parseUploadError(err)})
			return
		}

		body := strings.TrimSpace(c.Request.FormValue("body"))
		if body == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "内容不能为空"})
			return
		}
		if len([]rune(body)) > adminTicketBodyMaxLen {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "内容过长"})
			return
		}

		now := time.Now()
		files := c.Request.MultipartForm.File["attachments"]

		actorID, _ := userIDFromContext(c)
		attInputs, saved, errMsg := saveTicketAttachments(opts.TicketStorage, now, &actorID, files)
		if errMsg != "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": errMsg})
			return
		}
		cleanupOK := true
		defer func() {
			if !cleanupOK {
				deleteSavedAttachments(opts.TicketStorage, saved)
			}
		}()

		if _, err := opts.Store.AddTicketMessageWithAttachments(c.Request.Context(), ticketID, store.TicketActorTypeAdmin, &actorID, body, attInputs); err != nil {
			cleanupOK = false
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "回复失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已发送"})
	}
}

func adminTicketCloseHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminTicketsFeatureDisabled(c, opts) {
			return
		}

		ticketID, err := strconv.ParseInt(strings.TrimSpace(c.Param("ticket_id")), 10, 64)
		if err != nil || ticketID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}
		if err := opts.Store.CloseTicket(c.Request.Context(), ticketID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "关闭失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已关闭"})
	}
}

func adminTicketReopenHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminTicketsFeatureDisabled(c, opts) {
			return
		}

		ticketID, err := strconv.ParseInt(strings.TrimSpace(c.Param("ticket_id")), 10, 64)
		if err != nil || ticketID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}
		if err := opts.Store.ReopenTicket(c.Request.Context(), ticketID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "恢复失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已恢复"})
	}
}

func adminTicketAttachmentDownloadHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminTicketsFeatureDisabled(c, opts) {
			return
		}
		if opts.TicketStorage == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "ticket storage 未初始化"})
			return
		}

		ticketID, err := strconv.ParseInt(strings.TrimSpace(c.Param("ticket_id")), 10, 64)
		if err != nil || ticketID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}
		attachmentID, err := strconv.ParseInt(strings.TrimSpace(c.Param("attachment_id")), 10, 64)
		if err != nil || attachmentID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}

		att, err := opts.Store.GetTicketAttachmentByID(c.Request.Context(), attachmentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询附件失败"})
			return
		}
		if att.TicketID != ticketID {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
			return
		}
		if time.Now().After(att.ExpiresAt) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
			return
		}

		full, err := opts.TicketStorage.Resolve(att.StorageRelPath)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
			return
		}
		f, err := os.Open(full)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
			return
		}
		defer func() { _ = f.Close() }()

		name := sanitizeDownloadName(att.OriginalName)
		c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		c.Writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		if att.ContentType != nil && strings.TrimSpace(*att.ContentType) != "" {
			c.Writer.Header().Set("Content-Type", strings.TrimSpace(*att.ContentType))
		} else {
			c.Writer.Header().Set("Content-Type", "application/octet-stream")
		}
		http.ServeContent(c.Writer, c.Request, name, att.CreatedAt, f)
	}
}
