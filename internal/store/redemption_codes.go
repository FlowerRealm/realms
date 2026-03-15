package store

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	SubscriptionActivationModeImmediate = "immediate"
	SubscriptionActivationModeDeferred  = "deferred"
)

var (
	ErrRedemptionCodeNotFound             = errors.New("兑换码不存在")
	ErrRedemptionCodeInactive             = errors.New("兑换码不可用")
	ErrRedemptionCodeExpired              = errors.New("兑换码已过期")
	ErrRedemptionCodeExhausted            = errors.New("兑换码已兑完")
	ErrRedemptionCodeAlreadyRedeemed      = errors.New("你已兑换过该兑换码")
	ErrSubscriptionActivationModeRequired = errors.New("需要选择套餐生效方式")
	ErrSubscriptionActivationModeInvalid  = errors.New("subscription_activation_mode 不合法")
	ErrRedemptionCodeInvalidReward        = errors.New("兑换码奖励配置不合法")
	ErrRedemptionCodeImmutable            = errors.New("兑换码奖励内容不可修改")
	ErrRedemptionCodeDuplicate            = errors.New("兑换码已存在")
	ErrRedemptionCodeRewardMismatch       = errors.New("兑换码类型不匹配")
)

type RedemptionCodeCreate struct {
	BatchName          string
	Code               string
	DistributionMode   RedemptionCodeDistributionMode
	RewardType         RedemptionCodeRewardType
	SubscriptionPlanID *int64
	BalanceUSD         decimal.Decimal
	MaxRedemptions     int
	ExpiresAt          *time.Time
	Status             RedemptionCodeStatus
	CreatedBy          int64
}

type RedemptionCodeUpdate struct {
	ID             int64
	MaxRedemptions int
	ExpiresAt      *time.Time
	Status         RedemptionCodeStatus
}

type RedemptionCodeListFilter struct {
	Keyword          string
	BatchName        string
	DistributionMode RedemptionCodeDistributionMode
	RewardType       RedemptionCodeRewardType
	Status           *RedemptionCodeStatus
	Exhausted        *bool
	Limit            int
}

type RedemptionCodeListItem struct {
	Code RedemptionCode
	Plan *SubscriptionPlan
}

type RedeemCodeInput struct {
	UserID                     int64
	Code                       string
	ExpectedRewardType         RedemptionCodeRewardType
	SubscriptionActivationMode *string
	Now                        time.Time
}

type RedeemCodeResult struct {
	Code                       RedemptionCode
	RewardType                 RedemptionCodeRewardType
	BalanceUSD                 decimal.Decimal
	NewBalanceUSD              decimal.Decimal
	Plan                       *SubscriptionPlan
	Subscription               *UserSubscription
	SubscriptionActivationMode *string
}

func normalizeRedemptionCodeValue(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}

func normalizeRedemptionCodeDistributionMode(raw RedemptionCodeDistributionMode) RedemptionCodeDistributionMode {
	switch strings.TrimSpace(string(raw)) {
	case string(RedemptionCodeDistributionSingle):
		return RedemptionCodeDistributionSingle
	case string(RedemptionCodeDistributionShared):
		return RedemptionCodeDistributionShared
	default:
		return ""
	}
}

func normalizeRedemptionCodeRewardType(raw RedemptionCodeRewardType) RedemptionCodeRewardType {
	switch strings.TrimSpace(string(raw)) {
	case string(RedemptionCodeRewardBalance):
		return RedemptionCodeRewardBalance
	case string(RedemptionCodeRewardSubscription):
		return RedemptionCodeRewardSubscription
	default:
		return ""
	}
}

func parseSubscriptionActivationMode(raw *string) (string, bool, error) {
	if raw == nil {
		return "", false, nil
	}
	normalized := strings.TrimSpace(*raw)
	if normalized == "" {
		return "", false, nil
	}
	switch normalized {
	case SubscriptionActivationModeImmediate:
		return SubscriptionActivationModeImmediate, true, nil
	case SubscriptionActivationModeDeferred:
		return SubscriptionActivationModeDeferred, true, nil
	default:
		return "", false, ErrSubscriptionActivationModeInvalid
	}
}

