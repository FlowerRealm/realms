package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const (
	CacheInvalidationKeyUpstreamSnapshot = "upstream_snapshot"
	CacheInvalidationKeyTokenAuth        = "token_auth"
)

func normalizeInvalidationKey(key string) (string, error) {
	k := strings.TrimSpace(key)
	if k == "" {
		return "", errors.New("cache_key 不能为空")
	}
	if len(k) > 64 {
		return "", errors.New("cache_key 过长")
	}
	return k, nil
}

// BumpCacheInvalidation increments the version for a given cache key.
//
// Best-effort: if the table doesn't exist (older DB), it returns nil.
func (s *Store) BumpCacheInvalidation(ctx context.Context, key string) error {
	if s == nil || s.db == nil {
		return nil
	}
	k, err := normalizeInvalidationKey(key)
	if err != nil {
		return err
	}

	query := `
INSERT INTO cache_invalidation(cache_key, version, updated_at)
VALUES(?, 1, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE version=version+1, updated_at=CURRENT_TIMESTAMP
`
	if s.dialect == DialectSQLite {
		query = `
INSERT INTO cache_invalidation(cache_key, version, updated_at)
VALUES(?, 1, CURRENT_TIMESTAMP)
ON CONFLICT(cache_key) DO UPDATE SET version=cache_invalidation.version+1, updated_at=CURRENT_TIMESTAMP
`
	}
	if _, err := s.db.ExecContext(ctx, query, k); err != nil {
		if isMissingTableErr(err) {
			return nil
		}
		return fmt.Errorf("bump cache_invalidation(%s) 失败: %w", k, err)
	}
	return nil
}

// GetCacheInvalidationVersion returns (version, ok, err).
//
// ok=false when the row doesn't exist or the table doesn't exist.
func (s *Store) GetCacheInvalidationVersion(ctx context.Context, key string) (int64, bool, error) {
	if s == nil || s.db == nil {
		return 0, false, nil
	}
	k, err := normalizeInvalidationKey(key)
	if err != nil {
		return 0, false, err
	}
	var v int64
	err = s.db.QueryRowContext(ctx, `SELECT version FROM cache_invalidation WHERE cache_key=?`, k).Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || isMissingTableErr(err) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("读取 cache_invalidation(%s) 失败: %w", k, err)
	}
	return v, true, nil
}

// GetCacheInvalidationVersions returns the versions for a list of keys.
//
// Behavior:
// - Missing table: returns (nil, false, nil)
// - Missing rows: keys absent from the returned map
func (s *Store) GetCacheInvalidationVersions(ctx context.Context, keys []string) (map[string]int64, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, nil
	}
	if len(keys) == 0 {
		return map[string]int64{}, true, nil
	}

	norm := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		nk, err := normalizeInvalidationKey(k)
		if err != nil {
			return nil, false, err
		}
		if _, ok := seen[nk]; ok {
			continue
		}
		seen[nk] = struct{}{}
		norm = append(norm, nk)
	}
	if len(norm) == 0 {
		return map[string]int64{}, true, nil
	}

	args := make([]any, 0, len(norm))
	ph := make([]string, 0, len(norm))
	for _, k := range norm {
		args = append(args, k)
		ph = append(ph, "?")
	}
	q := `SELECT cache_key, version FROM cache_invalidation WHERE cache_key IN (` + strings.Join(ph, ",") + `)`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		if isMissingTableErr(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("批量读取 cache_invalidation 失败: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int64, len(norm))
	for rows.Next() {
		var (
			k string
			v int64
		)
		if err := rows.Scan(&k, &v); err != nil {
			return nil, false, fmt.Errorf("扫描 cache_invalidation 失败: %w", err)
		}
		k = strings.TrimSpace(k)
		if k != "" {
			out[k] = v
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("遍历 cache_invalidation 失败: %w", err)
	}
	return out, true, nil
}
