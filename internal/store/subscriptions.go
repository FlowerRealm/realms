package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type SubscriptionPlanDeleteSummary struct {
	OrdersDeleted        int64
	SubscriptionsDeleted int64
	UsageEventsUnbound   int64
}

type SubscriptionWithPlan struct {
	Subscription UserSubscription
	Plan         SubscriptionPlan
}

func truncateSubscriptionPlanMoney(p *SubscriptionPlan) {
	p.PriceCNY = p.PriceCNY.Truncate(CNYScale)
	if p.PriceMultiplier.IsNegative() || p.PriceMultiplier.LessThanOrEqual(decimal.Zero) {
		p.PriceMultiplier = DefaultGroupPriceMultiplier
	}
	p.PriceMultiplier = p.PriceMultiplier.Truncate(PriceMultiplierScale)
	p.Limit5HUSD = p.Limit5HUSD.Truncate(USDScale)
	p.Limit1DUSD = p.Limit1DUSD.Truncate(USDScale)
	p.Limit7DUSD = p.Limit7DUSD.Truncate(USDScale)
	p.Limit30DUSD = p.Limit30DUSD.Truncate(USDScale)
}

func (s *Store) GetSubscriptionWithPlanByID(ctx context.Context, subscriptionID int64) (SubscriptionWithPlan, error) {
	if subscriptionID <= 0 {
		return SubscriptionWithPlan{}, errors.New("subscriptionID 不合法")
	}
	var row SubscriptionWithPlan
	err := s.db.QueryRowContext(ctx, `
SELECT
  us.id, us.user_id, us.plan_id, us.start_at, us.end_at, us.status, us.created_at, us.updated_at,
  sp.id, sp.code, sp.name, sp.group_name, sp.price_multiplier, sp.price_cny, sp.limit_5h_usd, sp.limit_1d_usd, sp.limit_7d_usd, sp.limit_30d_usd, sp.duration_days, sp.status, sp.created_at, sp.updated_at
FROM user_subscriptions us
JOIN subscription_plans sp ON sp.id=us.plan_id
WHERE us.id=?
LIMIT 1
`, subscriptionID).Scan(
		&row.Subscription.ID, &row.Subscription.UserID, &row.Subscription.PlanID, &row.Subscription.StartAt, &row.Subscription.EndAt, &row.Subscription.Status, &row.Subscription.CreatedAt, &row.Subscription.UpdatedAt,
		&row.Plan.ID, &row.Plan.Code, &row.Plan.Name, &row.Plan.GroupName, &row.Plan.PriceMultiplier, &row.Plan.PriceCNY, &row.Plan.Limit5HUSD, &row.Plan.Limit1DUSD, &row.Plan.Limit7DUSD, &row.Plan.Limit30DUSD, &row.Plan.DurationDays, &row.Plan.Status, &row.Plan.CreatedAt, &row.Plan.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SubscriptionWithPlan{}, sql.ErrNoRows
		}
		return SubscriptionWithPlan{}, fmt.Errorf("查询 user_subscription 失败: %w", err)
	}
	truncateSubscriptionPlanMoney(&row.Plan)
	return row, nil
}

