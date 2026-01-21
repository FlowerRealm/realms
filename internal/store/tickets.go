// Package store 提供工单系统（tickets/messages/attachments）的数据库读写封装。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	TicketStatusOpen   = 1
	TicketStatusClosed = 2
)

const (
	TicketActorTypeUser   = "user"
	TicketActorTypeAdmin  = "admin"
	TicketActorTypeSystem = "system"
)

type TicketWithOwner struct {
	Ticket
	OwnerEmail    string
	OwnerUsername *string
}

type TicketMessageWithActor struct {
	TicketMessage
	ActorEmail    *string
	ActorUsername *string
}

type TicketAttachmentInput struct {
	UploaderUserID *int64
	OriginalName   string
	ContentType    *string
	SizeBytes      int64
	SHA256         []byte
	StorageRelPath string
	ExpiresAt      time.Time
}

func (s *Store) CreateTicketWithMessageAndAttachments(ctx context.Context, userID int64, subject string, body string, attachments []TicketAttachmentInput) (int64, int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
INSERT INTO tickets(user_id, subject, status, last_message_at, closed_at, created_at, updated_at)
VALUES(?, ?, ?, NOW(), NULL, NOW(), NOW())
`, userID, subject, TicketStatusOpen)
	if err != nil {
		return 0, 0, fmt.Errorf("创建工单失败: %w", err)
	}
	ticketID, err := res.LastInsertId()
	if err != nil {
		return 0, 0, fmt.Errorf("获取工单 id 失败: %w", err)
	}

	res, err = tx.ExecContext(ctx, `
INSERT INTO ticket_messages(ticket_id, actor_type, actor_user_id, body, created_at)
VALUES(?, ?, ?, ?, NOW())
`, ticketID, TicketActorTypeUser, userID, body)
	if err != nil {
		return 0, 0, fmt.Errorf("创建工单消息失败: %w", err)
	}
	messageID, err := res.LastInsertId()
	if err != nil {
		return 0, 0, fmt.Errorf("获取工单消息 id 失败: %w", err)
	}

	for _, a := range attachments {
		if strings.TrimSpace(a.OriginalName) == "" || strings.TrimSpace(a.StorageRelPath) == "" {
			return 0, 0, errors.New("附件参数错误")
		}
		if a.SizeBytes <= 0 {
			return 0, 0, errors.New("附件大小非法")
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO ticket_attachments(ticket_id, message_id, uploader_user_id, original_name, content_type, size_bytes, sha256, storage_rel_path, expires_at, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
`, ticketID, messageID, a.UploaderUserID, a.OriginalName, a.ContentType, a.SizeBytes, a.SHA256, a.StorageRelPath, a.ExpiresAt); err != nil {
			return 0, 0, fmt.Errorf("创建工单附件失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("提交事务失败: %w", err)
	}
	return ticketID, messageID, nil
}

func (s *Store) AddTicketMessageWithAttachments(ctx context.Context, ticketID int64, actorType string, actorUserID *int64, body string, attachments []TicketAttachmentInput) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
INSERT INTO ticket_messages(ticket_id, actor_type, actor_user_id, body, created_at)
VALUES(?, ?, ?, ?, NOW())
`, ticketID, actorType, actorUserID, body)
	if err != nil {
		return 0, fmt.Errorf("创建工单消息失败: %w", err)
	}
	messageID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取工单消息 id 失败: %w", err)
	}

	for _, a := range attachments {
		if strings.TrimSpace(a.OriginalName) == "" || strings.TrimSpace(a.StorageRelPath) == "" {
			return 0, errors.New("附件参数错误")
		}
		if a.SizeBytes <= 0 {
			return 0, errors.New("附件大小非法")
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO ticket_attachments(ticket_id, message_id, uploader_user_id, original_name, content_type, size_bytes, sha256, storage_rel_path, expires_at, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
`, ticketID, messageID, a.UploaderUserID, a.OriginalName, a.ContentType, a.SizeBytes, a.SHA256, a.StorageRelPath, a.ExpiresAt); err != nil {
			return 0, fmt.Errorf("创建工单附件失败: %w", err)
		}
	}

	res, err = tx.ExecContext(ctx, `
UPDATE tickets
SET last_message_at=NOW(), updated_at=NOW()
WHERE id=?
`, ticketID)
	if err != nil {
		return 0, fmt.Errorf("更新工单时间失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, sql.ErrNoRows
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交事务失败: %w", err)
	}
	return messageID, nil
}

