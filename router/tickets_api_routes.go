package router

import (
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"realms/internal/store"
	ticketspkg "realms/internal/tickets"
)

const (
	ticketSubjectMaxLen   = 200
	ticketBodyMaxLen      = 8000
	ticketMaxAttachments  = 10
	ticketMultipartMemory = 32 << 20
)

type ticketListItemView struct {
	ID            int64  `json:"id"`
	Subject       string `json:"subject"`
	StatusText    string `json:"status_text"`
	StatusBadge   string `json:"status_badge"`
	LastMessageAt string `json:"last_message_at"`
	CreatedAt     string `json:"created_at"`
}

type ticketDetailView struct {
	ID            int64  `json:"id"`
	Subject       string `json:"subject"`
	StatusText    string `json:"status_text"`
	StatusBadge   string `json:"status_badge"`
	LastMessageAt string `json:"last_message_at"`
	CreatedAt     string `json:"created_at"`
	ClosedAt      string `json:"closed_at"`
	CanReply      bool   `json:"can_reply"`
}

type ticketAttachmentView struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Size      string `json:"size"`
	ExpiresAt string `json:"expires_at"`
	URL       string `json:"url"`
}

type ticketMessageView struct {
	ID          int64               `json:"id"`
	Actor       string              `json:"actor"`
	ActorMeta   string              `json:"actor_meta"`
	Body        string              `json:"body"`
	CreatedAt   string              `json:"created_at"`
	Attachments []ticketAttachmentView `json:"attachments,omitempty"`
}

type ticketDetailAPIResponse struct {
	Ticket   ticketDetailView    `json:"ticket"`
	Messages []ticketMessageView `json:"messages"`
}

func setTicketAPIRoutes(r gin.IRoutes, opts Options) {
	authn := requireUserSession(opts)

	r.GET("/tickets", authn, ticketListHandler(opts))
	r.POST("/tickets", authn, ticketCreateHandler(opts))
	r.GET("/tickets/:ticket_id", authn, ticketDetailHandler(opts))
	r.POST("/tickets/:ticket_id/reply", authn, ticketReplyHandler(opts))
	r.GET("/tickets/:ticket_id/attachments/:attachment_id", authn, ticketAttachmentDownloadHandler(opts))
}

func ticketsFeatureDisabled(c *gin.Context, opts Options) bool {
	if c == nil || opts.Store == nil {
		return false
	}
	if opts.Store.FeatureDisabledEffective(c.Request.Context(), opts.SelfMode, store.SettingFeatureDisableTickets) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
		return true
	}
	return false
}

func ticketListHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if ticketsFeatureDisabled(c, opts) {
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

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

		rows, err := opts.Store.ListTicketsByUser(c.Request.Context(), userID, statusPtr)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询工单失败"})
			return
		}
		items := make([]ticketListItemView, 0, len(rows))
		for _, t := range rows {
			st, badge := ticketStatusView(t.Status)
			items = append(items, ticketListItemView{
				ID:            t.ID,
				Subject:       t.Subject,
				StatusText:    st,
				StatusBadge:   badge,
				LastMessageAt: t.LastMessageAt.Format("2006-01-02 15:04"),
				CreatedAt:     t.CreatedAt.Format("2006-01-02 15:04"),
			})
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": items})
	}
}

func ticketCreateHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if ticketsFeatureDisabled(c, opts) {
			return
		}
		if opts.TicketStorage == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "ticket storage 未初始化"})
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		if err := c.Request.ParseMultipartForm(ticketMultipartMemory); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": parseUploadError(err)})
			return
		}

		subject := strings.TrimSpace(c.Request.FormValue("subject"))
		body := strings.TrimSpace(c.Request.FormValue("body"))
		if subject == "" || body == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "标题与内容不能为空"})
			return
		}
		if len([]rune(subject)) > ticketSubjectMaxLen {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "标题过长"})
			return
		}
		if len([]rune(body)) > ticketBodyMaxLen {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "内容过长"})
			return
		}

		now := time.Now()
		files := c.Request.MultipartForm.File["attachments"]
		attInputs, saved, errMsg := saveTicketAttachments(opts.TicketStorage, now, &userID, files)
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

		ticketID, _, err := opts.Store.CreateTicketWithMessageAndAttachments(c.Request.Context(), userID, subject, body, attInputs)
		if err != nil {
			cleanupOK = false
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建工单失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"ticket_id": ticketID}})
	}
}

func ticketDetailHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if ticketsFeatureDisabled(c, opts) {
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		ticketID, err := strconv.ParseInt(strings.TrimSpace(c.Param("ticket_id")), 10, 64)
		if err != nil || ticketID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}

		tk, err := opts.Store.GetTicketByIDForUser(c.Request.Context(), ticketID, userID)
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

		dv := ticketDetailFromStore(tk)
		mv := make([]ticketMessageView, 0, len(msgs))
		for _, m := range msgs {
			actor, meta := ticketActorView(m.ActorType, m.ActorEmail, m.ActorUsername)
			av := make([]ticketAttachmentView, 0, len(attsByMsg[m.ID]))
			for _, a := range attsByMsg[m.ID] {
				av = append(av, ticketAttachmentView{
					ID:        a.ID,
					Name:      a.OriginalName,
					Size:      ticketspkg.FormatBytes(a.SizeBytes),
					ExpiresAt: a.ExpiresAt.Format("2006-01-02 15:04"),
					URL:       fmt.Sprintf("/api/tickets/%d/attachments/%d", ticketID, a.ID),
				})
			}
			mv = append(mv, ticketMessageView{
				ID:          m.ID,
				Actor:       actor,
				ActorMeta:   meta,
				Body:        m.Body,
				CreatedAt:   m.CreatedAt.Format("2006-01-02 15:04"),
				Attachments: av,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": ticketDetailAPIResponse{
				Ticket:   dv,
				Messages: mv,
			},
		})
	}
}

func ticketReplyHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if ticketsFeatureDisabled(c, opts) {
			return
		}
		if opts.TicketStorage == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "ticket storage 未初始化"})
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		ticketID, err := strconv.ParseInt(strings.TrimSpace(c.Param("ticket_id")), 10, 64)
		if err != nil || ticketID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}

		tk, err := opts.Store.GetTicketByIDForUser(c.Request.Context(), ticketID, userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询工单失败"})
			return
		}
		if tk.Status != store.TicketStatusOpen {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "工单已关闭，如需进一步帮助请创建新工单。"})
			return
		}

		if err := c.Request.ParseMultipartForm(ticketMultipartMemory); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": parseUploadError(err)})
			return
		}
		body := strings.TrimSpace(c.Request.FormValue("body"))
		if body == "" {
			c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
			return
		}
		if len([]rune(body)) > ticketBodyMaxLen {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "内容过长"})
			return
		}

		now := time.Now()
		files := c.Request.MultipartForm.File["attachments"]
		attInputs, saved, errMsg := saveTicketAttachments(opts.TicketStorage, now, &userID, files)
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

		if _, err := opts.Store.AddTicketMessageWithAttachments(c.Request.Context(), ticketID, store.TicketActorTypeUser, &userID, body, attInputs); err != nil {
			cleanupOK = false
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "回复失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}

func ticketAttachmentDownloadHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if ticketsFeatureDisabled(c, opts) {
			return
		}
		if opts.TicketStorage == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "ticket storage 未初始化"})
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		ticketID, err := strconv.ParseInt(strings.TrimSpace(c.Param("ticket_id")), 10, 64)
		if err != nil || ticketID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}
		attID, err := strconv.ParseInt(strings.TrimSpace(c.Param("attachment_id")), 10, 64)
		if err != nil || attID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}

		if _, err := opts.Store.GetTicketByIDForUser(c.Request.Context(), ticketID, userID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询工单失败"})
			return
		}
		att, err := opts.Store.GetTicketAttachmentByID(c.Request.Context(), attID)
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

func ticketStatusView(status int) (string, string) {
	switch status {
	case store.TicketStatusOpen:
		return "打开", "bg-success"
	case store.TicketStatusClosed:
		return "已关闭", "bg-secondary"
	default:
		return "未知", "bg-light text-secondary border"
	}
}

