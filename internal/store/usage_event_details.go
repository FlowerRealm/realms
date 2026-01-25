package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const maxUsageEventDetailBytes = 256 << 10

type UsageEventDetail struct {
	UsageEventID         int64
	UpstreamRequestBody  *string
	UpstreamResponseBody *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type UpsertUsageEventDetailInput struct {
	UsageEventID         int64
	UpstreamRequestBody  *string
	UpstreamResponseBody *string
}

func (s *Store) UpsertUsageEventDetail(ctx context.Context, in UpsertUsageEventDetailInput) error {
	if in.UsageEventID <= 0 {
		return errors.New("usage_event_id 不能为空")
	}

	reqBody := truncateDetail(in.UpstreamRequestBody, maxUsageEventDetailBytes)
	respBody := truncateDetail(in.UpstreamResponseBody, maxUsageEventDetailBytes)
	if reqBody == nil && respBody == nil {
		return nil
	}

	var reqAny any
	if reqBody != nil {
		reqAny = *reqBody
	}
	var respAny any
	if respBody != nil {
		respAny = *respBody
	}

	res, err := s.db.ExecContext(ctx, `
UPDATE usage_event_details
SET upstream_request_body=?, upstream_response_body=?, updated_at=CURRENT_TIMESTAMP
WHERE usage_event_id=?
`, reqAny, respAny, in.UsageEventID)
	if err != nil {
		return fmt.Errorf("更新 usage_event_details 失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO usage_event_details(usage_event_id, upstream_request_body, upstream_response_body, created_at, updated_at)
VALUES(?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, in.UsageEventID, reqAny, respAny)
	if err != nil {
		return fmt.Errorf("写入 usage_event_details 失败: %w", err)
	}
	return nil
}

func truncateDetail(v *string, maxBytes int) *string {
	if v == nil {
		return nil
	}
	if strings.TrimSpace(*v) == "" {
		return nil
	}
	if maxBytes <= 0 || len(*v) <= maxBytes {
		s := *v
		return &s
	}
	s := (*v)[:maxBytes] + "\n... (truncated)"
	return &s
}

func (s *Store) GetUsageEventDetail(ctx context.Context, usageEventID int64) (UsageEventDetail, error) {
	if usageEventID <= 0 {
		return UsageEventDetail{}, errors.New("usage_event_id 不能为空")
	}

	var d UsageEventDetail
	var req sql.NullString
	var resp sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT usage_event_id, upstream_request_body, upstream_response_body, created_at, updated_at
FROM usage_event_details
WHERE usage_event_id=?
`, usageEventID).Scan(&d.UsageEventID, &req, &resp, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UsageEventDetail{}, sql.ErrNoRows
		}
		return UsageEventDetail{}, fmt.Errorf("查询 usage_event_details 失败: %w", err)
	}
	if req.Valid {
		d.UpstreamRequestBody = &req.String
	}
	if resp.Valid {
		d.UpstreamResponseBody = &resp.String
	}
	return d, nil
}