func truncateRedemptionCodeMoney(code *RedemptionCode) {
	code.BalanceUSD = code.BalanceUSD.Truncate(USDScale)
}

func mapRedemptionRecordInsertError(err error) error {
	if err == nil {
		return nil
	}
	if isUniqueConstraintError(err) {
		return ErrRedemptionCodeAlreadyRedeemed
	}
	return fmt.Errorf("写入兑换记录失败: %w", err)
}

func hasRedemptionRecordForUserTx(ctx context.Context, tx *sql.Tx, codeID, userID int64) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, `
SELECT 1
FROM redemption_code_redemptions
WHERE code_id=? AND user_id=?
LIMIT 1
`, codeID, userID).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("查询兑换记录失败: %w", err)
}

func reserveRedemptionSlotTx(ctx context.Context, tx *sql.Tx, code RedemptionCode, userID int64) (RedemptionCode, error) {
	res, err := tx.ExecContext(ctx, `
UPDATE redemption_codes
SET redeemed_count=redeemed_count+1, updated_at=CURRENT_TIMESTAMP
WHERE id=? AND redeemed_count < max_redemptions
`, code.ID)
	if err != nil {
		return code, fmt.Errorf("抢占兑换名额失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return code, fmt.Errorf("读取兑换名额结果失败: %w", err)
	}
	if affected == 0 {
		if code.DistributionMode == RedemptionCodeDistributionShared {
			exists, err := hasRedemptionRecordForUserTx(ctx, tx, code.ID, userID)
			if err != nil {
				return code, err
			}
			if exists {
				return code, ErrRedemptionCodeAlreadyRedeemed
			}
		}
		return code, ErrRedemptionCodeExhausted
	}
	code.RedeemedCount++
	return code, nil
}

func validateRedemptionCodeCreateInput(in *RedemptionCodeCreate) error {
	if in == nil {
		return errors.New("参数不能为空")
	}
	in.BatchName = strings.TrimSpace(in.BatchName)
	if in.BatchName == "" {
		return errors.New("批次名不能为空")
	}
	in.Code = normalizeRedemptionCodeValue(in.Code)
	if in.Code == "" {
		return errors.New("兑换码不能为空")
	}
	in.DistributionMode = normalizeRedemptionCodeDistributionMode(in.DistributionMode)
	if in.DistributionMode == "" {
		return errors.New("发码模式不合法")
	}
	in.RewardType = normalizeRedemptionCodeRewardType(in.RewardType)
	if in.RewardType == "" {
		return errors.New("奖励类型不合法")
	}
	if in.CreatedBy < 0 {
		return errors.New("created_by 不合法")
	}
	if in.Status != RedemptionCodeStatusActive && in.Status != RedemptionCodeStatusDisabled {
		return errors.New("status 不合法")
	}
	if in.DistributionMode == RedemptionCodeDistributionSingle {
		in.MaxRedemptions = 1
	}
	if in.MaxRedemptions <= 0 {
		return errors.New("可兑换次数必须大于 0")
	}
	switch in.RewardType {
	case RedemptionCodeRewardBalance:
		in.BalanceUSD = in.BalanceUSD.Truncate(USDScale)
		if in.BalanceUSD.LessThanOrEqual(decimal.Zero) {
			return errors.New("余额奖励必须大于 0")
		}
		in.SubscriptionPlanID = nil
	case RedemptionCodeRewardSubscription:
		if in.SubscriptionPlanID == nil || *in.SubscriptionPlanID <= 0 {
			return errors.New("套餐不能为空")
		}
		in.BalanceUSD = decimal.Zero
	default:
		return ErrRedemptionCodeInvalidReward
	}
	return nil
}

func scanRedemptionCode(scanner interface{ Scan(dest ...any) error }, withPlan bool) (RedemptionCodeListItem, error) {
	var item RedemptionCodeListItem
	var exp sql.NullTime
	var codePlanID sql.NullInt64
	var itemPlanID sql.NullInt64
	var planCode sql.NullString
	var planName sql.NullString
	var planGroup sql.NullString
	var planPriceMultiplier decimal.NullDecimal
	var planPriceCNY decimal.NullDecimal
	var planLimit5H decimal.NullDecimal
	var planLimit1D decimal.NullDecimal
	var planLimit7D decimal.NullDecimal
	var planLimit30D decimal.NullDecimal
	var planDuration sql.NullInt64
	var planStatus sql.NullInt64
	var planCreated sql.NullTime
	var planUpdated sql.NullTime

	dest := []any{
		&item.Code.ID,
		&item.Code.BatchName,
		&item.Code.Code,
		&item.Code.DistributionMode,
		&item.Code.RewardType,
		&codePlanID,
		&item.Code.BalanceUSD,
		&item.Code.MaxRedemptions,
		&item.Code.RedeemedCount,
		&exp,
		&item.Code.Status,
		&item.Code.CreatedBy,
		&item.Code.CreatedAt,
		&item.Code.UpdatedAt,
	}
	if withPlan {
		dest = append(dest,
			&itemPlanID,
			&planCode,
			&planName,
			&planGroup,
			&planPriceMultiplier,
			&planPriceCNY,
			&planLimit5H,
			&planLimit1D,
			&planLimit7D,
			&planLimit30D,
			&planDuration,
			&planStatus,
			&planCreated,
			&planUpdated,
		)
	}
	if err := scanner.Scan(dest...); err != nil {
		return RedemptionCodeListItem{}, err
	}
	if codePlanID.Valid && codePlanID.Int64 > 0 {
		v := codePlanID.Int64
		item.Code.SubscriptionPlanID = &v
	}
	if exp.Valid {
		t := exp.Time
		item.Code.ExpiresAt = &t
	}
	truncateRedemptionCodeMoney(&item.Code)
	if withPlan && itemPlanID.Valid && itemPlanID.Int64 > 0 {
		p := SubscriptionPlan{
			ID:           itemPlanID.Int64,
			Code:         strings.TrimSpace(planCode.String),
			Name:         strings.TrimSpace(planName.String),
			GroupName:    strings.TrimSpace(planGroup.String),
			DurationDays: int(planDuration.Int64),
			Status:       int(planStatus.Int64),
		}
		if planPriceMultiplier.Valid {
			p.PriceMultiplier = planPriceMultiplier.Decimal
		}
		if planPriceCNY.Valid {
			p.PriceCNY = planPriceCNY.Decimal
		}
		if planLimit5H.Valid {
			p.Limit5HUSD = planLimit5H.Decimal
		}
		if planLimit1D.Valid {
			p.Limit1DUSD = planLimit1D.Decimal
		}
		if planLimit7D.Valid {
			p.Limit7DUSD = planLimit7D.Decimal
		}
		if planLimit30D.Valid {
			p.Limit30DUSD = planLimit30D.Decimal
		}
		if planCreated.Valid {
			p.CreatedAt = planCreated.Time
		}
		if planUpdated.Valid {
			p.UpdatedAt = planUpdated.Time
		}
		truncateSubscriptionPlanMoney(&p)
		item.Plan = &p
	}
	return item, nil
}

func (s *Store) CreateRedemptionCode(ctx context.Context, in RedemptionCodeCreate) (int64, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("store 未初始化")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, err := s.createRedemptionCodeTx(ctx, tx, in)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交事务失败: %w", err)
	}
	return id, nil
}