func (s *Store) ListSubscriptionPlans(ctx context.Context) ([]SubscriptionPlan, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, code, name, group_name, price_multiplier, price_cny, limit_5h_usd, limit_1d_usd, limit_7d_usd, limit_30d_usd, duration_days, status, created_at, updated_at
FROM subscription_plans
WHERE status=1
ORDER BY id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("查询 subscription_plans 失败: %w", err)
	}
	defer rows.Close()

	var out []SubscriptionPlan
	for rows.Next() {
		var p SubscriptionPlan
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.GroupName, &p.PriceMultiplier, &p.PriceCNY, &p.Limit5HUSD, &p.Limit1DUSD, &p.Limit7DUSD, &p.Limit30DUSD, &p.DurationDays, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 subscription_plans 失败: %w", err)
		}
		truncateSubscriptionPlanMoney(&p)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 subscription_plans 失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetActiveSubscriptionWithPlan(ctx context.Context, userID int64, now time.Time) (UserSubscription, SubscriptionPlan, error) {
	var us UserSubscription
	var p SubscriptionPlan
	err := s.db.QueryRowContext(ctx, `
SELECT
  us.id, us.user_id, us.plan_id, us.start_at, us.end_at, us.status, us.created_at, us.updated_at,
  sp.id, sp.code, sp.name, sp.group_name, sp.price_multiplier, sp.price_cny, sp.limit_5h_usd, sp.limit_1d_usd, sp.limit_7d_usd, sp.limit_30d_usd, sp.duration_days, sp.status, sp.created_at, sp.updated_at
FROM user_subscriptions us
JOIN subscription_plans sp ON sp.id=us.plan_id
WHERE us.user_id=? AND us.status=1 AND us.start_at <= ? AND us.end_at > ? AND sp.status=1
ORDER BY us.end_at ASC, us.id ASC
LIMIT 1
`, userID, now, now).Scan(
		&us.ID, &us.UserID, &us.PlanID, &us.StartAt, &us.EndAt, &us.Status, &us.CreatedAt, &us.UpdatedAt,
		&p.ID, &p.Code, &p.Name, &p.GroupName, &p.PriceMultiplier, &p.PriceCNY, &p.Limit5HUSD, &p.Limit1DUSD, &p.Limit7DUSD, &p.Limit30DUSD, &p.DurationDays, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserSubscription{}, SubscriptionPlan{}, sql.ErrNoRows
		}
		return UserSubscription{}, SubscriptionPlan{}, fmt.Errorf("查询 user_subscription 失败: %w", err)
	}
	truncateSubscriptionPlanMoney(&p)
	return us, p, nil
}

func (s *Store) ListActiveSubscriptionsWithPlans(ctx context.Context, userID int64, now time.Time) ([]SubscriptionWithPlan, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  us.id, us.user_id, us.plan_id, us.start_at, us.end_at, us.status, us.created_at, us.updated_at,
  sp.id, sp.code, sp.name, sp.group_name, sp.price_multiplier, sp.price_cny, sp.limit_5h_usd, sp.limit_1d_usd, sp.limit_7d_usd, sp.limit_30d_usd, sp.duration_days, sp.status, sp.created_at, sp.updated_at
FROM user_subscriptions us
JOIN subscription_plans sp ON sp.id=us.plan_id
WHERE us.user_id=? AND us.status=1 AND us.start_at <= ? AND us.end_at > ? AND sp.status=1
ORDER BY us.end_at ASC, us.id ASC
`, userID, now, now)
	if err != nil {
		return nil, fmt.Errorf("查询 user_subscriptions 失败: %w", err)
	}
	defer rows.Close()

	var out []SubscriptionWithPlan
	for rows.Next() {
		var row SubscriptionWithPlan
		if err := rows.Scan(
			&row.Subscription.ID, &row.Subscription.UserID, &row.Subscription.PlanID, &row.Subscription.StartAt, &row.Subscription.EndAt, &row.Subscription.Status, &row.Subscription.CreatedAt, &row.Subscription.UpdatedAt,
			&row.Plan.ID, &row.Plan.Code, &row.Plan.Name, &row.Plan.GroupName, &row.Plan.PriceMultiplier, &row.Plan.PriceCNY, &row.Plan.Limit5HUSD, &row.Plan.Limit1DUSD, &row.Plan.Limit7DUSD, &row.Plan.Limit30DUSD, &row.Plan.DurationDays, &row.Plan.Status, &row.Plan.CreatedAt, &row.Plan.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描 user_subscriptions 失败: %w", err)
		}
		truncateSubscriptionPlanMoney(&row.Plan)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 user_subscriptions 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListNonExpiredSubscriptionsWithPlans(ctx context.Context, userID int64, now time.Time) ([]SubscriptionWithPlan, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  us.id, us.user_id, us.plan_id, us.start_at, us.end_at, us.status, us.created_at, us.updated_at,
  sp.id, sp.code, sp.name, sp.group_name, sp.price_multiplier, sp.price_cny, sp.limit_5h_usd, sp.limit_1d_usd, sp.limit_7d_usd, sp.limit_30d_usd, sp.duration_days, sp.status, sp.created_at, sp.updated_at
FROM user_subscriptions us
JOIN subscription_plans sp ON sp.id=us.plan_id
WHERE us.user_id=? AND us.status=1 AND us.end_at > ? AND sp.status=1
ORDER BY us.start_at ASC, us.end_at ASC, us.id ASC
`, userID, now)
	if err != nil {
		return nil, fmt.Errorf("查询 user_subscriptions 失败: %w", err)
	}
	defer rows.Close()

	var out []SubscriptionWithPlan
	for rows.Next() {
		var row SubscriptionWithPlan
		if err := rows.Scan(
			&row.Subscription.ID, &row.Subscription.UserID, &row.Subscription.PlanID, &row.Subscription.StartAt, &row.Subscription.EndAt, &row.Subscription.Status, &row.Subscription.CreatedAt, &row.Subscription.UpdatedAt,
			&row.Plan.ID, &row.Plan.Code, &row.Plan.Name, &row.Plan.GroupName, &row.Plan.PriceMultiplier, &row.Plan.PriceCNY, &row.Plan.Limit5HUSD, &row.Plan.Limit1DUSD, &row.Plan.Limit7DUSD, &row.Plan.Limit30DUSD, &row.Plan.DurationDays, &row.Plan.Status, &row.Plan.CreatedAt, &row.Plan.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描 user_subscriptions 失败: %w", err)
		}
		truncateSubscriptionPlanMoney(&row.Plan)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 user_subscriptions 失败: %w", err)
	}
	return out, nil
}

