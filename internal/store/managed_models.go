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
	PublicID                   string
	GroupName                  string
	OwnedBy                    *string
	InputUSDPer1M              decimal.Decimal
	OutputUSDPer1M             decimal.Decimal
	CacheInputUSDPer1M         decimal.Decimal
	CacheOutputUSDPer1M        decimal.Decimal
	PriorityPricingEnabled     bool
	PriorityInputUSDPer1M      *decimal.Decimal
	PriorityOutputUSDPer1M     *decimal.Decimal
	PriorityCacheInputUSDPer1M *decimal.Decimal
	HighContextPricing         *ManagedModelHighContextPricing
	Status                     int
}

type ManagedModelUpdate struct {
	ID                         int64
	PublicID                   string
	GroupName                  string
	OwnedBy                    *string
	InputUSDPer1M              decimal.Decimal
	OutputUSDPer1M             decimal.Decimal
	CacheInputUSDPer1M         decimal.Decimal
	CacheOutputUSDPer1M        decimal.Decimal
	PriorityPricingEnabled     bool
	PriorityInputUSDPer1M      *decimal.Decimal
	PriorityOutputUSDPer1M     *decimal.Decimal
	PriorityCacheInputUSDPer1M *decimal.Decimal
	HighContextPricing         *ManagedModelHighContextPricing
	Status                     int
}

const managedModelSelectColumns = `id, public_id, group_name, upstream_model, owned_by,
       input_usd_per_1m, output_usd_per_1m, cache_input_usd_per_1m, cache_output_usd_per_1m,
       priority_pricing_enabled, priority_input_usd_per_1m, priority_output_usd_per_1m, priority_cache_input_usd_per_1m,
       high_context_pricing_json, status, created_at`

const managedModelSelectColumnsWithAliasM = `m.id, m.public_id, m.group_name, m.upstream_model, m.owned_by,
       m.input_usd_per_1m, m.output_usd_per_1m, m.cache_input_usd_per_1m, m.cache_output_usd_per_1m,
       m.priority_pricing_enabled, m.priority_input_usd_per_1m, m.priority_output_usd_per_1m, m.priority_cache_input_usd_per_1m,
       m.high_context_pricing_json, m.status, m.created_at`

type managedModelScanner interface {
	Scan(dest ...any) error
}

func normalizeOptionalManagedModelPrice(v *decimal.Decimal) (*decimal.Decimal, error) {
	if v == nil {
		return nil, nil
	}
	n := v.Truncate(USDScale)
	if n.IsNegative() {
		return nil, errors.New("模型定价不合法")
	}
	return &n, nil
}

func parseOptionalManagedModelPrice(v sql.NullString) (*decimal.Decimal, error) {
	if !v.Valid {
		return nil, nil
	}
	s := strings.TrimSpace(v.String)
	if s == "" {
		return nil, nil
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return nil, errors.New("模型定价不合法")
	}
	d = d.Truncate(USDScale)
	if d.IsNegative() {
		return nil, errors.New("模型定价不合法")
	}
	return &d, nil
}

func validateManagedModelPriorityPricing(enabled bool, inUSD, outUSD *decimal.Decimal) error {
	if !enabled {
		return nil
	}
	if inUSD == nil || outUSD == nil {
		return errors.New("fast mode 定价不合法")
	}
	return nil
}

