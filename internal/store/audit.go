// Package store 提供审计事件写入，确保关键管理操作与数据面请求可追溯（不记录输入内容与明文凭据）。
package store

import (
	"context"
	"fmt"
)

type AuditEventInput struct {
	Time               string
	RequestID          string
	ActorType          string
	UserID             *int64
	TokenID            *int64
	Action             string
	Endpoint           string
	Model              *string
	UpstreamChannelID  *int64
	UpstreamEndpointID *int64
	UpstreamCredID     *int64
	StatusCode         int
	LatencyMS          int
	ErrorClass         *string
	ErrorMessage       *string
}

func (s *Store) InsertAuditEvent(ctx context.Context, in AuditEventInput) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO audit_events(
  time, request_id, actor_type, user_id, token_id, action, endpoint, model,
  upstream_channel_id, upstream_endpoint_id, upstream_credential_id, status_code, latency_ms, error_class, error_message
) VALUES(
  CURRENT_TIMESTAMP, ?, ?, ?, ?, ?, ?, ?,
  ?, ?, ?, ?, ?, ?, ?
)
`, in.RequestID, in.ActorType, in.UserID, in.TokenID, in.Action, in.Endpoint, in.Model,
		in.UpstreamChannelID, in.UpstreamEndpointID, in.UpstreamCredID, in.StatusCode, in.LatencyMS, in.ErrorClass, in.ErrorMessage)
	if err != nil {
		return fmt.Errorf("写入 audit_events 失败: %w", err)
	}
	return nil
}