func (s *Store) PurchaseSubscriptionByPlanID(ctx context.Context, userID int64, planID int64, now time.Time) (UserSubscription, SubscriptionPlan, error) {
	plan, err := s.GetSubscriptionPlanByID(ctx, planID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserSubscription{}, SubscriptionPlan{}, errors.New("订阅套餐不可用")
		}
		return UserSubscription{}, SubscriptionPlan{}, err
	}
	if plan.Status != 1 {
		return UserSubscription{}, SubscriptionPlan{}, errors.New("订阅套餐不可用")
	}
	group := strings.TrimSpace(plan.GroupName)
	if group == "" {
		group = DefaultGroupName
	}
	ok, err := s.UserMainGroupAllowsSubgroup(ctx, userID, group)
	if err != nil {
		return UserSubscription{}, SubscriptionPlan{}, err
	}
	if !ok {
		return UserSubscription{}, SubscriptionPlan{}, errors.New("无权限购买该套餐")
	}
	if plan.DurationDays <= 0 {
		plan.DurationDays = 30
	}
	duration := time.Duration(plan.DurationDays) * 24 * time.Hour

	us := UserSubscription{
		UserID:  userID,
		PlanID:  plan.ID,
		StartAt: now,
		EndAt:   now.Add(duration),
		Status:  1,
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO user_subscriptions(user_id, plan_id, start_at, end_at, status, created_at, updated_at)
VALUES(?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, us.UserID, us.PlanID, us.StartAt, us.EndAt)
	if err != nil {
		return UserSubscription{}, SubscriptionPlan{}, fmt.Errorf("创建 user_subscription 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return UserSubscription{}, SubscriptionPlan{}, fmt.Errorf("获取 user_subscription id 失败: %w", err)
	}
	us.ID = id
	return us, plan, nil
}

type SubscriptionPlanCreate struct {
	Code            string
	Name            string
	GroupName       string
	PriceMultiplier decimal.Decimal
	PriceCNY        decimal.Decimal
	Limit5HUSD      decimal.Decimal
	Limit1DUSD      decimal.Decimal
	Limit7DUSD      decimal.Decimal
	Limit30DUSD     decimal.Decimal
	DurationDays    int
	Status          int
}

type SubscriptionPlanUpdate struct {
	ID              int64
	Code            string
	Name            string
	GroupName       string
	PriceMultiplier decimal.Decimal
	PriceCNY        decimal.Decimal
	Limit5HUSD      decimal.Decimal
	Limit1DUSD      decimal.Decimal
	Limit7DUSD      decimal.Decimal
	Limit30DUSD     decimal.Decimal
	DurationDays    int
	Status          int
}

func (s *Store) ListAllSubscriptionPlans(ctx context.Context) ([]SubscriptionPlan, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, code, name, group_name, price_multiplier, price_cny, limit_5h_usd, limit_1d_usd, limit_7d_usd, limit_30d_usd, duration_days, status, created_at, updated_at
FROM subscription_plans
ORDER BY status DESC, id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("查询 subscription_plans 失败: %w", err)
	}
	defer rows.Close()

	var out []SubscriptionPlan
	for rows.Next() {
		var p SubscriptionPlan
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.GroupName, &p.PriceMultiplier, &p.PriceCNY, &p.Limit5HUSD, &p.Limit1DUSD, &p.Limit7DUSD, &p.Limit30DUSD, &p.DurationDays, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 subscription_plans 失败: %w", err)
		}
		truncateSubscriptionPlanMoney(&p)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 subscription_plans 失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetSubscriptionPlanByID(ctx context.Context, id int64) (SubscriptionPlan, error) {
	var p SubscriptionPlan
	err := s.db.QueryRowContext(ctx, `
SELECT id, code, name, group_name, price_multiplier, price_cny, limit_5h_usd, limit_1d_usd, limit_7d_usd, limit_30d_usd, duration_days, status, created_at, updated_at
FROM subscription_plans
WHERE id=?
`, id).Scan(&p.ID, &p.Code, &p.Name, &p.GroupName, &p.PriceMultiplier, &p.PriceCNY, &p.Limit5HUSD, &p.Limit1DUSD, &p.Limit7DUSD, &p.Limit30DUSD, &p.DurationDays, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SubscriptionPlan{}, sql.ErrNoRows
		}
		return SubscriptionPlan{}, fmt.Errorf("查询 subscription_plan 失败: %w", err)
	}
	truncateSubscriptionPlanMoney(&p)
	return p, nil
}

