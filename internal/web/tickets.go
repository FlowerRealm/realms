package web

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

	"realms/internal/auth"
	emailpkg "realms/internal/email"
	"realms/internal/middleware"
	"realms/internal/store"
	ticketspkg "realms/internal/tickets"
)

const (
	ticketSubjectMaxLen   = 200
	ticketBodyMaxLen      = 8000
	ticketMaxAttachments  = 10
	ticketMultipartMemory = 32 << 20
)

type TicketListItemView struct {
	ID            int64
	Subject       string
	StatusText    string
	StatusBadge   string
	LastMessageAt string
	CreatedAt     string
}

type TicketDetailView struct {
	ID            int64
	Subject       string
	StatusText    string
	StatusBadge   string
	LastMessageAt string
	CreatedAt     string
	ClosedAt      string
	CanReply      bool
}

type TicketMessageView struct {
	ID          int64
	Actor       string
	ActorMeta   string
	Body        string
	CreatedAt   string
	Attachments []TicketAttachmentView
}

type TicketAttachmentView struct {
	ID        int64
	Name      string
	Size      string
	ExpiresAt string
	URL       string
}

func (s *Server) TicketsPage(w http.ResponseWriter, r *http.Request) {
	s.ticketsPageWithStatus(w, r, nil)
}

func (s *Server) TicketsOpenPage(w http.ResponseWriter, r *http.Request) {
	status := store.TicketStatusOpen
	s.ticketsPageWithStatus(w, r, &status)
}

func (s *Server) TicketsClosedPage(w http.ResponseWriter, r *http.Request) {
	status := store.TicketStatusClosed
	s.ticketsPageWithStatus(w, r, &status)
}

func (s *Server) ticketsPageWithStatus(w http.ResponseWriter, r *http.Request, statusPtr *int) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	rows, err := s.store.ListTicketsByUser(r.Context(), p.UserID, statusPtr)
	if err != nil {
		http.Error(w, "查询工单失败", http.StatusInternalServerError)
		return
	}

	items := make([]TicketListItemView, 0, len(rows))
	for _, t := range rows {
		st, badge := ticketStatusView(t.Status)
		items = append(items, TicketListItemView{
			ID:            t.ID,
			Subject:       t.Subject,
			StatusText:    st,
			StatusBadge:   badge,
			LastMessageAt: t.LastMessageAt.Format("2006-01-02 15:04"),
			CreatedAt:     t.CreatedAt.Format("2006-01-02 15:04"),
		})
	}

	s.Render(w, "page_tickets", s.withFeatures(r.Context(), TemplateData{
		Title:     "工单 - Realms",
		User:      userViewFromUser(u),
		CSRFToken: csrfToken(p),
		Error:     strings.TrimSpace(middleware.FlashError(r.Context())),
		Notice:    strings.TrimSpace(middleware.FlashNotice(r.Context())),
		Tickets:   items,
	}))
}

func (s *Server) TicketNewPage(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}
	s.Render(w, "page_ticket_new", s.withFeatures(r.Context(), TemplateData{
		Title:     "创建工单 - Realms",
		User:      userViewFromUser(u),
		CSRFToken: csrfToken(p),
	}))
}

