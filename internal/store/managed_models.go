// Package store 封装模型目录（managed_models）的读写，用于模型白名单、展示元信息与定价。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

type ManagedModelCreate struct {
	PublicID       string
	OwnedBy        *string
	InputUSDPer1M  decimal.Decimal
	OutputUSDPer1M decimal.Decimal
	CacheUSDPer1M  decimal.Decimal
	Status         int
}

type ManagedModelUpdate struct {
	ID             int64
	PublicID       string
	OwnedBy        *string
	InputUSDPer1M  decimal.Decimal
	OutputUSDPer1M decimal.Decimal
	CacheUSDPer1M  decimal.Decimal
	Status         int
}

func normalizeManagedModelPricing(m *ManagedModel) error {
	m.InputUSDPer1M = m.InputUSDPer1M.Truncate(USDScale)
	m.OutputUSDPer1M = m.OutputUSDPer1M.Truncate(USDScale)
	m.CacheUSDPer1M = m.CacheUSDPer1M.Truncate(USDScale)
	if m.InputUSDPer1M.IsNegative() || m.OutputUSDPer1M.IsNegative() || m.CacheUSDPer1M.IsNegative() {
		return errors.New("模型定价不合法")
	}
	return nil
}

func (s *Store) ListManagedModels(ctx context.Context) ([]ManagedModel, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, public_id, upstream_model, owned_by,
       input_usd_per_1m, output_usd_per_1m, cache_usd_per_1m,
       status, created_at
FROM managed_models
ORDER BY status DESC, id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("查询 managed_models 失败: %w", err)
	}
	defer rows.Close()

	var out []ManagedModel
	for rows.Next() {
		var m ManagedModel
		var upstreamModel sql.NullString
		var ownedBy sql.NullString
		if err := rows.Scan(&m.ID, &m.PublicID, &upstreamModel, &ownedBy, &m.InputUSDPer1M, &m.OutputUSDPer1M, &m.CacheUSDPer1M, &m.Status, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描 managed_models 失败: %w", err)
		}
		if err := normalizeManagedModelPricing(&m); err != nil {
			return nil, err
		}
		if upstreamModel.Valid {
			v := upstreamModel.String
			m.UpstreamModel = &v
		}
		if ownedBy.Valid {
			v := ownedBy.String
			m.OwnedBy = &v
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 managed_models 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListEnabledManagedModels(ctx context.Context) ([]ManagedModel, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, public_id, upstream_model, owned_by,
       input_usd_per_1m, output_usd_per_1m, cache_usd_per_1m,
       status, created_at
FROM managed_models
WHERE status=1
ORDER BY id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("查询 managed_models 失败: %w", err)
	}
	defer rows.Close()

	var out []ManagedModel
	for rows.Next() {
		var m ManagedModel
		var upstreamModel sql.NullString
		var ownedBy sql.NullString
		if err := rows.Scan(&m.ID, &m.PublicID, &upstreamModel, &ownedBy, &m.InputUSDPer1M, &m.OutputUSDPer1M, &m.CacheUSDPer1M, &m.Status, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描 managed_models 失败: %w", err)
		}
		if err := normalizeManagedModelPricing(&m); err != nil {
			return nil, err
		}
		if upstreamModel.Valid {
			v := upstreamModel.String
			m.UpstreamModel = &v
		}
		if ownedBy.Valid {
			v := ownedBy.String
			m.OwnedBy = &v
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 managed_models 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListEnabledManagedModelsWithBindings(ctx context.Context) ([]ManagedModel, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT m.id, m.public_id, m.upstream_model, m.owned_by,
       m.input_usd_per_1m, m.output_usd_per_1m, m.cache_usd_per_1m,
       m.status, m.created_at
FROM managed_models m
JOIN channel_models cm ON cm.public_id=m.public_id AND cm.status=1
JOIN upstream_channels ch ON ch.id=cm.channel_id AND ch.status=1
WHERE m.status=1
ORDER BY m.id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("查询可用 managed_models 失败: %w", err)
	}
	defer rows.Close()

	var out []ManagedModel
	for rows.Next() {
		var m ManagedModel
		var upstreamModel sql.NullString
		var ownedBy sql.NullString
		if err := rows.Scan(&m.ID, &m.PublicID, &upstreamModel, &ownedBy, &m.InputUSDPer1M, &m.OutputUSDPer1M, &m.CacheUSDPer1M, &m.Status, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描可用 managed_models 失败: %w", err)
		}
		if err := normalizeManagedModelPricing(&m); err != nil {
			return nil, err
		}
		if upstreamModel.Valid {
			v := upstreamModel.String
			m.UpstreamModel = &v
		}
		if ownedBy.Valid {
			v := ownedBy.String
			m.OwnedBy = &v
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历可用 managed_models 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListEnabledManagedModelsWithBindingsForChannel(ctx context.Context, channelID int64) ([]ManagedModel, error) {
	if channelID == 0 {
		return nil, errors.New("channel_id 不能为空")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT m.id, m.public_id, m.upstream_model, m.owned_by,
       m.input_usd_per_1m, m.output_usd_per_1m, m.cache_usd_per_1m,
       m.status, m.created_at
FROM managed_models m
JOIN channel_models cm ON cm.public_id=m.public_id AND cm.status=1 AND cm.channel_id=?
JOIN upstream_channels ch ON ch.id=cm.channel_id AND ch.status=1
WHERE m.status=1
ORDER BY m.id DESC
`, channelID)
	if err != nil {
		return nil, fmt.Errorf("查询可用 managed_models 失败: %w", err)
	}
	defer rows.Close()

	var out []ManagedModel
	for rows.Next() {
		var m ManagedModel
		var upstreamModel sql.NullString
		var ownedBy sql.NullString
		if err := rows.Scan(&m.ID, &m.PublicID, &upstreamModel, &ownedBy, &m.InputUSDPer1M, &m.OutputUSDPer1M, &m.CacheUSDPer1M, &m.Status, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描可用 managed_models 失败: %w", err)
		}
		if err := normalizeManagedModelPricing(&m); err != nil {
			return nil, err
		}
		if upstreamModel.Valid {
			v := upstreamModel.String
			m.UpstreamModel = &v
		}
		if ownedBy.Valid {
			v := ownedBy.String
			m.OwnedBy = &v
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历可用 managed_models 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListEnabledManagedModelsWithBindingsForGroup(ctx context.Context, groupName string) ([]ManagedModel, error) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return nil, errors.New("group_name 不能为空")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT m.id, m.public_id, m.upstream_model, m.owned_by,
       m.input_usd_per_1m, m.output_usd_per_1m, m.cache_usd_per_1m,
       m.status, m.created_at
FROM managed_models m
JOIN channel_models cm ON cm.public_id=m.public_id AND cm.status=1
JOIN upstream_channels ch ON ch.id=cm.channel_id AND ch.status=1
WHERE m.status=1 AND FIND_IN_SET(?, ch.groups) > 0
ORDER BY m.id DESC
`, groupName)
	if err != nil {
		return nil, fmt.Errorf("查询可用 managed_models 失败: %w", err)
	}
	defer rows.Close()

	var out []ManagedModel
	for rows.Next() {
		var m ManagedModel
		var upstreamModel sql.NullString
		var ownedBy sql.NullString
		if err := rows.Scan(&m.ID, &m.PublicID, &upstreamModel, &ownedBy, &m.InputUSDPer1M, &m.OutputUSDPer1M, &m.CacheUSDPer1M, &m.Status, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描可用 managed_models 失败: %w", err)
		}
		if err := normalizeManagedModelPricing(&m); err != nil {
			return nil, err
		}
		if upstreamModel.Valid {
			v := upstreamModel.String
			m.UpstreamModel = &v
		}
		if ownedBy.Valid {
			v := ownedBy.String
			m.OwnedBy = &v
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历可用 managed_models 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListEnabledManagedModelsWithBindingsForGroups(ctx context.Context, groups []string) ([]ManagedModel, error) {
	seen := make(map[string]struct{}, len(groups))
	var groupNames []string
	for _, g := range groups {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		groupNames = append(groupNames, g)
	}
	if len(groupNames) == 0 {
		groupNames = []string{DefaultGroupName}
	}
	if len(groupNames) > 20 {
		groupNames = groupNames[:20]
	}

	var b strings.Builder
	b.WriteString(`
SELECT DISTINCT m.id, m.public_id, m.upstream_model, m.owned_by,
       m.input_usd_per_1m, m.output_usd_per_1m, m.cache_usd_per_1m,
       m.status, m.created_at
FROM managed_models m
JOIN channel_models cm ON cm.public_id=m.public_id AND cm.status=1
JOIN upstream_channels ch ON ch.id=cm.channel_id AND ch.status=1
WHERE m.status=1 AND (`)

	args := make([]any, 0, len(groupNames))
	for i, g := range groupNames {
		if i > 0 {
			b.WriteString(" OR ")
		}
		b.WriteString("FIND_IN_SET(?, ch.groups) > 0")
		args = append(args, g)
	}
	b.WriteString(")\nORDER BY m.id DESC\n")

	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("查询可用 managed_models 失败: %w", err)
	}
	defer rows.Close()

	var out []ManagedModel
	for rows.Next() {
		var m ManagedModel
		var upstreamModel sql.NullString
		var ownedBy sql.NullString
		if err := rows.Scan(&m.ID, &m.PublicID, &upstreamModel, &ownedBy, &m.InputUSDPer1M, &m.OutputUSDPer1M, &m.CacheUSDPer1M, &m.Status, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描可用 managed_models 失败: %w", err)
		}
		if err := normalizeManagedModelPricing(&m); err != nil {
			return nil, err
		}
		if upstreamModel.Valid {
			v := upstreamModel.String
			m.UpstreamModel = &v
		}
		if ownedBy.Valid {
			v := ownedBy.String
			m.OwnedBy = &v
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历可用 managed_models 失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetManagedModelByID(ctx context.Context, id int64) (ManagedModel, error) {
	var m ManagedModel
	var upstreamModel sql.NullString
	var ownedBy sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, public_id, upstream_model, owned_by,
       input_usd_per_1m, output_usd_per_1m, cache_usd_per_1m,
       status, created_at
FROM managed_models
WHERE id=?
`, id).Scan(&m.ID, &m.PublicID, &upstreamModel, &ownedBy, &m.InputUSDPer1M, &m.OutputUSDPer1M, &m.CacheUSDPer1M, &m.Status, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ManagedModel{}, sql.ErrNoRows
		}
		return ManagedModel{}, fmt.Errorf("查询 managed_model 失败: %w", err)
	}
	if err := normalizeManagedModelPricing(&m); err != nil {
		return ManagedModel{}, err
	}
	if upstreamModel.Valid {
		v := upstreamModel.String
		m.UpstreamModel = &v
	}
	if ownedBy.Valid {
		v := ownedBy.String
		m.OwnedBy = &v
	}
	return m, nil
}

func (s *Store) GetManagedModelByPublicID(ctx context.Context, publicID string) (ManagedModel, error) {
	var m ManagedModel
	var upstreamModel sql.NullString
	var ownedBy sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, public_id, upstream_model, owned_by,
       input_usd_per_1m, output_usd_per_1m, cache_usd_per_1m,
       status, created_at
FROM managed_models
WHERE public_id=?
LIMIT 1
`, publicID).Scan(&m.ID, &m.PublicID, &upstreamModel, &ownedBy, &m.InputUSDPer1M, &m.OutputUSDPer1M, &m.CacheUSDPer1M, &m.Status, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ManagedModel{}, sql.ErrNoRows
		}
		return ManagedModel{}, fmt.Errorf("查询 managed_model 失败: %w", err)
	}
	if err := normalizeManagedModelPricing(&m); err != nil {
		return ManagedModel{}, err
	}
	if upstreamModel.Valid {
		v := upstreamModel.String
		m.UpstreamModel = &v
	}
	if ownedBy.Valid {
		v := ownedBy.String
		m.OwnedBy = &v
	}
	return m, nil
}

func (s *Store) GetEnabledManagedModelByPublicID(ctx context.Context, publicID string) (ManagedModel, error) {
	var m ManagedModel
	var upstreamModel sql.NullString
	var ownedBy sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, public_id, upstream_model, owned_by,
       input_usd_per_1m, output_usd_per_1m, cache_usd_per_1m,
       status, created_at
FROM managed_models
WHERE public_id=? AND status=1
LIMIT 1
`, publicID).Scan(&m.ID, &m.PublicID, &upstreamModel, &ownedBy, &m.InputUSDPer1M, &m.OutputUSDPer1M, &m.CacheUSDPer1M, &m.Status, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ManagedModel{}, sql.ErrNoRows
		}
		return ManagedModel{}, fmt.Errorf("查询 managed_model 失败: %w", err)
	}
	if err := normalizeManagedModelPricing(&m); err != nil {
		return ManagedModel{}, err
	}
	if upstreamModel.Valid {
		v := upstreamModel.String
		m.UpstreamModel = &v
	}
	if ownedBy.Valid {
		v := ownedBy.String
		m.OwnedBy = &v
	}
	return m, nil
}

func (s *Store) CreateManagedModel(ctx context.Context, in ManagedModelCreate) (int64, error) {
	inUSD := in.InputUSDPer1M.Truncate(USDScale)
	outUSD := in.OutputUSDPer1M.Truncate(USDScale)
	cacheUSD := in.CacheUSDPer1M.Truncate(USDScale)
	if inUSD.IsNegative() || outUSD.IsNegative() || cacheUSD.IsNegative() {
		return 0, errors.New("模型定价不合法")
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO managed_models(
  public_id, owned_by,
  input_usd_per_1m, output_usd_per_1m, cache_usd_per_1m,
  status, created_at
) VALUES(
  ?, ?,
  ?, ?, ?,
  ?, CURRENT_TIMESTAMP
)
`, in.PublicID, in.OwnedBy, inUSD, outUSD, cacheUSD, in.Status)
	if err != nil {
		return 0, fmt.Errorf("创建 managed_model 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取 managed_model id 失败: %w", err)
	}
	return id, nil
}

func (s *Store) UpdateManagedModel(ctx context.Context, in ManagedModelUpdate) error {
	if in.ID == 0 {
		return errors.New("id 不能为空")
	}
	inUSD := in.InputUSDPer1M.Truncate(USDScale)
	outUSD := in.OutputUSDPer1M.Truncate(USDScale)
	cacheUSD := in.CacheUSDPer1M.Truncate(USDScale)
	if inUSD.IsNegative() || outUSD.IsNegative() || cacheUSD.IsNegative() {
		return errors.New("模型定价不合法")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var oldPublicID string
	if err := tx.QueryRowContext(ctx, `SELECT public_id FROM managed_models WHERE id=?`, in.ID).Scan(&oldPublicID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("查询旧 public_id 失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE managed_models
SET public_id=?, owned_by=?,
    input_usd_per_1m=?, output_usd_per_1m=?, cache_usd_per_1m=?,
    status=?
WHERE id=?
`, in.PublicID, in.OwnedBy, inUSD, outUSD, cacheUSD, in.Status, in.ID); err != nil {
		return fmt.Errorf("更新 managed_model 失败: %w", err)
	}

	if oldPublicID != in.PublicID {
		if _, err := tx.ExecContext(ctx, `
UPDATE channel_models
SET public_id=?, updated_at=CURRENT_TIMESTAMP
WHERE public_id=?
`, in.PublicID, oldPublicID); err != nil {
			return fmt.Errorf("同步更新 channel_models public_id 失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) DeleteManagedModel(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM managed_models WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("删除 managed_model 失败: %w", err)
	}
	return nil
}

type ManagedModelPricingUpsert struct {
	PublicID       string
	InputUSDPer1M  decimal.Decimal
	OutputUSDPer1M decimal.Decimal
	CacheUSDPer1M  decimal.Decimal
}

type UpsertManagedModelPricingResult struct {
	Added     []string
	Updated   []string
	Unchanged []string
}

func (s *Store) UpsertManagedModelPricing(ctx context.Context, items []ManagedModelPricingUpsert) (UpsertManagedModelPricingResult, error) {
	var res UpsertManagedModelPricingResult
	if len(items) == 0 {
		return res, nil
	}

	byPublicID := make(map[string]ManagedModelPricingUpsert, len(items))
	for _, it := range items {
		id := strings.TrimSpace(it.PublicID)
		if id == "" {
			return UpsertManagedModelPricingResult{}, errors.New("public_id 不能为空")
		}
		it.PublicID = id
		it.InputUSDPer1M = it.InputUSDPer1M.Truncate(USDScale)
		it.OutputUSDPer1M = it.OutputUSDPer1M.Truncate(USDScale)
		it.CacheUSDPer1M = it.CacheUSDPer1M.Truncate(USDScale)
		if it.InputUSDPer1M.IsNegative() || it.OutputUSDPer1M.IsNegative() || it.CacheUSDPer1M.IsNegative() {
			return UpsertManagedModelPricingResult{}, errors.New("模型定价不合法")
		}
		byPublicID[id] = it
	}

	publicIDs := make([]string, 0, len(byPublicID))
	for id := range byPublicID {
		publicIDs = append(publicIDs, id)
	}
	sort.Strings(publicIDs)

	var b strings.Builder
	b.WriteString("SELECT id, public_id, input_usd_per_1m, output_usd_per_1m, cache_usd_per_1m\n")
	b.WriteString("FROM managed_models\n")
	b.WriteString("WHERE public_id IN (")
	args := make([]any, 0, len(publicIDs))
	for i, id := range publicIDs {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("?")
		args = append(args, id)
	}
	b.WriteString(")")

	type existing struct {
		ID             int64
		InputUSDPer1M  decimal.Decimal
		OutputUSDPer1M decimal.Decimal
		CacheUSDPer1M  decimal.Decimal
	}
	existingByPublicID := make(map[string]existing, len(publicIDs))
	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return UpsertManagedModelPricingResult{}, fmt.Errorf("查询 managed_models 失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e existing
		var pid string
		if err := rows.Scan(&e.ID, &pid, &e.InputUSDPer1M, &e.OutputUSDPer1M, &e.CacheUSDPer1M); err != nil {
			return UpsertManagedModelPricingResult{}, fmt.Errorf("扫描 managed_models 失败: %w", err)
		}
		pid = strings.TrimSpace(pid)
		if pid == "" || e.ID == 0 {
			continue
		}
		existingByPublicID[pid] = e
	}
	if err := rows.Err(); err != nil {
		return UpsertManagedModelPricingResult{}, fmt.Errorf("遍历 managed_models 失败: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpsertManagedModelPricingResult{}, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	updateStmt, err := tx.PrepareContext(ctx, `
UPDATE managed_models
SET input_usd_per_1m=?, output_usd_per_1m=?, cache_usd_per_1m=?
WHERE id=?
`)
	if err != nil {
		return UpsertManagedModelPricingResult{}, fmt.Errorf("准备更新 managed_models 失败: %w", err)
	}
	defer updateStmt.Close()

	insertStmt, err := tx.PrepareContext(ctx, `
INSERT INTO managed_models(
  public_id, owned_by,
  input_usd_per_1m, output_usd_per_1m, cache_usd_per_1m,
  status, created_at
) VALUES(
  ?, NULL,
  ?, ?, ?,
  0, CURRENT_TIMESTAMP
)
`)
	if err != nil {
		return UpsertManagedModelPricingResult{}, fmt.Errorf("准备创建 managed_models 失败: %w", err)
	}
	defer insertStmt.Close()

	for _, publicID := range publicIDs {
		it := byPublicID[publicID]
		if e, ok := existingByPublicID[publicID]; ok {
			inUSD := it.InputUSDPer1M.Truncate(USDScale)
			outUSD := it.OutputUSDPer1M.Truncate(USDScale)
			cacheUSD := it.CacheUSDPer1M.Truncate(USDScale)
			if e.InputUSDPer1M.Truncate(USDScale).Equal(inUSD) &&
				e.OutputUSDPer1M.Truncate(USDScale).Equal(outUSD) &&
				e.CacheUSDPer1M.Truncate(USDScale).Equal(cacheUSD) {
				res.Unchanged = append(res.Unchanged, publicID)
				continue
			}
			if _, err := updateStmt.ExecContext(ctx, inUSD, outUSD, cacheUSD, e.ID); err != nil {
				return UpsertManagedModelPricingResult{}, fmt.Errorf("更新 managed_model(%s) 失败: %w", publicID, err)
			}
			res.Updated = append(res.Updated, publicID)
			continue
		}
		if _, err := insertStmt.ExecContext(ctx, publicID, it.InputUSDPer1M, it.OutputUSDPer1M, it.CacheUSDPer1M); err != nil {
			return UpsertManagedModelPricingResult{}, fmt.Errorf("创建 managed_model(%s) 失败: %w", publicID, err)
		}
		res.Added = append(res.Added, publicID)
	}

	if err := tx.Commit(); err != nil {
		return UpsertManagedModelPricingResult{}, fmt.Errorf("提交事务失败: %w", err)
	}
	return res, nil
}