func (s *Store) CreateSubscriptionPlan(ctx context.Context, in SubscriptionPlanCreate) (int64, error) {
	if in.DurationDays <= 0 {
		in.DurationDays = 30
	}
	if in.Status != 0 && in.Status != 1 {
		in.Status = 1
	}
	if strings.TrimSpace(in.GroupName) == "" {
		in.GroupName = DefaultGroupName
	}

	priceMultiplier := in.PriceMultiplier
	if priceMultiplier.IsNegative() || priceMultiplier.LessThanOrEqual(decimal.Zero) {
		priceMultiplier = DefaultGroupPriceMultiplier
	}
	priceMultiplier = priceMultiplier.Truncate(PriceMultiplierScale)

	priceCNY := in.PriceCNY.Truncate(CNYScale)
	limit5HUSD := in.Limit5HUSD.Truncate(USDScale)
	limit1DUSD := in.Limit1DUSD.Truncate(USDScale)
	limit7DUSD := in.Limit7DUSD.Truncate(USDScale)
	limit30DUSD := in.Limit30DUSD.Truncate(USDScale)
	if priceCNY.IsNegative() || limit5HUSD.IsNegative() || limit1DUSD.IsNegative() || limit7DUSD.IsNegative() || limit30DUSD.IsNegative() {
		return 0, errors.New("订阅套餐金额不合法")
	}

	res, err := s.db.ExecContext(ctx, `
INSERT INTO subscription_plans(
  code, name, group_name, price_multiplier, price_cny,
  limit_5h_usd, limit_1d_usd, limit_7d_usd, limit_30d_usd,
  duration_days, status, created_at, updated_at
) VALUES(
  ?, ?, ?, ?, ?,
  ?, ?, ?, ?,
  ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
)
`, in.Code, in.Name, strings.TrimSpace(in.GroupName), priceMultiplier, priceCNY, limit5HUSD, limit1DUSD, limit7DUSD, limit30DUSD, in.DurationDays, in.Status)
	if err != nil {
		return 0, fmt.Errorf("创建 subscription_plan 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取 subscription_plan id 失败: %w", err)
	}
	return id, nil
}

