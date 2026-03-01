package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ErrorPassthroughRule struct {
	ID              int64
	Name            string
	Enabled         bool
	Priority        int
	ErrorCodes      []int
	Keywords        []string
	MatchMode       string
	Platforms       []string
	PassthroughCode bool
	ResponseCode    *int
	PassthroughBody bool
	CustomMessage   *string
	SkipMonitoring  bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (s *Store) ListErrorPassthroughRules(ctx context.Context) ([]ErrorPassthroughRule, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  id,
  name,
  enabled,
  priority,
  error_codes_json,
  keywords_json,
  match_mode,
  platforms_json,
  passthrough_code,
  response_code,
  passthrough_body,
  custom_message,
  skip_monitoring,
  created_at,
  updated_at
FROM error_passthrough_rules
ORDER BY priority ASC, id ASC
`)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") ||
			strings.Contains(strings.ToLower(err.Error()), "doesn't exist") {
			return nil, nil
		}
		return nil, fmt.Errorf("查询错误透传规则失败: %w", err)
	}
	defer rows.Close()

	out := make([]ErrorPassthroughRule, 0, 16)
	for rows.Next() {
		var (
			r ErrorPassthroughRule

			enabledI         int
			passthroughCodeI int
			passthroughBodyI int
			skipMonitoringI  int

			errorCodesJSON string
			keywordsJSON   string
			platformsJSON  string

			responseCode sql.NullInt64
			customMsg    sql.NullString
		)
		if err := rows.Scan(
			&r.ID,
			&r.Name,
			&enabledI,
			&r.Priority,
			&errorCodesJSON,
			&keywordsJSON,
			&r.MatchMode,
			&platformsJSON,
			&passthroughCodeI,
			&responseCode,
			&passthroughBodyI,
			&customMsg,
			&skipMonitoringI,
			&r.CreatedAt,
			&r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描错误透传规则失败: %w", err)
		}
		r.Enabled = enabledI == 1
		r.PassthroughCode = passthroughCodeI == 1
		r.PassthroughBody = passthroughBodyI == 1
		r.SkipMonitoring = skipMonitoringI == 1

		if responseCode.Valid {
			v := int(responseCode.Int64)
			r.ResponseCode = &v
		}
		if customMsg.Valid {
			msg := strings.TrimSpace(customMsg.String)
			if msg != "" {
				r.CustomMessage = &msg
			}
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(errorCodesJSON)), &r.ErrorCodes); err != nil {
			r.ErrorCodes = nil
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(keywordsJSON)), &r.Keywords); err != nil {
			r.Keywords = nil
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(platformsJSON)), &r.Platforms); err != nil {
			r.Platforms = nil
		}
		r.MatchMode = strings.ToLower(strings.TrimSpace(r.MatchMode))
		if r.MatchMode == "" {
			r.MatchMode = "any"
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历错误透传规则失败: %w", err)
	}
	return out, nil
}