func (s *Server) CreateTicket(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	if err := r.ParseMultipartForm(ticketMultipartMemory); err != nil {
		s.Render(w, "page_ticket_new", s.withFeatures(r.Context(), TemplateData{
			Title:     "创建工单 - Realms",
			User:      userViewFromUser(u),
			CSRFToken: csrfToken(p),
			Error:     parseUploadError(err),
		}))
		return
	}

	subject := strings.TrimSpace(r.FormValue("subject"))
	body := strings.TrimSpace(r.FormValue("body"))
	if subject == "" || body == "" {
		s.Render(w, "page_ticket_new", s.withFeatures(r.Context(), TemplateData{Title: "创建工单 - Realms", User: userViewFromUser(u), CSRFToken: csrfToken(p), Error: "标题与内容不能为空"}))
		return
	}
	if len([]rune(subject)) > ticketSubjectMaxLen {
		s.Render(w, "page_ticket_new", s.withFeatures(r.Context(), TemplateData{Title: "创建工单 - Realms", User: userViewFromUser(u), CSRFToken: csrfToken(p), Error: "标题过长"}))
		return
	}
	if len([]rune(body)) > ticketBodyMaxLen {
		s.Render(w, "page_ticket_new", s.withFeatures(r.Context(), TemplateData{Title: "创建工单 - Realms", User: userViewFromUser(u), CSRFToken: csrfToken(p), Error: "内容过长"}))
		return
	}

	now := time.Now()
	files := r.MultipartForm.File["attachments"]
	attInputs, saved, errMsg := s.saveTicketAttachments(now, &p.UserID, files)
	if errMsg != "" {
		s.Render(w, "page_ticket_new", s.withFeatures(r.Context(), TemplateData{Title: "创建工单 - Realms", User: userViewFromUser(u), CSRFToken: csrfToken(p), Error: errMsg}))
		return
	}
	defer func() {
		if errMsg != "" {
			s.deleteSavedAttachments(saved)
		}
	}()

	ticketID, _, err := s.store.CreateTicketWithMessageAndAttachments(r.Context(), p.UserID, subject, body, attInputs)
	if err != nil {
		errMsg = "创建工单失败"
		slog.Error("创建工单失败", "err", err)
		s.Render(w, "page_ticket_new", s.withFeatures(r.Context(), TemplateData{Title: "创建工单 - Realms", User: userViewFromUser(u), CSRFToken: csrfToken(p), Error: errMsg}))
		return
	}

	s.sendTicketMailBestEffort(r, func(ctxCtx time.Time, mailer emailpkg.Mailer) error {
		_ = ctxCtx
		return ticketspkg.NotifyRootsNewTicket(r.Context(), s.store, mailer, s.baseURLFromRequest(r), ticketID, u.Email, subject, body)
	})

	http.Redirect(w, r, fmt.Sprintf("/tickets/%d", ticketID), http.StatusFound)
}

func (s *Server) TicketDetailPage(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	ticketID, err := parseInt64(strings.TrimSpace(r.PathValue("ticket_id")))
	if err != nil || ticketID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	tk, err := s.store.GetTicketByIDForUser(r.Context(), ticketID, p.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "查询工单失败", http.StatusInternalServerError)
		return
	}

	msgs, err := s.store.ListTicketMessagesWithActors(r.Context(), ticketID)
	if err != nil {
		http.Error(w, "查询工单消息失败", http.StatusInternalServerError)
		return
	}
	atts, err := s.store.ListTicketAttachmentsByTicketID(r.Context(), ticketID)
	if err != nil {
		http.Error(w, "查询工单附件失败", http.StatusInternalServerError)
		return
	}
	attsByMsg := make(map[int64][]store.TicketAttachment)
	for _, a := range atts {
		attsByMsg[a.MessageID] = append(attsByMsg[a.MessageID], a)
	}

	dv := ticketDetailView(tk)
	mv := make([]TicketMessageView, 0, len(msgs))
	for _, m := range msgs {
		actor, meta := ticketActorView(m.ActorType, m.ActorEmail, m.ActorUsername)
		av := make([]TicketAttachmentView, 0, len(attsByMsg[m.ID]))
		for _, a := range attsByMsg[m.ID] {
			av = append(av, TicketAttachmentView{
				ID:        a.ID,
				Name:      a.OriginalName,
				Size:      ticketspkg.FormatBytes(a.SizeBytes),
				ExpiresAt: a.ExpiresAt.Format("2006-01-02 15:04"),
				URL:       fmt.Sprintf("/tickets/%d/attachments/%d", ticketID, a.ID),
			})
		}
		mv = append(mv, TicketMessageView{
			ID:          m.ID,
			Actor:       actor,
			ActorMeta:   meta,
			Body:        m.Body,
			CreatedAt:   m.CreatedAt.Format("2006-01-02 15:04"),
			Attachments: av,
		})
	}

	s.Render(w, "page_ticket_detail", s.withFeatures(r.Context(), TemplateData{
		Title:          fmt.Sprintf("工单 #%d - Realms", ticketID),
		User:           userViewFromUser(u),
		CSRFToken:      csrfToken(p),
		Error:          strings.TrimSpace(middleware.FlashError(r.Context())),
		Notice:         strings.TrimSpace(middleware.FlashNotice(r.Context())),
		Ticket:         &dv,
		TicketMessages: mv,
	}))
}