func (s *Store) UpdateSubscriptionPlan(ctx context.Context, in SubscriptionPlanUpdate) error {
	if in.ID == 0 {
		return errors.New("id 不能为空")
	}
	if in.DurationDays <= 0 {
		in.DurationDays = 30
	}
	if in.Status != 0 && in.Status != 1 {
		in.Status = 1
	}
	if strings.TrimSpace(in.GroupName) == "" {
		in.GroupName = DefaultGroupName
	}

	priceMultiplier := in.PriceMultiplier
	if priceMultiplier.IsNegative() || priceMultiplier.LessThanOrEqual(decimal.Zero) {
		priceMultiplier = DefaultGroupPriceMultiplier
	}
	priceMultiplier = priceMultiplier.Truncate(PriceMultiplierScale)

	priceCNY := in.PriceCNY.Truncate(CNYScale)
	limit5HUSD := in.Limit5HUSD.Truncate(USDScale)
	limit1DUSD := in.Limit1DUSD.Truncate(USDScale)
	limit7DUSD := in.Limit7DUSD.Truncate(USDScale)
	limit30DUSD := in.Limit30DUSD.Truncate(USDScale)
	if priceCNY.IsNegative() || limit5HUSD.IsNegative() || limit1DUSD.IsNegative() || limit7DUSD.IsNegative() || limit30DUSD.IsNegative() {
		return errors.New("订阅套餐金额不合法")
	}

	_, err := s.db.ExecContext(ctx, `
UPDATE subscription_plans
SET code=?, name=?, group_name=?, price_multiplier=?, price_cny=?,
    limit_5h_usd=?, limit_1d_usd=?, limit_7d_usd=?, limit_30d_usd=?,
    duration_days=?, status=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, in.Code, in.Name, strings.TrimSpace(in.GroupName), priceMultiplier, priceCNY, limit5HUSD, limit1DUSD, limit7DUSD, limit30DUSD, in.DurationDays, in.Status, in.ID)
	if err != nil {
		return fmt.Errorf("更新 subscription_plan 失败: %w", err)
	}
	return nil
}

func (s *Store) DeleteSubscriptionPlan(ctx context.Context, id int64) (SubscriptionPlanDeleteSummary, error) {
	if id <= 0 {
		return SubscriptionPlanDeleteSummary{}, errors.New("id 不合法")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubscriptionPlanDeleteSummary{}, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var planID int64
	qPlan := "SELECT id FROM subscription_plans WHERE id=?" + forUpdateClause(s.dialect)
	if err := tx.QueryRowContext(ctx, qPlan, id).Scan(&planID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SubscriptionPlanDeleteSummary{}, sql.ErrNoRows
		}
		return SubscriptionPlanDeleteSummary{}, fmt.Errorf("查询 subscription_plan 失败: %w", err)
	}

	var sum SubscriptionPlanDeleteSummary

	res, err := tx.ExecContext(ctx, `
UPDATE usage_events
SET subscription_id=NULL
WHERE subscription_id IN (SELECT id FROM user_subscriptions WHERE plan_id=?)
`, id)
	if err != nil {
		return SubscriptionPlanDeleteSummary{}, fmt.Errorf("解绑 usage_events.subscription_id 失败: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil {
		sum.UsageEventsUnbound = n
	}

	res, err = tx.ExecContext(ctx, `DELETE FROM user_subscriptions WHERE plan_id=?`, id)
	if err != nil {
		return SubscriptionPlanDeleteSummary{}, fmt.Errorf("删除 user_subscriptions 失败: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil {
		sum.SubscriptionsDeleted = n
	}

	res, err = tx.ExecContext(ctx, `DELETE FROM subscription_orders WHERE plan_id=?`, id)
	if err != nil {
		return SubscriptionPlanDeleteSummary{}, fmt.Errorf("删除 subscription_orders 失败: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil {
		sum.OrdersDeleted = n
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM subscription_plans WHERE id=?`, id); err != nil {
		return SubscriptionPlanDeleteSummary{}, fmt.Errorf("删除 subscription_plan 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return SubscriptionPlanDeleteSummary{}, fmt.Errorf("提交事务失败: %w", err)
	}
	return sum, nil
}