func scanManagedModelRow(scanner managedModelScanner) (ManagedModel, error) {
	var m ManagedModel
	var groupName sql.NullString
	var upstreamModel sql.NullString
	var ownedBy sql.NullString
	var priorityPricingEnabled int
	var priorityInputUSD sql.NullString
	var priorityOutputUSD sql.NullString
	var priorityCacheInputUSD sql.NullString
	var highContextPricingJSON sql.NullString
	if err := scanner.Scan(
		&m.ID, &m.PublicID, &groupName, &upstreamModel, &ownedBy,
		&m.InputUSDPer1M, &m.OutputUSDPer1M, &m.CacheInputUSDPer1M, &m.CacheOutputUSDPer1M,
		&priorityPricingEnabled, &priorityInputUSD, &priorityOutputUSD, &priorityCacheInputUSD,
		&highContextPricingJSON, &m.Status, &m.CreatedAt,
	); err != nil {
		return ManagedModel{}, err
	}
	if err := normalizeManagedModelPricing(&m); err != nil {
		return ManagedModel{}, err
	}
	m.PriorityPricingEnabled = priorityPricingEnabled != 0
	var err error
	m.PriorityInputUSDPer1M, err = parseOptionalManagedModelPrice(priorityInputUSD)
	if err != nil {
		return ManagedModel{}, err
	}
	m.PriorityOutputUSDPer1M, err = parseOptionalManagedModelPrice(priorityOutputUSD)
	if err != nil {
		return ManagedModel{}, err
	}
	m.PriorityCacheInputUSDPer1M, err = parseOptionalManagedModelPrice(priorityCacheInputUSD)
	if err != nil {
		return ManagedModel{}, err
	}
	if err := validateManagedModelPriorityPricing(m.PriorityPricingEnabled, m.PriorityInputUSDPer1M, m.PriorityOutputUSDPer1M); err != nil {
		return ManagedModel{}, err
	}
	m.HighContextPricing, err = parseManagedModelHighContextPricing(highContextPricingJSON)
	if err != nil {
		return ManagedModel{}, err
	}
	m.GroupName = normalizeManagedModelGroupName(groupName.String)
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

func normalizeManagedModelPricing(m *ManagedModel) error {
	m.InputUSDPer1M = m.InputUSDPer1M.Truncate(USDScale)
	m.OutputUSDPer1M = m.OutputUSDPer1M.Truncate(USDScale)
	m.CacheInputUSDPer1M = m.CacheInputUSDPer1M.Truncate(USDScale)
	m.CacheOutputUSDPer1M = m.CacheOutputUSDPer1M.Truncate(USDScale)
	if m.InputUSDPer1M.IsNegative() || m.OutputUSDPer1M.IsNegative() || m.CacheInputUSDPer1M.IsNegative() || m.CacheOutputUSDPer1M.IsNegative() {
		return errors.New("模型定价不合法")
	}
	var err error
	m.PriorityInputUSDPer1M, err = normalizeOptionalManagedModelPrice(m.PriorityInputUSDPer1M)
	if err != nil {
		return err
	}
	m.PriorityOutputUSDPer1M, err = normalizeOptionalManagedModelPrice(m.PriorityOutputUSDPer1M)
	if err != nil {
		return err
	}
	m.PriorityCacheInputUSDPer1M, err = normalizeOptionalManagedModelPrice(m.PriorityCacheInputUSDPer1M)
	if err != nil {
		return err
	}
	m.HighContextPricing, err = normalizeManagedModelHighContextPricing(m.HighContextPricing)
	if err != nil {
		return err
	}
	return nil
}

func normalizeManagedModelGroupName(groupName string) string {
	return strings.TrimSpace(groupName)
}

func (s *Store) ListManagedModels(ctx context.Context) ([]ManagedModel, error) {
	rows, err := s.db.QueryContext(ctx, `
	SELECT `+managedModelSelectColumns+`
	FROM managed_models
	ORDER BY status DESC, id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("查询 managed_models 失败: %w", err)
	}
	defer rows.Close()

	var out []ManagedModel
	for rows.Next() {
		m, err := scanManagedModelRow(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描 managed_models 失败: %w", err)
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
	SELECT `+managedModelSelectColumns+`
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
		m, err := scanManagedModelRow(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描 managed_models 失败: %w", err)
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
SELECT DISTINCT `+managedModelSelectColumnsWithAliasM+`
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
		m, err := scanManagedModelRow(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描可用 managed_models 失败: %w", err)
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
SELECT DISTINCT `+managedModelSelectColumnsWithAliasM+`
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
		m, err := scanManagedModelRow(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描可用 managed_models 失败: %w", err)
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
	groupsCol := "`groups`"
	query := fmt.Sprintf(`
	SELECT DISTINCT `+managedModelSelectColumnsWithAliasM+`
	FROM managed_models m
	JOIN channel_models cm ON cm.public_id=m.public_id AND cm.status=1
	JOIN upstream_channels ch ON ch.id=cm.channel_id AND ch.status=1
	WHERE m.status=1
	  AND TRIM(m.group_name)=?
	  AND FIND_IN_SET(?, ch.%s) > 0
	ORDER BY m.id DESC
	`, groupsCol)
	if s.dialect == DialectSQLite {
		query = fmt.Sprintf(`
	SELECT DISTINCT `+managedModelSelectColumnsWithAliasM+`
	FROM managed_models m
	JOIN channel_models cm ON cm.public_id=m.public_id AND cm.status=1
	JOIN upstream_channels ch ON ch.id=cm.channel_id AND ch.status=1
	WHERE m.status=1
	  AND TRIM(m.group_name)=?
	  AND INSTR(',' || REPLACE(IFNULL(ch.%s, ''), ' ', '') || ',', ',' || ? || ',') > 0
	ORDER BY m.id DESC
	`, groupsCol)
	}

	rows, err := s.db.QueryContext(ctx, query, groupName, groupName)
	if err != nil {
		return nil, fmt.Errorf("查询可用 managed_models 失败: %w", err)
	}
	defer rows.Close()

	var out []ManagedModel
	for rows.Next() {
		m, err := scanManagedModelRow(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描可用 managed_models 失败: %w", err)
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
		return nil, nil
	}
	if len(groupNames) > 20 {
		groupNames = groupNames[:20]
	}

	var b strings.Builder
	b.WriteString(`
SELECT DISTINCT ` + managedModelSelectColumnsWithAliasM + `
FROM managed_models m
JOIN channel_models cm ON cm.public_id=m.public_id AND cm.status=1
JOIN upstream_channels ch ON ch.id=cm.channel_id AND ch.status=1
WHERE m.status=1
  AND TRIM(m.group_name) IN (`)

	args := make([]any, 0, len(groupNames)*2)
	for i, g := range groupNames {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("?")
		args = append(args, g)
	}
	b.WriteString(`)
  AND (`)

	for i, g := range groupNames {
		if i > 0 {
			b.WriteString(" OR ")
		}
		if s.dialect == DialectSQLite {
			b.WriteString("INSTR(',' || REPLACE(IFNULL(ch.`groups`, ''), ' ', '') || ',', ',' || ? || ',') > 0")
		} else {
			b.WriteString("FIND_IN_SET(?, ch.`groups`) > 0")
		}
		args = append(args, g)
	}
	b.WriteString(`)
ORDER BY m.id DESC
`)

	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("查询可用 managed_models 失败: %w", err)
	}
	defer rows.Close()

	var out []ManagedModel
	for rows.Next() {
		m, err := scanManagedModelRow(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描可用 managed_models 失败: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历可用 managed_models 失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetManagedModelByID(ctx context.Context, id int64) (ManagedModel, error) {
	m, err := scanManagedModelRow(s.db.QueryRowContext(ctx, `
SELECT `+managedModelSelectColumns+`
FROM managed_models
WHERE id=?
`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ManagedModel{}, sql.ErrNoRows
		}
		return ManagedModel{}, fmt.Errorf("查询 managed_model 失败: %w", err)
	}
	return m, nil
}

func (s *Store) GetManagedModelByPublicID(ctx context.Context, publicID string) (ManagedModel, error) {
	m, err := scanManagedModelRow(s.db.QueryRowContext(ctx, `
SELECT `+managedModelSelectColumns+`
FROM managed_models
WHERE public_id=?
LIMIT 1
`, publicID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ManagedModel{}, sql.ErrNoRows
		}
		return ManagedModel{}, fmt.Errorf("查询 managed_model 失败: %w", err)
	}
	return m, nil
}

func (s *Store) GetEnabledManagedModelByPublicID(ctx context.Context, publicID string) (ManagedModel, error) {
	m, err := scanManagedModelRow(s.db.QueryRowContext(ctx, `
SELECT `+managedModelSelectColumns+`
FROM managed_models
WHERE public_id=? AND status=1
LIMIT 1
`, publicID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ManagedModel{}, sql.ErrNoRows
		}
		return ManagedModel{}, fmt.Errorf("查询 managed_model 失败: %w", err)
	}
	return m, nil
}

func (s *Store) CreateManagedModel(ctx context.Context, in ManagedModelCreate) (int64, error) {
	in.GroupName = normalizeManagedModelGroupName(in.GroupName)
	inUSD := in.InputUSDPer1M.Truncate(USDScale)
	outUSD := in.OutputUSDPer1M.Truncate(USDScale)
	cacheInUSD := in.CacheInputUSDPer1M.Truncate(USDScale)
	cacheOutUSD := in.CacheOutputUSDPer1M.Truncate(USDScale)
	priorityInUSD, err := normalizeOptionalManagedModelPrice(in.PriorityInputUSDPer1M)
	if err != nil {
		return 0, err
	}
	priorityOutUSD, err := normalizeOptionalManagedModelPrice(in.PriorityOutputUSDPer1M)
	if err != nil {
		return 0, err
	}
	priorityCacheInUSD, err := normalizeOptionalManagedModelPrice(in.PriorityCacheInputUSDPer1M)
	if err != nil {
		return 0, err
	}
	highContextPricingJSON, err := marshalManagedModelHighContextPricing(in.HighContextPricing)
	if err != nil {
		return 0, err
	}
	if inUSD.IsNegative() || outUSD.IsNegative() || cacheInUSD.IsNegative() || cacheOutUSD.IsNegative() {
		return 0, errors.New("模型定价不合法")
	}
	if err := validateManagedModelPriorityPricing(in.PriorityPricingEnabled, priorityInUSD, priorityOutUSD); err != nil {
		return 0, err
	}
	priorityEnabled := 0
	if in.PriorityPricingEnabled {
		priorityEnabled = 1
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO managed_models(
  public_id, group_name, owned_by,
  input_usd_per_1m, output_usd_per_1m, cache_input_usd_per_1m, cache_output_usd_per_1m,
  priority_pricing_enabled, priority_input_usd_per_1m, priority_output_usd_per_1m, priority_cache_input_usd_per_1m,
  high_context_pricing_json,
  status, created_at
) VALUES(
  ?, ?, ?,
  ?, ?, ?, ?,
  ?, ?, ?, ?,
  ?,
  ?, CURRENT_TIMESTAMP
)
`, in.PublicID, in.GroupName, in.OwnedBy, inUSD, outUSD, cacheInUSD, cacheOutUSD,
		priorityEnabled, priorityInUSD, priorityOutUSD, priorityCacheInUSD,
		highContextPricingJSON,
		in.Status)
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
	in.GroupName = normalizeManagedModelGroupName(in.GroupName)
	inUSD := in.InputUSDPer1M.Truncate(USDScale)
	outUSD := in.OutputUSDPer1M.Truncate(USDScale)
	cacheInUSD := in.CacheInputUSDPer1M.Truncate(USDScale)
	cacheOutUSD := in.CacheOutputUSDPer1M.Truncate(USDScale)
	priorityInUSD, err := normalizeOptionalManagedModelPrice(in.PriorityInputUSDPer1M)
	if err != nil {
		return err
	}
	priorityOutUSD, err := normalizeOptionalManagedModelPrice(in.PriorityOutputUSDPer1M)
	if err != nil {
		return err
	}
	priorityCacheInUSD, err := normalizeOptionalManagedModelPrice(in.PriorityCacheInputUSDPer1M)
	if err != nil {
		return err
	}
	highContextPricingJSON, err := marshalManagedModelHighContextPricing(in.HighContextPricing)
	if err != nil {
		return err
	}
	if inUSD.IsNegative() || outUSD.IsNegative() || cacheInUSD.IsNegative() || cacheOutUSD.IsNegative() {
		return errors.New("模型定价不合法")
	}
	if err := validateManagedModelPriorityPricing(in.PriorityPricingEnabled, priorityInUSD, priorityOutUSD); err != nil {
		return err
	}
	priorityEnabled := 0
	if in.PriorityPricingEnabled {
		priorityEnabled = 1
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
SET public_id=?, group_name=?, owned_by=?,
    input_usd_per_1m=?, output_usd_per_1m=?, cache_input_usd_per_1m=?, cache_output_usd_per_1m=?,
    priority_pricing_enabled=?, priority_input_usd_per_1m=?, priority_output_usd_per_1m=?, priority_cache_input_usd_per_1m=?,
    high_context_pricing_json=?,
    status=?
WHERE id=?
`, in.PublicID, in.GroupName, in.OwnedBy, inUSD, outUSD, cacheInUSD, cacheOutUSD,
		priorityEnabled, priorityInUSD, priorityOutUSD, priorityCacheInUSD,
		highContextPricingJSON,
		in.Status, in.ID); err != nil {
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var publicID string
	if err := tx.QueryRowContext(ctx, `SELECT public_id FROM managed_models WHERE id=?`, id).Scan(&publicID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("查询 managed_model public_id 失败: %w", err)
	}
	publicID = strings.TrimSpace(publicID)

	// 删除模型目录条目时，联动清理所有渠道绑定（channel_models），避免旧渠道仍残留不可选模型。
	if publicID != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM channel_models WHERE public_id=?`, publicID); err != nil {
			return fmt.Errorf("联动删除 channel_models 失败: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM managed_models WHERE id=?`, id); err != nil {
		return fmt.Errorf("删除 managed_model 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

type ManagedModelPricingUpsert struct {
	PublicID                   string
	BasePricingSpecified       bool
	InputUSDPer1M              decimal.Decimal
	OutputUSDPer1M             decimal.Decimal
	CacheInputUSDPer1M         decimal.Decimal
	CacheOutputUSDPer1M        decimal.Decimal
	PriorityPricingEnabled     *bool
	PriorityInputUSDPer1M      *decimal.Decimal
	PriorityOutputUSDPer1M     *decimal.Decimal
	PriorityCacheInputUSDPer1M *decimal.Decimal
	HighContextPricingSpecified bool
	HighContextPricing          *ManagedModelHighContextPricing
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

	type normalizedPricingUpsert struct {
		PublicID                   string
		BasePricingSpecified       bool
		InputUSDPer1M              decimal.Decimal
		OutputUSDPer1M             decimal.Decimal
		CacheInputUSDPer1M         decimal.Decimal
		CacheOutputUSDPer1M        decimal.Decimal
		PriorityPricingEnabled     *bool
		PriorityInputUSDPer1M      *decimal.Decimal
		PriorityOutputUSDPer1M     *decimal.Decimal
		PriorityCacheInputUSDPer1M *decimal.Decimal
		HighContextPricingSpecified bool
		HighContextPricing          *ManagedModelHighContextPricing
	}

	byPublicID := make(map[string]normalizedPricingUpsert, len(items))
	for _, it := range items {
		id := strings.TrimSpace(it.PublicID)
		if id == "" {
			return UpsertManagedModelPricingResult{}, errors.New("public_id 不能为空")
		}
		inUSD := it.InputUSDPer1M.Truncate(USDScale)
		outUSD := it.OutputUSDPer1M.Truncate(USDScale)
		cacheInUSD := it.CacheInputUSDPer1M.Truncate(USDScale)
		cacheOutUSD := it.CacheOutputUSDPer1M.Truncate(USDScale)
		if inUSD.IsNegative() || outUSD.IsNegative() || cacheInUSD.IsNegative() || cacheOutUSD.IsNegative() {
			return UpsertManagedModelPricingResult{}, errors.New("模型定价不合法")
		}
		priorityInUSD, err := normalizeOptionalManagedModelPrice(it.PriorityInputUSDPer1M)
		if err != nil {
			return UpsertManagedModelPricingResult{}, err
		}
		priorityOutUSD, err := normalizeOptionalManagedModelPrice(it.PriorityOutputUSDPer1M)
		if err != nil {
			return UpsertManagedModelPricingResult{}, err
		}
		priorityCacheInUSD, err := normalizeOptionalManagedModelPrice(it.PriorityCacheInputUSDPer1M)
		if err != nil {
			return UpsertManagedModelPricingResult{}, err
		}
		highContextPricing, err := normalizeManagedModelHighContextPricing(it.HighContextPricing)
		if err != nil {
			return UpsertManagedModelPricingResult{}, err
		}
		byPublicID[id] = normalizedPricingUpsert{
			PublicID:                   id,
			BasePricingSpecified:       it.BasePricingSpecified,
			InputUSDPer1M:              inUSD,
			OutputUSDPer1M:             outUSD,
			CacheInputUSDPer1M:         cacheInUSD,
			CacheOutputUSDPer1M:        cacheOutUSD,
			PriorityPricingEnabled:     it.PriorityPricingEnabled,
			PriorityInputUSDPer1M:      priorityInUSD,
			PriorityOutputUSDPer1M:     priorityOutUSD,
			PriorityCacheInputUSDPer1M: priorityCacheInUSD,
			HighContextPricingSpecified: it.HighContextPricingSpecified,
			HighContextPricing:          highContextPricing,
		}
	}

	publicIDs := make([]string, 0, len(byPublicID))
	for id := range byPublicID {
		publicIDs = append(publicIDs, id)
	}
	sort.Strings(publicIDs)

	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(managedModelSelectColumns)
	b.WriteString("\n")
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

	existingByPublicID := make(map[string]ManagedModel, len(publicIDs))
	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return UpsertManagedModelPricingResult{}, fmt.Errorf("查询 managed_models 失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		m, err := scanManagedModelRow(rows)
		if err != nil {
			return UpsertManagedModelPricingResult{}, fmt.Errorf("扫描 managed_models 失败: %w", err)
		}
		pid := strings.TrimSpace(m.PublicID)
		if pid == "" || m.ID == 0 {
			continue
		}
		existingByPublicID[pid] = m
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
SET input_usd_per_1m=?, output_usd_per_1m=?, cache_input_usd_per_1m=?, cache_output_usd_per_1m=?,
    priority_pricing_enabled=?, priority_input_usd_per_1m=?, priority_output_usd_per_1m=?, priority_cache_input_usd_per_1m=?,
    high_context_pricing_json=?
WHERE id=?
`)
	if err != nil {
		return UpsertManagedModelPricingResult{}, fmt.Errorf("准备更新 managed_models 失败: %w", err)
	}
	defer updateStmt.Close()

	insertStmt, err := tx.PrepareContext(ctx, `
INSERT INTO managed_models(
  public_id, group_name, owned_by,
  input_usd_per_1m, output_usd_per_1m, cache_input_usd_per_1m, cache_output_usd_per_1m,
  priority_pricing_enabled, priority_input_usd_per_1m, priority_output_usd_per_1m, priority_cache_input_usd_per_1m,
  high_context_pricing_json,
  status, created_at
) VALUES(
  ?, ?, NULL,
  ?, ?, ?, ?,
  ?, ?, ?, ?,
  ?,
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
			if !it.BasePricingSpecified {
				it.InputUSDPer1M = e.InputUSDPer1M.Truncate(USDScale)
				it.OutputUSDPer1M = e.OutputUSDPer1M.Truncate(USDScale)
				it.CacheInputUSDPer1M = e.CacheInputUSDPer1M.Truncate(USDScale)
				it.CacheOutputUSDPer1M = e.CacheOutputUSDPer1M.Truncate(USDScale)
			}
			priorityEnabled := e.PriorityPricingEnabled
			if it.PriorityPricingEnabled != nil {
				priorityEnabled = *it.PriorityPricingEnabled
			}
			priorityInUSD := e.PriorityInputUSDPer1M
			if it.PriorityInputUSDPer1M != nil {
				priorityInUSD = it.PriorityInputUSDPer1M
			}
			priorityOutUSD := e.PriorityOutputUSDPer1M
			if it.PriorityOutputUSDPer1M != nil {
				priorityOutUSD = it.PriorityOutputUSDPer1M
			}
			priorityCacheInUSD := e.PriorityCacheInputUSDPer1M
			if it.PriorityCacheInputUSDPer1M != nil {
				priorityCacheInUSD = it.PriorityCacheInputUSDPer1M
			}
			highContextPricing := e.HighContextPricing
			if it.HighContextPricingSpecified {
				highContextPricing = it.HighContextPricing
			}
			if err := validateManagedModelPriorityPricing(priorityEnabled, priorityInUSD, priorityOutUSD); err != nil {
				return UpsertManagedModelPricingResult{}, err
			}
			highContextPricingJSON, err := marshalManagedModelHighContextPricing(highContextPricing)
			if err != nil {
				return UpsertManagedModelPricingResult{}, err
			}
			if e.InputUSDPer1M.Truncate(USDScale).Equal(it.InputUSDPer1M) &&
				e.OutputUSDPer1M.Truncate(USDScale).Equal(it.OutputUSDPer1M) &&
				e.CacheInputUSDPer1M.Truncate(USDScale).Equal(it.CacheInputUSDPer1M) &&
				e.CacheOutputUSDPer1M.Truncate(USDScale).Equal(it.CacheOutputUSDPer1M) &&
				e.PriorityPricingEnabled == priorityEnabled &&
				equalOptionalDecimal(e.PriorityInputUSDPer1M, priorityInUSD) &&
				equalOptionalDecimal(e.PriorityOutputUSDPer1M, priorityOutUSD) &&
				equalOptionalDecimal(e.PriorityCacheInputUSDPer1M, priorityCacheInUSD) &&
				equalManagedModelHighContextPricing(e.HighContextPricing, highContextPricing) {
				res.Unchanged = append(res.Unchanged, publicID)
				continue
			}
			priorityFlag := 0
			if priorityEnabled {
				priorityFlag = 1
			}
			if _, err := updateStmt.ExecContext(ctx,
				it.InputUSDPer1M, it.OutputUSDPer1M, it.CacheInputUSDPer1M, it.CacheOutputUSDPer1M,
				priorityFlag, priorityInUSD, priorityOutUSD, priorityCacheInUSD,
				highContextPricingJSON,
				e.ID,
			); err != nil {
				return UpsertManagedModelPricingResult{}, fmt.Errorf("更新 managed_model(%s) 失败: %w", publicID, err)
			}
			res.Updated = append(res.Updated, publicID)
			continue
		}

		if !it.BasePricingSpecified {
			return UpsertManagedModelPricingResult{}, errors.New("模型基础定价不合法")
		}
		priorityEnabled := false
		if it.PriorityPricingEnabled != nil {
			priorityEnabled = *it.PriorityPricingEnabled
		}
		if err := validateManagedModelPriorityPricing(priorityEnabled, it.PriorityInputUSDPer1M, it.PriorityOutputUSDPer1M); err != nil {
			return UpsertManagedModelPricingResult{}, err
		}
		highContextPricingJSON, err := marshalManagedModelHighContextPricing(it.HighContextPricing)
		if err != nil {
			return UpsertManagedModelPricingResult{}, err
		}
		priorityFlag := 0
		if priorityEnabled {
			priorityFlag = 1
		}
		if _, err := insertStmt.ExecContext(ctx,
			publicID, "",
			it.InputUSDPer1M, it.OutputUSDPer1M, it.CacheInputUSDPer1M, it.CacheOutputUSDPer1M,
			priorityFlag, it.PriorityInputUSDPer1M, it.PriorityOutputUSDPer1M, it.PriorityCacheInputUSDPer1M,
			highContextPricingJSON,
		); err != nil {
			return UpsertManagedModelPricingResult{}, fmt.Errorf("创建 managed_model(%s) 失败: %w", publicID, err)
		}
		res.Added = append(res.Added, publicID)
	}

	if err := tx.Commit(); err != nil {
		return UpsertManagedModelPricingResult{}, fmt.Errorf("提交事务失败: %w", err)
	}
	return res, nil
}