func (s *Store) CreateRedemptionCodes(ctx context.Context, items []RedemptionCodeCreate) ([]int64, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store 未初始化")
	}
	if len(items) == 0 {
		return nil, errors.New("兑换码不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ids := make([]int64, 0, len(items))
	for _, item := range items {
		id, err := s.createRedemptionCodeTx(ctx, tx, item)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交事务失败: %w", err)
	}
	return ids, nil
}

func (s *Store) createRedemptionCodeTx(ctx context.Context, tx *sql.Tx, in RedemptionCodeCreate) (int64, error) {
	if err := validateRedemptionCodeCreateInput(&in); err != nil {
		return 0, err
	}
	if in.RewardType == RedemptionCodeRewardSubscription {
		if _, err := getSubscriptionPlanByIDTx(ctx, tx, *in.SubscriptionPlanID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, errors.New("套餐不存在")
			}
			return 0, err
		}
	}
	res, err := tx.ExecContext(ctx, `
INSERT INTO redemption_codes(
  batch_name, code, distribution_mode, reward_type, subscription_plan_id, balance_usd,
  max_redemptions, redeemed_count, expires_at, status, created_by, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, in.BatchName, in.Code, string(in.DistributionMode), string(in.RewardType), in.SubscriptionPlanID, in.BalanceUSD, in.MaxRedemptions, in.ExpiresAt, int(in.Status), in.CreatedBy)
	if err != nil {
		if isUniqueConstraintError(err) {
			return 0, ErrRedemptionCodeDuplicate
		}
		return 0, fmt.Errorf("创建兑换码失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取兑换码 id 失败: %w", err)
	}
	return id, nil
}

func (s *Store) GetRedemptionCodeByID(ctx context.Context, id int64) (RedemptionCodeListItem, error) {
	if s == nil || s.db == nil {
		return RedemptionCodeListItem{}, errors.New("store 未初始化")
	}
	if id <= 0 {
		return RedemptionCodeListItem{}, errors.New("id 不合法")
	}
	row := s.db.QueryRowContext(ctx, `
SELECT
  rc.id, rc.batch_name, rc.code, rc.distribution_mode, rc.reward_type, rc.subscription_plan_id, rc.balance_usd,
  rc.max_redemptions, rc.redeemed_count, rc.expires_at, rc.status, rc.created_by, rc.created_at, rc.updated_at,
  sp.id, sp.code, sp.name, sp.group_name, sp.price_multiplier, sp.price_cny,
  sp.limit_5h_usd, sp.limit_1d_usd, sp.limit_7d_usd, sp.limit_30d_usd,
  sp.duration_days, sp.status, sp.created_at, sp.updated_at
FROM redemption_codes rc
LEFT JOIN subscription_plans sp ON sp.id=rc.subscription_plan_id
WHERE rc.id=?
LIMIT 1
`, id)
	item, err := scanRedemptionCode(row, true)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RedemptionCodeListItem{}, sql.ErrNoRows
		}
		return RedemptionCodeListItem{}, fmt.Errorf("查询兑换码失败: %w", err)
	}
	return item, nil
}

func (s *Store) ListRedemptionCodes(ctx context.Context, filter RedemptionCodeListFilter) ([]RedemptionCodeListItem, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store 未初始化")
	}
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 200
	}
	var b strings.Builder
	args := make([]any, 0, 8)
	b.WriteString(`
SELECT
  rc.id, rc.batch_name, rc.code, rc.distribution_mode, rc.reward_type, rc.subscription_plan_id, rc.balance_usd,
  rc.max_redemptions, rc.redeemed_count, rc.expires_at, rc.status, rc.created_by, rc.created_at, rc.updated_at,
  sp.id, sp.code, sp.name, sp.group_name, sp.price_multiplier, sp.price_cny,
  sp.limit_5h_usd, sp.limit_1d_usd, sp.limit_7d_usd, sp.limit_30d_usd,
  sp.duration_days, sp.status, sp.created_at, sp.updated_at
FROM redemption_codes rc
LEFT JOIN subscription_plans sp ON sp.id=rc.subscription_plan_id
WHERE 1=1
`)
	if q := strings.TrimSpace(filter.Keyword); q != "" {
		p := buildLikePattern(normalizeRedemptionCodeValue(q))
		b.WriteString(` AND (rc.code LIKE ? OR rc.batch_name LIKE ?)`)
		args = append(args, p, buildLikePattern(strings.TrimSpace(filter.Keyword)))
	}
	if batch := strings.TrimSpace(filter.BatchName); batch != "" {
		b.WriteString(` AND rc.batch_name=?`)
		args = append(args, batch)
	}
	if mode := normalizeRedemptionCodeDistributionMode(filter.DistributionMode); mode != "" {
		b.WriteString(` AND rc.distribution_mode=?`)
		args = append(args, string(mode))
	}
	if rewardType := normalizeRedemptionCodeRewardType(filter.RewardType); rewardType != "" {
		b.WriteString(` AND rc.reward_type=?`)
		args = append(args, string(rewardType))
	}
	if filter.Status != nil {
		b.WriteString(` AND rc.status=?`)
		args = append(args, int(*filter.Status))
	}
	if filter.Exhausted != nil {
		if *filter.Exhausted {
			b.WriteString(` AND rc.redeemed_count >= rc.max_redemptions`)
		} else {
			b.WriteString(` AND rc.redeemed_count < rc.max_redemptions`)
		}
	}
	b.WriteString(` ORDER BY rc.id DESC LIMIT ?`)
	args = append(args, filter.Limit)

	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("查询兑换码列表失败: %w", err)
	}
	defer rows.Close()

	var out []RedemptionCodeListItem
	for rows.Next() {
		item, err := scanRedemptionCode(rows, true)
		if err != nil {
			return nil, fmt.Errorf("扫描兑换码失败: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历兑换码失败: %w", err)
	}
	return out, nil
}

func (s *Store) UpdateSharedRedemptionCode(ctx context.Context, in RedemptionCodeUpdate) error {
	if s == nil || s.db == nil {
		return errors.New("store 未初始化")
	}
	if in.ID <= 0 {
		return errors.New("id 不合法")
	}
	if in.MaxRedemptions <= 0 {
		return errors.New("可兑换次数必须大于 0")
	}
	if in.Status != RedemptionCodeStatusActive && in.Status != RedemptionCodeStatusDisabled {
		return errors.New("status 不合法")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var mode string
	var redeemed int
	q := `
SELECT distribution_mode, redeemed_count
FROM redemption_codes
WHERE id=?
` + forUpdateClause(s.dialect)
	if err := tx.QueryRowContext(ctx, q, in.ID).Scan(&mode, &redeemed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("查询兑换码失败: %w", err)
	}
	if normalizeRedemptionCodeDistributionMode(RedemptionCodeDistributionMode(mode)) != RedemptionCodeDistributionShared {
		return ErrRedemptionCodeImmutable
	}
	if in.MaxRedemptions < redeemed {
		return errors.New("可兑换次数不能小于已兑换次数")
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE redemption_codes
SET max_redemptions=?, expires_at=?, status=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, in.MaxRedemptions, in.ExpiresAt, int(in.Status), in.ID); err != nil {
		return fmt.Errorf("更新兑换码失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) DisableRedemptionCode(ctx context.Context, id int64) error {
	if s == nil || s.db == nil {
		return errors.New("store 未初始化")
	}
	if id <= 0 {
		return errors.New("id 不合法")
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE redemption_codes
SET status=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, int(RedemptionCodeStatusDisabled), id)
	if err != nil {
		return fmt.Errorf("停用兑换码失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ExportRedemptionCodesCSV(ctx context.Context, w io.Writer, filter RedemptionCodeListFilter) error {
	items, err := s.ListRedemptionCodes(ctx, filter)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{"code", "batch_name", "distribution_mode", "reward_type", "reward_value", "status", "redeemed_count", "max_redemptions", "expires_at", "created_at"}); err != nil {
		return err
	}
	for _, item := range items {
		rewardValue := ""
		switch item.Code.RewardType {
		case RedemptionCodeRewardBalance:
			rewardValue = item.Code.BalanceUSD.StringFixed(USDScale)
		case RedemptionCodeRewardSubscription:
			if item.Plan != nil {
				rewardValue = item.Plan.Name
			}
		}
		exp := ""
		if item.Code.ExpiresAt != nil {
			exp = item.Code.ExpiresAt.Format("2006-01-02 15:04:05")
		}
		status := "disabled"
		if item.Code.Status == RedemptionCodeStatusActive {
			status = "active"
		}
		if err := cw.Write([]string{
			item.Code.Code,
			item.Code.BatchName,
			string(item.Code.DistributionMode),
			string(item.Code.RewardType),
			rewardValue,
			status,
			fmt.Sprintf("%d", item.Code.RedeemedCount),
			fmt.Sprintf("%d", item.Code.MaxRedemptions),
			exp,
			item.Code.CreatedAt.Format("2006-01-02 15:04:05"),
		}); err != nil {
			return err
		}
	}
	return cw.Error()
}

func (s *Store) RedeemCode(ctx context.Context, in RedeemCodeInput) (RedeemCodeResult, error) {
	if s == nil || s.db == nil {
		return RedeemCodeResult{}, errors.New("store 未初始化")
	}
	if in.UserID <= 0 {
		return RedeemCodeResult{}, errors.New("user_id 不合法")
	}
	codeValue := normalizeRedemptionCodeValue(in.Code)
	if codeValue == "" {
		return RedeemCodeResult{}, errors.New("兑换码不能为空")
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	mode, hasMode, err := parseSubscriptionActivationMode(in.SubscriptionActivationMode)
	if err != nil {
		return RedeemCodeResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RedeemCodeResult{}, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
SELECT id, batch_name, code, distribution_mode, reward_type, subscription_plan_id, balance_usd,
       max_redemptions, redeemed_count, expires_at, status, created_by, created_at, updated_at
FROM redemption_codes
WHERE code=?
`+forUpdateClause(s.dialect), codeValue)
	item, err := scanRedemptionCode(row, false)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RedeemCodeResult{}, ErrRedemptionCodeNotFound
		}
		return RedeemCodeResult{}, fmt.Errorf("查询兑换码失败: %w", err)
	}
	code := item.Code
	if code.Status != RedemptionCodeStatusActive {
		return RedeemCodeResult{}, ErrRedemptionCodeInactive
	}
	if expected := normalizeRedemptionCodeRewardType(in.ExpectedRewardType); expected != "" && code.RewardType != expected {
		return RedeemCodeResult{}, ErrRedemptionCodeRewardMismatch
	}
	if code.ExpiresAt != nil && !code.ExpiresAt.After(now) {
		return RedeemCodeResult{}, ErrRedemptionCodeExpired
	}
	if code.DistributionMode == RedemptionCodeDistributionShared {
		exists, err := hasRedemptionRecordForUserTx(ctx, tx, code.ID, in.UserID)
		if err != nil {
			return RedeemCodeResult{}, err
		}
		if exists {
			return RedeemCodeResult{}, ErrRedemptionCodeAlreadyRedeemed
		}
	}
	if code.RedeemedCount >= code.MaxRedemptions {
		return RedeemCodeResult{}, ErrRedemptionCodeExhausted
	}
	code, err = reserveRedemptionSlotTx(ctx, tx, code, in.UserID)
	if err != nil {
		return RedeemCodeResult{}, err
	}

	result := RedeemCodeResult{
		Code:       code,
		RewardType: code.RewardType,
	}
	switch code.RewardType {
	case RedemptionCodeRewardBalance:
		if code.BalanceUSD.LessThanOrEqual(decimal.Zero) {
			return RedeemCodeResult{}, ErrRedemptionCodeInvalidReward
		}
		balance, err := addUserBalanceUSDTx(ctx, tx, s.dialect, in.UserID, code.BalanceUSD)
		if err != nil {
			return RedeemCodeResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO redemption_code_redemptions(code_id, user_id, reward_type, balance_usd, subscription_id, subscription_activation_mode, created_at)
VALUES(?, ?, ?, ?, NULL, NULL, CURRENT_TIMESTAMP)
`, code.ID, in.UserID, string(code.RewardType), code.BalanceUSD); err != nil {
			return RedeemCodeResult{}, mapRedemptionRecordInsertError(err)
		}
		result.BalanceUSD = code.BalanceUSD
		result.NewBalanceUSD = balance
	case RedemptionCodeRewardSubscription:
		if code.SubscriptionPlanID == nil || *code.SubscriptionPlanID <= 0 {
			return RedeemCodeResult{}, ErrRedemptionCodeInvalidReward
		}
		plan, err := getSubscriptionPlanByIDTx(ctx, tx, *code.SubscriptionPlanID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return RedeemCodeResult{}, errors.New("套餐不存在")
			}
			return RedeemCodeResult{}, err
		}
		if plan.Status != 1 {
			return RedeemCodeResult{}, errors.New("套餐不可用")
		}
		grantMode := SubscriptionActivationModeImmediate
		samePlanActive, err := hasNonExpiredSubscriptionForPlanTx(ctx, tx, in.UserID, plan.ID, now)
		if err != nil {
			return RedeemCodeResult{}, err
		}
		if samePlanActive {
			if !hasMode {
				return RedeemCodeResult{}, ErrSubscriptionActivationModeRequired
			}
			grantMode = mode
		}
		sub, err := grantSubscriptionByPlanTx(ctx, tx, in.UserID, plan, now, grantMode)
		if err != nil {
			return RedeemCodeResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO redemption_code_redemptions(code_id, user_id, reward_type, balance_usd, subscription_id, subscription_activation_mode, created_at)
VALUES(?, ?, ?, 0, ?, ?, CURRENT_TIMESTAMP)
`, code.ID, in.UserID, string(code.RewardType), sub.ID, grantMode); err != nil {
			return RedeemCodeResult{}, mapRedemptionRecordInsertError(err)
		}
		result.Plan = &plan
		result.Subscription = &sub
		result.SubscriptionActivationMode = &grantMode
	default:
		return RedeemCodeResult{}, ErrRedemptionCodeInvalidReward
	}

	result.Code = code

	if err := tx.Commit(); err != nil {
		return RedeemCodeResult{}, fmt.Errorf("提交事务失败: %w", err)
	}
	return result, nil
}

func addUserBalanceUSDTx(ctx context.Context, tx *sql.Tx, dialect Dialect, userID int64, deltaUSD decimal.Decimal) (decimal.Decimal, error) {
	if userID <= 0 {
		return decimal.Zero, errors.New("user_id 不能为空")
	}
	deltaUSD = deltaUSD.Truncate(USDScale)
	if deltaUSD.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, errors.New("delta_usd 不合法")
	}
	stmtInitBalance := fmt.Sprintf(`
%s INTO user_balances(user_id, usd, created_at, updated_at)
VALUES(?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, insertIgnoreVerb(dialect))
	if _, err := tx.ExecContext(ctx, stmtInitBalance, userID); err != nil {
		return decimal.Zero, fmt.Errorf("初始化余额失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, userBalancesAddSQL(dialect), deltaUSD, userID); err != nil {
		return decimal.Zero, fmt.Errorf("入账失败: %w", err)
	}
	var newBal decimal.Decimal
	if err := tx.QueryRowContext(ctx, `SELECT usd FROM user_balances WHERE user_id=?`, userID).Scan(&newBal); err != nil {
		return decimal.Zero, fmt.Errorf("查询余额失败: %w", err)
	}
	return newBal.Truncate(USDScale), nil
}