func ticketDetailFromStore(t store.Ticket) ticketDetailView {
	st, badge := ticketStatusView(t.Status)
	closedAt := ""
	if t.ClosedAt != nil {
		closedAt = t.ClosedAt.Format("2006-01-02 15:04")
	}
	return ticketDetailView{
		ID:            t.ID,
		Subject:       t.Subject,
		StatusText:    st,
		StatusBadge:   badge,
		LastMessageAt: t.LastMessageAt.Format("2006-01-02 15:04"),
		CreatedAt:     t.CreatedAt.Format("2006-01-02 15:04"),
		ClosedAt:      closedAt,
		CanReply:      t.Status == store.TicketStatusOpen,
	}
}

func ticketActorView(actorType string, email *string, username *string) (string, string) {
	switch actorType {
	case store.TicketActorTypeAdmin:
		if email != nil && *email != "" {
			return "管理员", *email
		}
		return "管理员", ""
	case store.TicketActorTypeUser:
		if username != nil && *username != "" {
			return "用户", "@" + *username
		}
		if email != nil && *email != "" {
			return "用户", *email
		}
		return "用户", ""
	default:
		return "系统", ""
	}
}

func saveTicketAttachments(storage *ticketspkg.Storage, now time.Time, uploaderUserID *int64, files []*multipart.FileHeader) ([]store.TicketAttachmentInput, []string, string) {
	if len(files) == 0 {
		return nil, nil, ""
	}
	if len(files) > ticketMaxAttachments {
		return nil, nil, "附件数量过多"
	}
	for _, fh := range files {
		if fh == nil {
			continue
		}
		if fh.Size <= 0 {
			return nil, nil, "附件为空文件"
		}
	}

	inputs := make([]store.TicketAttachmentInput, 0, len(files))
	saved := make([]string, 0, len(files))
	for _, fh := range files {
		if fh == nil {
			continue
		}
		src, err := fh.Open()
		if err != nil {
			deleteSavedAttachments(storage, saved)
			return nil, nil, "读取附件失败"
		}
		func() {
			defer func() { _ = src.Close() }()
			res, err := storage.Save(now, src)
			if err != nil {
				deleteSavedAttachments(storage, saved)
				inputs = nil
				saved = nil
				return
			}
			saved = append(saved, res.RelPath)
			ct := strings.TrimSpace(fh.Header.Get("Content-Type"))
			var ctPtr *string
			if ct != "" {
				ct = truncateASCII(ct, 255)
				ctPtr = &ct
			}
			expiresAt := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
			inputs = append(inputs, store.TicketAttachmentInput{
				UploaderUserID: uploaderUserID,
				OriginalName:   sanitizeOriginalName(fh.Filename),
				ContentType:    ctPtr,
				SizeBytes:      res.SizeBytes,
				SHA256:         nil,
				StorageRelPath: res.RelPath,
				ExpiresAt:      expiresAt,
			})
		}()
		if inputs == nil {
			return nil, nil, "保存附件失败（请检查磁盘空间）"
		}
	}
	return inputs, saved, ""
}

func deleteSavedAttachments(storage *ticketspkg.Storage, saved []string) {
	for _, rel := range saved {
		full, err := storage.Resolve(rel)
		if err != nil {
			continue
		}
		_ = os.Remove(full)
	}
}

func sanitizeOriginalName(name string) string {
	s := strings.TrimSpace(name)
	s = strings.ReplaceAll(s, "\\", "/")
	s = path.Base(s)
	s = strings.TrimSpace(s)
	if s == "" {
		return "attachment"
	}
	rs := []rune(s)
	if len(rs) > 200 {
		s = string(rs[:200])
	}
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
	if strings.TrimSpace(s) == "" {
		return "attachment"
	}
	return s
}

func sanitizeDownloadName(name string) string {
	s := sanitizeOriginalName(name)
	s = strings.ReplaceAll(s, "\"", "'")
	return s
}

func parseUploadError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, http.ErrNotMultipart) {
		return "表单格式错误"
	}
	if strings.Contains(err.Error(), "request body too large") {
		return "上传超过大小限制"
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return "上传超过大小限制"
	}
	return "表单解析失败"
}

func truncateASCII(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