func (s *Store) CloseTicket(ctx context.Context, ticketID int64) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE tickets
SET status=?, closed_at=NOW(), updated_at=NOW()
WHERE id=?
`, TicketStatusClosed, ticketID)
	if err != nil {
		return fmt.Errorf("关闭工单失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ReopenTicket(ctx context.Context, ticketID int64) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE tickets
SET status=?, closed_at=NULL, updated_at=NOW()
WHERE id=?
`, TicketStatusOpen, ticketID)
	if err != nil {
		return fmt.Errorf("恢复工单失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListTicketsByUser(ctx context.Context, userID int64, status *int) ([]Ticket, error) {
	q := `
SELECT id, user_id, subject, status, last_message_at, closed_at, created_at, updated_at
FROM tickets
WHERE user_id=?
`
	args := []any{userID}
	if status != nil {
		q += " AND status=?"
		args = append(args, *status)
	}
	q += "\nORDER BY last_message_at DESC, id DESC"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("查询工单列表失败: %w", err)
	}
	defer rows.Close()

	var out []Ticket
	for rows.Next() {
		var t Ticket
		var closedAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.UserID, &t.Subject, &t.Status, &t.LastMessageAt, &closedAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描工单失败: %w", err)
		}
		if closedAt.Valid {
			t.ClosedAt = &closedAt.Time
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历工单失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListTicketsForAdmin(ctx context.Context, status *int) ([]TicketWithOwner, error) {
	q := `
SELECT t.id, t.user_id, u.email, u.username, t.subject, t.status, t.last_message_at, t.closed_at, t.created_at, t.updated_at
FROM tickets t
JOIN users u ON u.id=t.user_id
WHERE 1=1
`
	var args []any
	if status != nil {
		q += " AND t.status=?"
		args = append(args, *status)
	}
	q += "\nORDER BY t.last_message_at DESC, t.id DESC"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("查询工单列表失败: %w", err)
	}
	defer rows.Close()

	var out []TicketWithOwner
	for rows.Next() {
		var v TicketWithOwner
		var username sql.NullString
		var closedAt sql.NullTime
		if err := rows.Scan(&v.ID, &v.UserID, &v.OwnerEmail, &username, &v.Subject, &v.Status, &v.LastMessageAt, &closedAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描工单失败: %w", err)
		}
		if username.Valid {
			v.OwnerUsername = &username.String
		}
		if closedAt.Valid {
			v.ClosedAt = &closedAt.Time
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历工单失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetTicketByIDForUser(ctx context.Context, ticketID int64, userID int64) (Ticket, error) {
	var t Ticket
	var closedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, subject, status, last_message_at, closed_at, created_at, updated_at
FROM tickets
WHERE id=? AND user_id=?
`, ticketID, userID).Scan(&t.ID, &t.UserID, &t.Subject, &t.Status, &t.LastMessageAt, &closedAt, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Ticket{}, sql.ErrNoRows
		}
		return Ticket{}, fmt.Errorf("查询工单失败: %w", err)
	}
	if closedAt.Valid {
		t.ClosedAt = &closedAt.Time
	}
	return t, nil
}

func (s *Store) GetTicketWithOwnerByID(ctx context.Context, ticketID int64) (TicketWithOwner, error) {
	var v TicketWithOwner
	var username sql.NullString
	var closedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
SELECT t.id, t.user_id, u.email, u.username, t.subject, t.status, t.last_message_at, t.closed_at, t.created_at, t.updated_at
FROM tickets t
JOIN users u ON u.id=t.user_id
WHERE t.id=?
`, ticketID).Scan(&v.ID, &v.UserID, &v.OwnerEmail, &username, &v.Subject, &v.Status, &v.LastMessageAt, &closedAt, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TicketWithOwner{}, sql.ErrNoRows
		}
		return TicketWithOwner{}, fmt.Errorf("查询工单失败: %w", err)
	}
	if username.Valid {
		v.OwnerUsername = &username.String
	}
	if closedAt.Valid {
		v.ClosedAt = &closedAt.Time
	}
	return v, nil
}

func (s *Store) ListTicketMessagesWithActors(ctx context.Context, ticketID int64) ([]TicketMessageWithActor, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT m.id, m.ticket_id, m.actor_type, m.actor_user_id, u.email, u.username, m.body, m.created_at
FROM ticket_messages m
LEFT JOIN users u ON u.id=m.actor_user_id
WHERE m.ticket_id=?
ORDER BY m.id ASC
`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("查询工单消息失败: %w", err)
	}
	defer rows.Close()

	var out []TicketMessageWithActor
	for rows.Next() {
		var v TicketMessageWithActor
		var actorUserID sql.NullInt64
		var email sql.NullString
		var username sql.NullString
		if err := rows.Scan(&v.ID, &v.TicketID, &v.ActorType, &actorUserID, &email, &username, &v.Body, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描工单消息失败: %w", err)
		}
		if actorUserID.Valid {
			v.ActorUserID = &actorUserID.Int64
		}
		if email.Valid {
			v.ActorEmail = &email.String
		}
		if username.Valid {
			v.ActorUsername = &username.String
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历工单消息失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListTicketAttachmentsByTicketID(ctx context.Context, ticketID int64) ([]TicketAttachment, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, ticket_id, message_id, uploader_user_id, original_name, content_type, size_bytes, sha256, storage_rel_path, expires_at, created_at
FROM ticket_attachments
WHERE ticket_id=?
ORDER BY id ASC
`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("查询工单附件失败: %w", err)
	}
	defer rows.Close()

	var out []TicketAttachment
	for rows.Next() {
		var a TicketAttachment
		var uploaderUserID sql.NullInt64
		var contentType sql.NullString
		if err := rows.Scan(&a.ID, &a.TicketID, &a.MessageID, &uploaderUserID, &a.OriginalName, &contentType, &a.SizeBytes, &a.SHA256, &a.StorageRelPath, &a.ExpiresAt, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描工单附件失败: %w", err)
		}
		if uploaderUserID.Valid {
			a.UploaderUserID = &uploaderUserID.Int64
		}
		if contentType.Valid {
			a.ContentType = &contentType.String
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历工单附件失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetTicketAttachmentByID(ctx context.Context, attachmentID int64) (TicketAttachment, error) {
	var a TicketAttachment
	var uploaderUserID sql.NullInt64
	var contentType sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, ticket_id, message_id, uploader_user_id, original_name, content_type, size_bytes, sha256, storage_rel_path, expires_at, created_at
FROM ticket_attachments
WHERE id=?
`, attachmentID).Scan(&a.ID, &a.TicketID, &a.MessageID, &uploaderUserID, &a.OriginalName, &contentType, &a.SizeBytes, &a.SHA256, &a.StorageRelPath, &a.ExpiresAt, &a.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TicketAttachment{}, sql.ErrNoRows
		}
		return TicketAttachment{}, fmt.Errorf("查询工单附件失败: %w", err)
	}
	if uploaderUserID.Valid {
		a.UploaderUserID = &uploaderUserID.Int64
	}
	if contentType.Valid {
		a.ContentType = &contentType.String
	}
	return a, nil
}

func (s *Store) ListExpiredTicketAttachments(ctx context.Context, limit int) ([]TicketAttachment, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, ticket_id, message_id, uploader_user_id, original_name, content_type, size_bytes, sha256, storage_rel_path, expires_at, created_at
FROM ticket_attachments
WHERE expires_at < NOW()
ORDER BY expires_at ASC
LIMIT ?
`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询过期附件失败: %w", err)
	}
	defer rows.Close()

	var out []TicketAttachment
	for rows.Next() {
		var a TicketAttachment
		var uploaderUserID sql.NullInt64
		var contentType sql.NullString
		if err := rows.Scan(&a.ID, &a.TicketID, &a.MessageID, &uploaderUserID, &a.OriginalName, &contentType, &a.SizeBytes, &a.SHA256, &a.StorageRelPath, &a.ExpiresAt, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描过期附件失败: %w", err)
		}
		if uploaderUserID.Valid {
			a.UploaderUserID = &uploaderUserID.Int64
		}
		if contentType.Valid {
			a.ContentType = &contentType.String
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历过期附件失败: %w", err)
	}
	return out, nil
}

func (s *Store) DeleteTicketAttachmentsByIDs(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var b strings.Builder
	b.WriteString("DELETE FROM ticket_attachments WHERE id IN (")
	for i := range ids {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("?")
	}
	b.WriteString(")")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := s.db.ExecContext(ctx, b.String(), args...)
	if err != nil {
		return 0, fmt.Errorf("删除工单附件失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取删除结果失败: %w", err)
	}
	return n, nil
}
