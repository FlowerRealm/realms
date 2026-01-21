// subscription_orders.go 提供订阅订单的创建、查询与生效（支付/批准）逻辑。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	SubscriptionOrderStatusPending  = 0
	SubscriptionOrderStatusActive   = 1
	SubscriptionOrderStatusCanceled = 2
)

type SubscriptionOrderWithPlan struct {
	Order SubscriptionOrder
	Plan  SubscriptionPlan
}

type SubscriptionOrderWithUserAndPlan struct {
	Order     SubscriptionOrder
	UserEmail string
	Plan      SubscriptionPlan
}

func (s *Store) GetSubscriptionOrderByID(ctx context.Context, orderID int64) (SubscriptionOrder, error) {
	var o SubscriptionOrder
	var paidAt sql.NullTime
	var paidMethod sql.NullString
	var paidRef sql.NullString
	var paidChannelID sql.NullInt64
	var approvedAt sql.NullTime
	var approvedBy sql.NullInt64
	var subID sql.NullInt64
	var note sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, plan_id, amount_cny, status,
  paid_at, paid_method, paid_ref, paid_channel_id,
  approved_at, approved_by,
  subscription_id, note,
  created_at, updated_at
FROM subscription_orders
WHERE id=?
`, orderID).Scan(
		&o.ID, &o.UserID, &o.PlanID, &o.AmountCNY, &o.Status,
		&paidAt, &paidMethod, &paidRef, &paidChannelID,
		&approvedAt, &approvedBy,
		&subID, &note,
		&o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SubscriptionOrder{}, sql.ErrNoRows
		}
		return SubscriptionOrder{}, fmt.Errorf("查询 subscription_order 失败: %w", err)
	}
	o.AmountCNY = o.AmountCNY.Truncate(CNYScale)
	if paidAt.Valid {
		t := paidAt.Time
		o.PaidAt = &t
	}
	if paidMethod.Valid {
		v := strings.TrimSpace(paidMethod.String)
		if v != "" {
			o.PaidMethod = &v
		}
	}
	if paidRef.Valid {
		v := strings.TrimSpace(paidRef.String)
		if v != "" {
			o.PaidRef = &v
		}
	}
	if paidChannelID.Valid {
		v := paidChannelID.Int64
		if v > 0 {
			o.PaidChannelID = &v
		}
	}
	if approvedAt.Valid {
		t := approvedAt.Time
		o.ApprovedAt = &t
	}
	if approvedBy.Valid {
		v := approvedBy.Int64
		o.ApprovedBy = &v
	}
	if subID.Valid {
		v := subID.Int64
		o.SubscriptionID = &v
	}
	if note.Valid {
		v := strings.TrimSpace(note.String)
		if v != "" {
			o.Note = &v
		}
	}
	return o, nil
}

func (s *Store) CreateSubscriptionOrderByPlanID(ctx context.Context, userID int64, planID int64, now time.Time) (SubscriptionOrder, SubscriptionPlan, error) {
	plan, err := s.GetSubscriptionPlanByID(ctx, planID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SubscriptionOrder{}, SubscriptionPlan{}, errors.New("订阅套餐不可用")
		}
		return SubscriptionOrder{}, SubscriptionPlan{}, err
	}
	if plan.Status != 1 {
		return SubscriptionOrder{}, SubscriptionPlan{}, errors.New("订阅套餐不可用")
	}

	group := strings.TrimSpace(plan.GroupName)
	if group == "" {
		group = DefaultGroupName
	}
	ok, err := s.UserHasGroup(ctx, userID, group)
	if err != nil {
		return SubscriptionOrder{}, SubscriptionPlan{}, err
	}
	if !ok {
		return SubscriptionOrder{}, SubscriptionPlan{}, errors.New("无权限购买该套餐")
	}

	o := SubscriptionOrder{
		UserID:    userID,
		PlanID:    plan.ID,
		AmountCNY: plan.PriceCNY.Truncate(CNYScale),
		Status:    SubscriptionOrderStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO subscription_orders(user_id, plan_id, amount_cny, status, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, o.UserID, o.PlanID, o.AmountCNY, o.Status)
	if err != nil {
		return SubscriptionOrder{}, SubscriptionPlan{}, fmt.Errorf("创建 subscription_order 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return SubscriptionOrder{}, SubscriptionPlan{}, fmt.Errorf("获取 subscription_order id 失败: %w", err)
	}
	o.ID = id
	return o, plan, nil
}

func (s *Store) ListSubscriptionOrdersByUser(ctx context.Context, userID int64, limit int) ([]SubscriptionOrderWithPlan, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  o.id, o.user_id, o.plan_id, o.amount_cny, o.status,
  o.paid_at, o.paid_method, o.paid_ref, o.paid_channel_id,
  o.approved_at, o.approved_by,
  o.subscription_id, o.note,
  o.created_at, o.updated_at,
  sp.id, sp.code, sp.name, sp.group_name, sp.price_cny, sp.limit_5h_usd, sp.limit_1d_usd, sp.limit_7d_usd, sp.limit_30d_usd, sp.duration_days, sp.status, sp.created_at, sp.updated_at
FROM subscription_orders o
JOIN subscription_plans sp ON sp.id=o.plan_id
WHERE o.user_id=?
ORDER BY o.id DESC
LIMIT ?
`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("查询 subscription_orders 失败: %w", err)
	}
	defer rows.Close()

	var out []SubscriptionOrderWithPlan
	for rows.Next() {
		var row SubscriptionOrderWithPlan
		var paidAt sql.NullTime
		var paidMethod sql.NullString
		var paidRef sql.NullString
		var paidChannelID sql.NullInt64
		var approvedAt sql.NullTime
		var approvedBy sql.NullInt64
		var subID sql.NullInt64
		var note sql.NullString
		if err := rows.Scan(
			&row.Order.ID, &row.Order.UserID, &row.Order.PlanID, &row.Order.AmountCNY, &row.Order.Status,
			&paidAt, &paidMethod, &paidRef, &paidChannelID,
			&approvedAt, &approvedBy,
			&subID, &note,
			&row.Order.CreatedAt, &row.Order.UpdatedAt,
			&row.Plan.ID, &row.Plan.Code, &row.Plan.Name, &row.Plan.GroupName, &row.Plan.PriceCNY, &row.Plan.Limit5HUSD, &row.Plan.Limit1DUSD, &row.Plan.Limit7DUSD, &row.Plan.Limit30DUSD, &row.Plan.DurationDays, &row.Plan.Status, &row.Plan.CreatedAt, &row.Plan.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描 subscription_orders 失败: %w", err)
		}
		row.Order.AmountCNY = row.Order.AmountCNY.Truncate(CNYScale)
		row.Plan.PriceCNY = row.Plan.PriceCNY.Truncate(CNYScale)
		row.Plan.Limit5HUSD = row.Plan.Limit5HUSD.Truncate(USDScale)
		row.Plan.Limit1DUSD = row.Plan.Limit1DUSD.Truncate(USDScale)
		row.Plan.Limit7DUSD = row.Plan.Limit7DUSD.Truncate(USDScale)
		row.Plan.Limit30DUSD = row.Plan.Limit30DUSD.Truncate(USDScale)
		if paidAt.Valid {
			t := paidAt.Time
			row.Order.PaidAt = &t
		}
		if paidMethod.Valid {
			s := strings.TrimSpace(paidMethod.String)
			if s != "" {
				row.Order.PaidMethod = &s
			}
		}
		if paidRef.Valid {
			s := strings.TrimSpace(paidRef.String)
			if s != "" {
				row.Order.PaidRef = &s
			}
		}
		if paidChannelID.Valid {
			v := paidChannelID.Int64
			if v > 0 {
				row.Order.PaidChannelID = &v
			}
		}
		if approvedAt.Valid {
			t := approvedAt.Time
			row.Order.ApprovedAt = &t
		}
		if approvedBy.Valid {
			v := approvedBy.Int64
			row.Order.ApprovedBy = &v
		}
		if subID.Valid {
			v := subID.Int64
			row.Order.SubscriptionID = &v
		}
		if note.Valid {
			s := strings.TrimSpace(note.String)
			if s != "" {
				row.Order.Note = &s
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 subscription_orders 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListRecentSubscriptionOrders(ctx context.Context, limit int) ([]SubscriptionOrderWithUserAndPlan, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  o.id, o.user_id, u.email, o.plan_id, o.amount_cny, o.status,
  o.paid_at, o.paid_method, o.paid_ref, o.paid_channel_id,
  o.approved_at, o.approved_by,
  o.subscription_id, o.note,
  o.created_at, o.updated_at,
  sp.id, sp.code, sp.name, sp.group_name, sp.price_cny, sp.limit_5h_usd, sp.limit_1d_usd, sp.limit_7d_usd, sp.limit_30d_usd, sp.duration_days, sp.status, sp.created_at, sp.updated_at
FROM subscription_orders o
JOIN users u ON u.id=o.user_id
JOIN subscription_plans sp ON sp.id=o.plan_id
ORDER BY o.id DESC
LIMIT ?
`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询 subscription_orders 失败: %w", err)
	}
	defer rows.Close()

	var out []SubscriptionOrderWithUserAndPlan
	for rows.Next() {
		var row SubscriptionOrderWithUserAndPlan
		var paidAt sql.NullTime
		var paidMethod sql.NullString
		var paidRef sql.NullString
		var paidChannelID sql.NullInt64
		var approvedAt sql.NullTime
		var approvedBy sql.NullInt64
		var subID sql.NullInt64
		var note sql.NullString
		if err := rows.Scan(
			&row.Order.ID, &row.Order.UserID, &row.UserEmail, &row.Order.PlanID, &row.Order.AmountCNY, &row.Order.Status,
			&paidAt, &paidMethod, &paidRef, &paidChannelID,
			&approvedAt, &approvedBy,
			&subID, &note,
			&row.Order.CreatedAt, &row.Order.UpdatedAt,
			&row.Plan.ID, &row.Plan.Code, &row.Plan.Name, &row.Plan.GroupName, &row.Plan.PriceCNY, &row.Plan.Limit5HUSD, &row.Plan.Limit1DUSD, &row.Plan.Limit7DUSD, &row.Plan.Limit30DUSD, &row.Plan.DurationDays, &row.Plan.Status, &row.Plan.CreatedAt, &row.Plan.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描 subscription_orders 失败: %w", err)
		}
		row.Order.AmountCNY = row.Order.AmountCNY.Truncate(CNYScale)
		row.Plan.PriceCNY = row.Plan.PriceCNY.Truncate(CNYScale)
		row.Plan.Limit5HUSD = row.Plan.Limit5HUSD.Truncate(USDScale)
		row.Plan.Limit1DUSD = row.Plan.Limit1DUSD.Truncate(USDScale)
		row.Plan.Limit7DUSD = row.Plan.Limit7DUSD.Truncate(USDScale)
		row.Plan.Limit30DUSD = row.Plan.Limit30DUSD.Truncate(USDScale)
		if paidAt.Valid {
			t := paidAt.Time
			row.Order.PaidAt = &t
		}
		if paidMethod.Valid {
			s := strings.TrimSpace(paidMethod.String)
			if s != "" {
				row.Order.PaidMethod = &s
			}
		}
		if paidRef.Valid {
			s := strings.TrimSpace(paidRef.String)
			if s != "" {
				row.Order.PaidRef = &s
			}
		}
		if paidChannelID.Valid {
			v := paidChannelID.Int64
			if v > 0 {
				row.Order.PaidChannelID = &v
			}
		}
		if approvedAt.Valid {
			t := approvedAt.Time
			row.Order.ApprovedAt = &t
		}
		if approvedBy.Valid {
			v := approvedBy.Int64
			row.Order.ApprovedBy = &v
		}
		if subID.Valid {
			v := subID.Int64
			row.Order.SubscriptionID = &v
		}
		if note.Valid {
			s := strings.TrimSpace(note.String)
			if s != "" {
				row.Order.Note = &s
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 subscription_orders 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ApproveSubscriptionOrderAndDelete(ctx context.Context, orderID int64, approverUserID int64, approvedAt time.Time) (int64, error) {
	if orderID <= 0 {
		return 0, errors.New("order_id 不能为空")
	}
	if approverUserID <= 0 {
		return 0, errors.New("approver_user_id 不能为空")
	}
	if approvedAt.IsZero() {
		approvedAt = time.Now()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var o SubscriptionOrder
	var subID sql.NullInt64
	qOrder := `
SELECT id, user_id, plan_id, status, subscription_id, created_at, updated_at
FROM subscription_orders
WHERE id=?
` + forUpdateClause(s.dialect)
	err = tx.QueryRowContext(ctx, qOrder, orderID).Scan(&o.ID, &o.UserID, &o.PlanID, &o.Status, &subID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errors.New("订单不存在")
		}
		return 0, fmt.Errorf("查询订单失败: %w", err)
	}
	if o.Status == SubscriptionOrderStatusCanceled {
		return 0, ErrOrderCanceled
	}

	var plan SubscriptionPlan
	err = tx.QueryRowContext(ctx, `
SELECT id, group_name, duration_days, status, created_at, updated_at
FROM subscription_plans
WHERE id=?
`, o.PlanID).Scan(
		&plan.ID, &plan.GroupName, &plan.DurationDays, &plan.Status, &plan.CreatedAt, &plan.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errors.New("订阅套餐不存在")
		}
		return 0, fmt.Errorf("查询 subscription_plan 失败: %w", err)
	}
	if plan.Status != 1 {
		return 0, errors.New("订阅套餐不可用")
	}
	if plan.DurationDays <= 0 {
		plan.DurationDays = 30
	}

	subscriptionID := int64(0)
	if subID.Valid {
		subscriptionID = subID.Int64
	} else {
		us := UserSubscription{
			UserID:  o.UserID,
			PlanID:  o.PlanID,
			StartAt: approvedAt,
			EndAt:   approvedAt.Add(time.Duration(plan.DurationDays) * 24 * time.Hour),
			Status:  1,
		}
		res, err := tx.ExecContext(ctx, `
INSERT INTO user_subscriptions(user_id, plan_id, start_at, end_at, status, created_at, updated_at)
VALUES(?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, us.UserID, us.PlanID, us.StartAt, us.EndAt)
		if err != nil {
			return 0, fmt.Errorf("创建 user_subscription 失败: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("获取 user_subscription id 失败: %w", err)
		}
		subscriptionID = id
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE subscription_orders
SET status=?, approved_at=?, approved_by=?, subscription_id=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, SubscriptionOrderStatusActive, approvedAt, approverUserID, subscriptionID, o.ID); err != nil {
		return 0, fmt.Errorf("更新订单失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交事务失败: %w", err)
	}
	return subscriptionID, nil
}

func (s *Store) MarkSubscriptionOrderPaidAndActivateAndDelete(ctx context.Context, orderID int64, paidAt time.Time) (subscriptionID int64, processed bool, err error) {
	method := "webhook"
	return s.MarkSubscriptionOrderPaidAndActivate(ctx, orderID, paidAt, &method, nil, nil)
}

func (s *Store) RejectSubscriptionOrderAndDelete(ctx context.Context, orderID int64, rejecterUserID int64) error {
	if orderID <= 0 {
		return errors.New("order_id 不能为空")
	}
	if rejecterUserID <= 0 {
		return errors.New("rejecter_user_id 不能为空")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status int
	var subID sql.NullInt64
	q := `
SELECT status, subscription_id
FROM subscription_orders
WHERE id=?
` + forUpdateClause(s.dialect)
	if err := tx.QueryRowContext(ctx, q, orderID).Scan(&status, &subID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("订单不存在")
		}
		return fmt.Errorf("查询订单失败: %w", err)
	}
	if status == SubscriptionOrderStatusActive || subID.Valid {
		return errors.New("订单已生效，无法拒绝")
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE subscription_orders
SET status=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, SubscriptionOrderStatusCanceled, orderID); err != nil {
		return fmt.Errorf("更新订单失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) MarkSubscriptionOrderPaidAndActivate(ctx context.Context, orderID int64, paidAt time.Time, paidMethod, paidRef *string, paidChannelID *int64) (subscriptionID int64, processed bool, err error) {
	if orderID <= 0 {
		return 0, false, errors.New("order_id 不能为空")
	}
	if paidAt.IsZero() {
		paidAt = time.Now()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var o SubscriptionOrder
	var subID sql.NullInt64
	var existingPaidChannelID sql.NullInt64
	qOrder := `
SELECT id, user_id, plan_id, status, subscription_id, paid_channel_id, created_at, updated_at
FROM subscription_orders
WHERE id=?
` + forUpdateClause(s.dialect)
	err = tx.QueryRowContext(ctx, qOrder, orderID).Scan(&o.ID, &o.UserID, &o.PlanID, &o.Status, &subID, &existingPaidChannelID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("查询订单失败: %w", err)
	}

	if o.Status == SubscriptionOrderStatusCanceled {
		note := "订单已取消，但仍收到支付回调：请人工退款"
		if _, err := tx.ExecContext(ctx, `
UPDATE subscription_orders
SET paid_at=COALESCE(paid_at, ?),
    paid_method=COALESCE(paid_method, ?),
    paid_ref=COALESCE(paid_ref, ?),
    paid_channel_id=COALESCE(paid_channel_id, ?),
    note=COALESCE(note, ?),
    updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, paidAt, paidMethod, paidRef, paidChannelID, note, o.ID); err != nil {
			return 0, true, fmt.Errorf("更新订单失败: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return 0, true, fmt.Errorf("提交事务失败: %w", err)
		}
		return 0, true, ErrOrderCanceled
	}

	if o.Status == SubscriptionOrderStatusActive {
		if paidChannelID != nil && *paidChannelID > 0 && !existingPaidChannelID.Valid {
			if _, err := tx.ExecContext(ctx, `
UPDATE subscription_orders
SET paid_channel_id=?, updated_at=updated_at
WHERE id=?
`, *paidChannelID, o.ID); err != nil {
				return 0, true, fmt.Errorf("更新订单失败: %w", err)
			}
			if err := tx.Commit(); err != nil {
				return 0, true, fmt.Errorf("提交事务失败: %w", err)
			}
		}
		if subID.Valid {
			return subID.Int64, true, nil
		}
		if err := tx.Commit(); err != nil {
			return 0, true, fmt.Errorf("提交事务失败: %w", err)
		}
		return 0, true, nil
	}

	var plan SubscriptionPlan
	err = tx.QueryRowContext(ctx, `
SELECT id, group_name, duration_days, status, created_at, updated_at
FROM subscription_plans
WHERE id=?
`, o.PlanID).Scan(
		&plan.ID, &plan.GroupName, &plan.DurationDays, &plan.Status, &plan.CreatedAt, &plan.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, true, errors.New("订阅套餐不存在")
		}
		return 0, true, fmt.Errorf("查询 subscription_plan 失败: %w", err)
	}
	if plan.Status != 1 {
		return 0, true, errors.New("订阅套餐不可用")
	}
	if plan.DurationDays <= 0 {
		plan.DurationDays = 30
	}

	if subID.Valid {
		subscriptionID = subID.Int64
	} else {
		us := UserSubscription{
			UserID:  o.UserID,
			PlanID:  o.PlanID,
			StartAt: paidAt,
			EndAt:   paidAt.Add(time.Duration(plan.DurationDays) * 24 * time.Hour),
			Status:  1,
		}
		res, err := tx.ExecContext(ctx, `
INSERT INTO user_subscriptions(user_id, plan_id, start_at, end_at, status, created_at, updated_at)
VALUES(?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, us.UserID, us.PlanID, us.StartAt, us.EndAt)
		if err != nil {
			return 0, true, fmt.Errorf("创建 user_subscription 失败: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, true, fmt.Errorf("获取 user_subscription id 失败: %w", err)
		}
		subscriptionID = id
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE subscription_orders
SET status=?, paid_at=?, paid_method=?, paid_ref=?, paid_channel_id=COALESCE(paid_channel_id, ?), subscription_id=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, SubscriptionOrderStatusActive, paidAt, paidMethod, paidRef, paidChannelID, subscriptionID, o.ID); err != nil {
		return 0, true, fmt.Errorf("更新订单失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, true, fmt.Errorf("提交事务失败: %w", err)
	}
	return subscriptionID, true, nil
}

func (s *Store) CancelSubscriptionOrderByUser(ctx context.Context, userID int64, orderID int64) error {
	if userID <= 0 {
		return errors.New("user_id 不能为空")
	}
	if orderID <= 0 {
		return errors.New("order_id 不能为空")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status int
	q := `
SELECT status
FROM subscription_orders
WHERE id=? AND user_id=?
` + forUpdateClause(s.dialect)
	if err := tx.QueryRowContext(ctx, q, orderID, userID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("查询订单失败: %w", err)
	}

	switch status {
	case SubscriptionOrderStatusPending:
		note := "用户关闭订单"
		if _, err := tx.ExecContext(ctx, `
UPDATE subscription_orders
SET status=?, note=COALESCE(note, ?), updated_at=CURRENT_TIMESTAMP
WHERE id=? AND user_id=?
`, SubscriptionOrderStatusCanceled, note, orderID, userID); err != nil {
			return fmt.Errorf("更新订单失败: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交事务失败: %w", err)
		}
		return nil
	case SubscriptionOrderStatusCanceled:
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交事务失败: %w", err)
		}
		return nil
	default:
		return errors.New("订单状态不可取消")
	}
}
