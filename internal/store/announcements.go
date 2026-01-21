// Package store 提供公告系统的持久化：管理员发布公告，用户侧记录已读状态。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	AnnouncementStatusDraft     = 0
	AnnouncementStatusPublished = 1
)

// Announcement 表示一条公告（全量）。
type Announcement struct {
	ID        int64
	Title     string
	Body      string
	Status    int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AnnouncementWithRead 表示公告 + 指定用户的已读信息（ReadAt 为空表示未读）。
type AnnouncementWithRead struct {
	Announcement
	ReadAt *time.Time
}

func (s *Store) CreateAnnouncement(ctx context.Context, title string, body string, status int) (int64, error) {
	switch status {
	case AnnouncementStatusDraft, AnnouncementStatusPublished:
	default:
		return 0, errors.New("status 不合法")
	}

	res, err := s.db.ExecContext(ctx, `
INSERT INTO announcements(title, body, status, created_at, updated_at)
VALUES(?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, title, body, status)
	if err != nil {
		return 0, fmt.Errorf("创建公告失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取公告 id 失败: %w", err)
	}
	return id, nil
}

func (s *Store) ListAnnouncements(ctx context.Context) ([]Announcement, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, title, body, status, created_at, updated_at
FROM announcements
ORDER BY created_at DESC, id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("查询公告失败: %w", err)
	}
	defer rows.Close()

	var out []Announcement
	for rows.Next() {
		var a Announcement
		if err := rows.Scan(&a.ID, &a.Title, &a.Body, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描公告失败: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历公告失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetPublishedAnnouncementByID(ctx context.Context, announcementID int64) (Announcement, error) {
	var a Announcement
	err := s.db.QueryRowContext(ctx, `
SELECT id, title, body, status, created_at, updated_at
FROM announcements
WHERE id=? AND status=?
`, announcementID, AnnouncementStatusPublished).Scan(&a.ID, &a.Title, &a.Body, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Announcement{}, sql.ErrNoRows
		}
		return Announcement{}, fmt.Errorf("查询公告失败: %w", err)
	}
	return a, nil
}

func (s *Store) UpdateAnnouncementStatus(ctx context.Context, announcementID int64, status int) error {
	switch status {
	case AnnouncementStatusDraft, AnnouncementStatusPublished:
	default:
		return errors.New("status 不合法")
	}

	res, err := s.db.ExecContext(ctx, `
UPDATE announcements
SET status=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, status, announcementID)
	if err != nil {
		return fmt.Errorf("更新公告状态失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteAnnouncement(ctx context.Context, announcementID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
DELETE FROM announcement_reads
WHERE announcement_id=?
`, announcementID); err != nil {
		return fmt.Errorf("删除公告已读记录失败: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
DELETE FROM announcements
WHERE id=?
`, announcementID)
	if err != nil {
		return fmt.Errorf("删除公告失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) ListPublishedAnnouncementsWithRead(ctx context.Context, userID int64, limit int) ([]AnnouncementWithRead, error) {
	if limit <= 0 {
		limit = 200
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT a.id, a.title, a.body, a.status, a.created_at, a.updated_at, r.read_at
FROM announcements a
LEFT JOIN announcement_reads r
  ON r.announcement_id=a.id AND r.user_id=?
WHERE a.status=?
ORDER BY a.created_at DESC, a.id DESC
LIMIT ?
`, userID, AnnouncementStatusPublished, limit)
	if err != nil {
		return nil, fmt.Errorf("查询公告失败: %w", err)
	}
	defer rows.Close()

	var out []AnnouncementWithRead
	for rows.Next() {
		var row AnnouncementWithRead
		var readAt sql.NullTime
		if err := rows.Scan(&row.ID, &row.Title, &row.Body, &row.Status, &row.CreatedAt, &row.UpdatedAt, &readAt); err != nil {
			return nil, fmt.Errorf("扫描公告失败: %w", err)
		}
		if readAt.Valid {
			row.ReadAt = &readAt.Time
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历公告失败: %w", err)
	}
	return out, nil
}

func (s *Store) CountUnreadAnnouncements(ctx context.Context, userID int64) (int64, error) {
	var n int64
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1)
FROM announcements a
WHERE a.status=?
  AND NOT EXISTS (
    SELECT 1 FROM announcement_reads r
    WHERE r.user_id=? AND r.announcement_id=a.id
  )
`, AnnouncementStatusPublished, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("统计未读公告失败: %w", err)
	}
	return n, nil
}

func (s *Store) GetLatestUnreadAnnouncement(ctx context.Context, userID int64) (Announcement, error) {
	var a Announcement
	err := s.db.QueryRowContext(ctx, `
SELECT id, title, body, status, created_at, updated_at
FROM announcements a
WHERE a.status=?
  AND NOT EXISTS (
    SELECT 1 FROM announcement_reads r
    WHERE r.user_id=? AND r.announcement_id=a.id
  )
ORDER BY a.created_at DESC, a.id DESC
LIMIT 1
`, AnnouncementStatusPublished, userID).Scan(&a.ID, &a.Title, &a.Body, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Announcement{}, sql.ErrNoRows
		}
		return Announcement{}, fmt.Errorf("查询未读公告失败: %w", err)
	}
	return a, nil
}

func (s *Store) MarkAnnouncementRead(ctx context.Context, userID int64, announcementID int64) error {
	stmt := `
INSERT IGNORE INTO announcement_reads(user_id, announcement_id, read_at)
VALUES(?, ?, CURRENT_TIMESTAMP)
`
	if s.dialect == DialectSQLite {
		stmt = `
INSERT OR IGNORE INTO announcement_reads(user_id, announcement_id, read_at)
VALUES(?, ?, CURRENT_TIMESTAMP)
`
	}
	if _, err := s.db.ExecContext(ctx, stmt, userID, announcementID); err != nil {
		return fmt.Errorf("标记已读失败: %w", err)
	}
	return nil
}
