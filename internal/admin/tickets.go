package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	emailpkg "realms/internal/email"
	"realms/internal/store"
	ticketspkg "realms/internal/tickets"
)

const (
	adminTicketBodyMaxLen      = 8000
	adminTicketSubjectMaxLen   = 200
	adminTicketMaxAttachments  = 10
	adminTicketMultipartMemory = 32 << 20
)

type adminTicketListItemView struct {
	ID            int64
	UserEmail     string
	Subject       string
	StatusText    string
	StatusBadge   string
	LastMessageAt string
	CreatedAt     string
}

type adminTicketDetailView struct {
	ID            int64
	UserEmail     string
	Subject       string
	StatusText    string
	StatusBadge   string
	LastMessageAt string
	CreatedAt     string
	ClosedAt      string
	CanReply      bool
	Closed        bool
}

type adminTicketMessageView struct {
	ID          int64
	Actor       string
	ActorMeta   string
	Body        string
	CreatedAt   string
	Attachments []adminTicketAttachmentView
}

type adminTicketAttachmentView struct {
	ID        int64
	Name      string
	Size      string
	ExpiresAt string
	URL       string
}

func (s *Server) Tickets(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	loc, _ := s.adminTimeLocation(r.Context())

	var statusPtr *int
	switch strings.TrimSpace(r.URL.Query().Get("status")) {
	case "open":
		v := store.TicketStatusOpen
		statusPtr = &v
	case "closed":
		v := store.TicketStatusClosed
		statusPtr = &v
	default:
	}

	rows, err := s.st.ListTicketsForAdmin(r.Context(), statusPtr)
	if err != nil {
		http.Error(w, "查询工单失败", http.StatusInternalServerError)
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

	s.render(w, "admin_tickets", s.withFeatures(r.Context(), templateData{
		Title:     "工单 - Admin",
		User:      userView{ID: u.ID, Email: u.Email, Role: u.Role},
		IsRoot:    isRoot,
		CSRFToken: csrf,
		Tickets:   items,
	}))
}

func (s *Server) Ticket(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	loc, _ := s.adminTimeLocation(r.Context())

	ticketID, err := parseInt64(strings.TrimSpace(r.PathValue("ticket_id")))
	if err != nil || ticketID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	tk, err := s.st.GetTicketWithOwnerByID(r.Context(), ticketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "查询工单失败", http.StatusInternalServerError)
		return
	}
	msgs, err := s.st.ListTicketMessagesWithActors(r.Context(), ticketID)
	if err != nil {
		http.Error(w, "查询工单消息失败", http.StatusInternalServerError)
		return
	}
	atts, err := s.st.ListTicketAttachmentsByTicketID(r.Context(), ticketID)
	if err != nil {
		http.Error(w, "查询工单附件失败", http.StatusInternalServerError)
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
				URL:       fmt.Sprintf("/admin/tickets/%d/attachments/%d", ticketID, a.ID),
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

	s.render(w, "admin_ticket", s.withFeatures(r.Context(), templateData{
		Title:          fmt.Sprintf("工单 #%d - Admin", ticketID),
		User:           userView{ID: u.ID, Email: u.Email, Role: u.Role},
		IsRoot:         isRoot,
		CSRFToken:      csrf,
		Ticket:         &dv,
		TicketMessages: mv,
	}))
}

func (s *Server) ReplyTicket(w http.ResponseWriter, r *http.Request) {
	u, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	ticketID, err := parseInt64(strings.TrimSpace(r.PathValue("ticket_id")))
	if err != nil || ticketID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	tk, err := s.st.GetTicketWithOwnerByID(r.Context(), ticketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "查询工单失败", http.StatusInternalServerError)
		return
	}
	if tk.Status != store.TicketStatusOpen {
		http.Error(w, "工单已关闭，请先恢复", http.StatusBadRequest)
		return
	}

	if err := r.ParseMultipartForm(adminTicketMultipartMemory); err != nil {
		http.Error(w, adminParseUploadError(err), http.StatusBadRequest)
		return
	}
	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		http.Redirect(w, r, fmt.Sprintf("/admin/tickets/%d", ticketID), http.StatusFound)
		return
	}
	if len([]rune(body)) > adminTicketBodyMaxLen {
		http.Error(w, "内容过长", http.StatusBadRequest)
		return
	}

	now := time.Now()
	files := r.MultipartForm.File["attachments"]
	attInputs, saved, errMsg := s.adminSaveTicketAttachments(now, &u.ID, files)
	if errMsg != "" {
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	defer func() {
		if errMsg != "" {
			s.adminDeleteSavedAttachments(saved)
		}
	}()

	if _, err := s.st.AddTicketMessageWithAttachments(r.Context(), ticketID, store.TicketActorTypeAdmin, &u.ID, body, attInputs); err != nil {
		errMsg = "回复失败"
		slog.Error("管理员回复工单失败", "err", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	s.adminSendTicketMailBestEffort(r, func(mailer emailpkg.Mailer) error {
		return ticketspkg.NotifyUserAdminReply(r.Context(), mailer, s.baseURLFromRequest(r), ticketID, tk.OwnerEmail, tk.Subject, body)
	})

	http.Redirect(w, r, fmt.Sprintf("/admin/tickets/%d", ticketID), http.StatusFound)
}

func (s *Server) CloseTicket(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	ticketID, err := parseInt64(strings.TrimSpace(r.PathValue("ticket_id")))
	if err != nil || ticketID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	tk, err := s.st.GetTicketWithOwnerByID(r.Context(), ticketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "查询工单失败", http.StatusInternalServerError)
		return
	}
	if err := s.st.CloseTicket(r.Context(), ticketID); err != nil {
		http.Error(w, "关闭失败", http.StatusInternalServerError)
		return
	}
	s.adminSendTicketMailBestEffort(r, func(mailer emailpkg.Mailer) error {
		return ticketspkg.NotifyUserTicketStatus(r.Context(), mailer, s.baseURLFromRequest(r), ticketID, tk.OwnerEmail, tk.Subject, "已关闭")
	})
	http.Redirect(w, r, fmt.Sprintf("/admin/tickets/%d", ticketID), http.StatusFound)
}

func (s *Server) ReopenTicket(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	ticketID, err := parseInt64(strings.TrimSpace(r.PathValue("ticket_id")))
	if err != nil || ticketID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	tk, err := s.st.GetTicketWithOwnerByID(r.Context(), ticketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "查询工单失败", http.StatusInternalServerError)
		return
	}
	if err := s.st.ReopenTicket(r.Context(), ticketID); err != nil {
		http.Error(w, "恢复失败", http.StatusInternalServerError)
		return
	}
	s.adminSendTicketMailBestEffort(r, func(mailer emailpkg.Mailer) error {
		return ticketspkg.NotifyUserTicketStatus(r.Context(), mailer, s.baseURLFromRequest(r), ticketID, tk.OwnerEmail, tk.Subject, "已恢复")
	})
	http.Redirect(w, r, fmt.Sprintf("/admin/tickets/%d", ticketID), http.StatusFound)
}

