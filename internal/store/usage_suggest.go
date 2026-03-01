package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type UpstreamChannelLite struct {
	ID   int64
	Name string
	Type string
}

func (s *Store) SuggestUsageChannels(ctx context.Context, since, until time.Time, q string, limit int) ([]UpstreamChannelLite, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []UpstreamChannelLite{}, nil
	}
	if limit <= 0 {
		return []UpstreamChannelLite{}, nil
	}

	p := buildLikePattern(q)
	rows, err := s.db.QueryContext(ctx, `
SELECT uc.id, uc.name, uc.type
FROM usage_events ue
JOIN upstream_channels uc ON uc.id=ue.upstream_channel_id
WHERE ue.time >= ? AND ue.time < ?
  AND ue.state <> ?
  AND ue.upstream_channel_id > 0
  AND uc.name LIKE ?
GROUP BY uc.id, uc.name, uc.type
ORDER BY MAX(ue.time) DESC, uc.id DESC
LIMIT ?
`, since, until, UsageStateReserved, p, limit)
	if err != nil {
		return nil, fmt.Errorf("查询 usage_events channels suggest 失败: %w", err)
	}
	defer rows.Close()

	out := make([]UpstreamChannelLite, 0, limit)
	for rows.Next() {
		var v UpstreamChannelLite
		if err := rows.Scan(&v.ID, &v.Name, &v.Type); err != nil {
			return nil, fmt.Errorf("扫描 channels suggest 失败: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 channels suggest 失败: %w", err)
	}
	return out, nil
}

func (s *Store) SuggestUsageModels(ctx context.Context, since, until time.Time, q string, limit int) ([]string, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []string{}, nil
	}
	if limit <= 0 {
		return []string{}, nil
	}

	p := buildLikePattern(q)
	rows, err := s.db.QueryContext(ctx, `
SELECT ue.model, MAX(ue.id) AS last_id
FROM usage_events ue
WHERE ue.time >= ? AND ue.time < ?
  AND ue.state <> ?
  AND ue.model IS NOT NULL
  AND ue.model <> ''
  AND ue.model LIKE ?
GROUP BY ue.model
ORDER BY last_id DESC
LIMIT ?
`, since, until, UsageStateReserved, p, limit)
	if err != nil {
		return nil, fmt.Errorf("查询 usage_events models suggest 失败: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0, limit)
	for rows.Next() {
		var model sql.NullString
		var _lastID int64
		if err := rows.Scan(&model, &_lastID); err != nil {
			return nil, fmt.Errorf("扫描 models suggest 失败: %w", err)
		}
		if !model.Valid {
			continue
		}
		m := strings.TrimSpace(model.String)
		if m == "" {
			continue
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 models suggest 失败: %w", err)
	}
	return out, nil
}