func (s *Server) ReplyTicket(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	ticketID, err := parseInt64(strings.TrimSpace(r.PathValue("ticket_id")))
	if err != nil || ticketID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	tk, err := s.store.GetTicketByIDForUser(r.Context(), ticketID, p.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "查询工单失败", http.StatusInternalServerError)
		return
	}
	if tk.Status != store.TicketStatusOpen {
		middleware.SetFlashError(w, r, "工单已关闭，无法回复")
		http.Redirect(w, r, fmt.Sprintf("/tickets/%d", ticketID), http.StatusFound)
		return
	}

	if err := r.ParseMultipartForm(ticketMultipartMemory); err != nil {
		http.Error(w, parseUploadError(err), http.StatusBadRequest)
		return
	}
	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		http.Redirect(w, r, fmt.Sprintf("/tickets/%d", ticketID), http.StatusFound)
		return
	}
	if len([]rune(body)) > ticketBodyMaxLen {
		http.Error(w, "内容过长", http.StatusBadRequest)
		return
	}

	now := time.Now()
	files := r.MultipartForm.File["attachments"]
	attInputs, saved, errMsg := s.saveTicketAttachments(now, &p.UserID, files)
	if errMsg != "" {
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	defer func() {
		if errMsg != "" {
			s.deleteSavedAttachments(saved)
		}
	}()

	if _, err := s.store.AddTicketMessageWithAttachments(r.Context(), ticketID, store.TicketActorTypeUser, &p.UserID, body, attInputs); err != nil {
		errMsg = "回复失败"
		slog.Error("回复工单失败", "err", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	s.sendTicketMailBestEffort(r, func(ctxCtx time.Time, mailer emailpkg.Mailer) error {
		_ = ctxCtx
		return ticketspkg.NotifyRootsUserReply(r.Context(), s.store, mailer, s.baseURLFromRequest(r), ticketID, u.Email, tk.Subject, body)
	})

	http.Redirect(w, r, fmt.Sprintf("/tickets/%d", ticketID), http.StatusFound)
}

func (s *Server) TicketAttachmentDownload(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
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

	if _, err := s.store.GetTicketByIDForUser(r.Context(), ticketID, p.UserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "查询工单失败", http.StatusInternalServerError)
		return
	}
	att, err := s.store.GetTicketAttachmentByID(r.Context(), attID)
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

	name := sanitizeDownloadName(att.OriginalName)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	if att.ContentType != nil && strings.TrimSpace(*att.ContentType) != "" {
		w.Header().Set("Content-Type", strings.TrimSpace(*att.ContentType))
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	http.ServeContent(w, r, name, att.CreatedAt, f)
}

func (s *Server) saveTicketAttachments(now time.Time, uploaderUserID *int64, files []*multipart.FileHeader) ([]store.TicketAttachmentInput, []string, string) {
	if len(files) == 0 {
		return nil, nil, ""
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
			s.deleteSavedAttachments(saved)
			return nil, nil, "读取附件失败"
		}
		func() {
			defer func() { _ = src.Close() }()
			res, err := s.ticketStorage.Save(now, src)
			if err != nil {
				s.deleteSavedAttachments(saved)
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

func (s *Server) deleteSavedAttachments(saved []string) {
	for _, rel := range saved {
		full, err := s.ticketStorage.Resolve(rel)
		if err != nil {
			continue
		}
		_ = os.Remove(full)
	}
}

func (s *Server) sendTicketMailBestEffort(r *http.Request, fn func(now time.Time, mailer emailpkg.Mailer) error) {
	ctx := r.Context()
	smtpCfg := s.smtpConfigEffective(ctx)
	if strings.TrimSpace(smtpCfg.SMTPServer) == "" || strings.TrimSpace(smtpCfg.SMTPAccount) == "" || strings.TrimSpace(smtpCfg.SMTPToken) == "" {
		return
	}
	mailer := emailpkg.NewSMTPMailer(smtpCfg)
	if err := fn(time.Now(), mailer); err != nil {
		slog.Error("发送工单邮件失败", "err", err)
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

func ticketDetailView(t store.Ticket) TicketDetailView {
	st, badge := ticketStatusView(t.Status)
	closedAt := ""
	if t.ClosedAt != nil {
		closedAt = t.ClosedAt.Format("2006-01-02 15:04")
	}
	return TicketDetailView{
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