func (s *Server) TicketAttachmentDownload(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	ticketID, err := parseInt64(strings.TrimSpace(r.PathValue("ticket_id")))
	if err != nil || ticketID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	attID, err := parseInt64(strings.TrimSpace(r.PathValue("attachment_id")))
	if err != nil || attID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	att, err := s.st.GetTicketAttachmentByID(r.Context(), attID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "查询附件失败", http.StatusInternalServerError)
		return
	}
	if att.TicketID != ticketID {
		http.NotFound(w, r)
		return
	}
	if time.Now().After(att.ExpiresAt) {
		http.NotFound(w, r)
		return
	}

	full, err := s.ticketStorage.Resolve(att.StorageRelPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()

	name := adminSanitizeDownloadName(att.OriginalName)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	if att.ContentType != nil && strings.TrimSpace(*att.ContentType) != "" {
		w.Header().Set("Content-Type", strings.TrimSpace(*att.ContentType))
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	http.ServeContent(w, r, name, att.CreatedAt, f)
}

func (s *Server) adminSaveTicketAttachments(now time.Time, uploaderUserID *int64, files []*multipart.FileHeader) ([]store.TicketAttachmentInput, []string, string) {
	if len(files) == 0 {
		return nil, nil, ""
	}
	if len(files) > adminTicketMaxAttachments {
		return nil, nil, "附件数量过多"
	}
	var total int64
	for _, fh := range files {
		if fh == nil {
			continue
		}
		if fh.Size <= 0 {
			return nil, nil, "附件为空文件"
		}
		total += fh.Size
		if total > s.ticketsCfg.MaxUploadBytes {
			return nil, nil, "附件总大小超过限制"
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
			s.adminDeleteSavedAttachments(saved)
			return nil, nil, "读取附件失败"
		}
		func() {
			defer func() { _ = src.Close() }()
			res, err := s.ticketStorage.Save(now, src, s.ticketsCfg.MaxUploadBytes)
			if err != nil {
				s.adminDeleteSavedAttachments(saved)
				inputs = nil
				saved = nil
				return
			}
			saved = append(saved, res.RelPath)
			ct := strings.TrimSpace(fh.Header.Get("Content-Type"))
			var ctPtr *string
			if ct != "" {
				ct = adminTruncateASCII(ct, 255)
				ctPtr = &ct
			}
			expiresAt := now.Add(s.ticketsCfg.AttachmentTTL)
			inputs = append(inputs, store.TicketAttachmentInput{
				UploaderUserID: uploaderUserID,
				OriginalName:   adminSanitizeOriginalName(fh.Filename),
				ContentType:    ctPtr,
				SizeBytes:      res.SizeBytes,
				SHA256:         nil,
				StorageRelPath: res.RelPath,
				ExpiresAt:      expiresAt,
			})
		}()
		if inputs == nil {
			return nil, nil, "保存附件失败（请检查大小限制与磁盘空间）"
		}
	}
	return inputs, saved, ""
}

func (s *Server) adminDeleteSavedAttachments(saved []string) {
	for _, rel := range saved {
		full, err := s.ticketStorage.Resolve(rel)
		if err != nil {
			continue
		}
		_ = os.Remove(full)
	}
}

func (s *Server) adminSendTicketMailBestEffort(r *http.Request, fn func(mailer emailpkg.Mailer) error) {
	ctx := r.Context()
	smtpCfg := s.smtpConfigEffective(ctx)
	if strings.TrimSpace(smtpCfg.SMTPServer) == "" || strings.TrimSpace(smtpCfg.SMTPAccount) == "" || strings.TrimSpace(smtpCfg.SMTPToken) == "" {
		return
	}
	mailer := emailpkg.NewSMTPMailer(smtpCfg)
	if err := fn(mailer); err != nil {
		slog.Error("发送工单邮件失败", "err", err)
	}
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

func adminTicketActorView(actorType string, email *string, username *string) (string, string) {
	switch actorType {
	case store.TicketActorTypeAdmin:
		if username != nil && *username != "" {
			return "管理员", "@" + *username
		}
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

func adminSanitizeOriginalName(name string) string {
	s := strings.TrimSpace(name)
	s = strings.ReplaceAll(s, "\\", "/")
	s = path.Base(s)
	s = strings.TrimSpace(s)
	if s == "" {
		return "attachment"
	}
	rs := []rune(s)
	if len(rs) > adminTicketSubjectMaxLen {
		s = string(rs[:adminTicketSubjectMaxLen])
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

func adminSanitizeDownloadName(name string) string {
	s := adminSanitizeOriginalName(name)
	s = strings.ReplaceAll(s, "\"", "'")
	return s
}

func adminParseUploadError(err error) string {
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

func adminTruncateASCII(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}
