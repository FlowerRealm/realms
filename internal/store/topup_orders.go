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

const (
	TopupOrderStatusPending  = 0
	TopupOrderStatusPaid     = 1
	TopupOrderStatusCanceled = 2
)

type TopupOrderWithUser struct {
	Order     TopupOrder
	UserEmail string
}

func (s *Store) CreateTopupOrder(ctx context.Context, userID int64, amountCNY decimal.Decimal, creditUSD decimal.Decimal, now time.Time) (TopupOrder, error) {
	if userID <= 0 {
		return TopupOrder{}, errors.New("user_id 不能为空")
	}
	amountCNY = amountCNY.Truncate(CNYScale)
	if amountCNY.LessThanOrEqual(decimal.Zero) {
		return TopupOrder{}, errors.New("充值金额不合法")
	}
	creditUSD = creditUSD.Truncate(USDScale)
	if creditUSD.LessThanOrEqual(decimal.Zero) {
		return TopupOrder{}, errors.New("充值额度不合法")
	}
	if now.IsZero() {
		now = time.Now()
	}

	o := TopupOrder{
		UserID:    userID,
		AmountCNY: amountCNY,
		CreditUSD: creditUSD,
		Status:    TopupOrderStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}

	res, err := s.db.ExecContext(ctx, `
INSERT INTO topup_orders(user_id, amount_cny, credit_usd, status, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, o.UserID, o.AmountCNY, o.CreditUSD, o.Status)
	if err != nil {
		return TopupOrder{}, fmt.Errorf("创建 topup_order 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return TopupOrder{}, fmt.Errorf("获取 topup_order id 失败: %w", err)
	}
	o.ID = id
	return o, nil
}

func (s *Store) GetTopupOrderByID(ctx context.Context, orderID int64) (TopupOrder, error) {
	var o TopupOrder
	var paidAt sql.NullTime
	var paidMethod sql.NullString
	var paidRef sql.NullString
	var paidChannelID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, amount_cny, credit_usd, status, paid_at, paid_method, paid_ref, paid_channel_id, created_at, updated_at
FROM topup_orders
WHERE id=?
`, orderID).Scan(
		&o.ID, &o.UserID, &o.AmountCNY, &o.CreditUSD, &o.Status, &paidAt, &paidMethod, &paidRef, &paidChannelID, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TopupOrder{}, sql.ErrNoRows
		}
		return TopupOrder{}, fmt.Errorf("查询 topup_order 失败: %w", err)
	}
	o.AmountCNY = o.AmountCNY.Truncate(CNYScale)
	o.CreditUSD = o.CreditUSD.Truncate(USDScale)
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
	return o, nil
}

func (s *Store) ListTopupOrdersByUser(ctx context.Context, userID int64, limit int) ([]TopupOrder, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, amount_cny, credit_usd, status, paid_at, paid_method, paid_ref, paid_channel_id, created_at, updated_at
FROM topup_orders
WHERE user_id=?
ORDER BY id DESC
LIMIT ?
`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("查询 topup_orders 失败: %w", err)
	}
	defer rows.Close()

	var out []TopupOrder
	for rows.Next() {
		var o TopupOrder
		var paidAt sql.NullTime
		var paidMethod sql.NullString
		var paidRef sql.NullString
		var paidChannelID sql.NullInt64
		if err := rows.Scan(
			&o.ID, &o.UserID, &o.AmountCNY, &o.CreditUSD, &o.Status,
			&paidAt, &paidMethod, &paidRef, &paidChannelID,
			&o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描 topup_orders 失败: %w", err)
		}
		o.AmountCNY = o.AmountCNY.Truncate(CNYScale)
		o.CreditUSD = o.CreditUSD.Truncate(USDScale)
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
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 topup_orders 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListRecentTopupOrders(ctx context.Context, limit int) ([]TopupOrderWithUser, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  o.id, o.user_id, u.email, o.amount_cny, o.credit_usd, o.status,
  o.paid_at, o.paid_method, o.paid_ref, o.paid_channel_id,
  o.created_at, o.updated_at
FROM topup_orders o
JOIN users u ON u.id=o.user_id
ORDER BY o.id DESC
LIMIT ?
`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询 topup_orders 失败: %w", err)
	}
	defer rows.Close()

	var out []TopupOrderWithUser
	for rows.Next() {
		var row TopupOrderWithUser
		var paidAt sql.NullTime
		var paidMethod sql.NullString
		var paidRef sql.NullString
		var paidChannelID sql.NullInt64
		if err := rows.Scan(
			&row.Order.ID, &row.Order.UserID, &row.UserEmail, &row.Order.AmountCNY, &row.Order.CreditUSD, &row.Order.Status,
			&paidAt, &paidMethod, &paidRef, &paidChannelID,
			&row.Order.CreatedAt, &row.Order.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描 topup_orders 失败: %w", err)
		}
		row.Order.AmountCNY = row.Order.AmountCNY.Truncate(CNYScale)
		row.Order.CreditUSD = row.Order.CreditUSD.Truncate(USDScale)
		if paidAt.Valid {
			t := paidAt.Time
			row.Order.PaidAt = &t
		}
		if paidMethod.Valid {
			v := strings.TrimSpace(paidMethod.String)
			if v != "" {
				row.Order.PaidMethod = &v
			}
		}
		if paidRef.Valid {
			v := strings.TrimSpace(paidRef.String)
			if v != "" {
				row.Order.PaidRef = &v
			}
		}
		if paidChannelID.Valid {
			v := paidChannelID.Int64
			if v > 0 {
				row.Order.PaidChannelID = &v
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 topup_orders 失败: %w", err)
	}
	return out, nil
}

func (s *Store) MarkTopupOrderPaid(ctx context.Context, orderID int64, paidMethod, paidRef *string, paidChannelID *int64, paidAt time.Time) error {
	if orderID <= 0 {
		return errors.New("order_id 不能为空")
	}
	if paidAt.IsZero() {
		paidAt = time.Now()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var o TopupOrder
	var amountCNY decimal.Decimal
	var creditUSD decimal.Decimal
	var status int
	var existingPaidChannelID sql.NullInt64
	qOrder := `
SELECT id, user_id, amount_cny, credit_usd, status, paid_channel_id
FROM topup_orders
WHERE id=?
` + forUpdateClause(s.dialect)
	err = tx.QueryRowContext(ctx, qOrder, orderID).Scan(&o.ID, &o.UserID, &amountCNY, &creditUSD, &status, &existingPaidChannelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("订单不存在")
		}
		return fmt.Errorf("查询订单失败: %w", err)
	}
	if status == TopupOrderStatusCanceled {
		// 订单已取消：不入账，但会尽量记录支付元信息，便于后续人工退款处理。
		if _, err := tx.ExecContext(ctx, `
UPDATE topup_orders
SET paid_at=COALESCE(paid_at, ?),
    paid_method=COALESCE(paid_method, ?),
    paid_ref=COALESCE(paid_ref, ?),
    paid_channel_id=COALESCE(paid_channel_id, ?),
    updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, paidAt, paidMethod, paidRef, paidChannelID, o.ID); err != nil {
			return fmt.Errorf("更新订单失败: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交事务失败: %w", err)
		}
		return ErrOrderCanceled
	}
	if status == TopupOrderStatusPaid {
		if paidChannelID != nil && *paidChannelID > 0 && !existingPaidChannelID.Valid {
			if _, err := tx.ExecContext(ctx, `
UPDATE topup_orders
SET paid_channel_id=?, updated_at=updated_at
WHERE id=?
`, *paidChannelID, o.ID); err != nil {
				return fmt.Errorf("更新订单失败: %w", err)
			}
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("提交事务失败: %w", err)
			}
		}
		return nil
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE topup_orders
SET status=?, paid_at=?, paid_method=?, paid_ref=?, paid_channel_id=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
	`, TopupOrderStatusPaid, paidAt, paidMethod, paidRef, paidChannelID, o.ID); err != nil {
		return fmt.Errorf("更新订单失败: %w", err)
	}

	stmtInitBalance := fmt.Sprintf(`
%s INTO user_balances(user_id, usd, created_at, updated_at)
VALUES(?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, insertIgnoreVerb(s.dialect))
	if _, err := tx.ExecContext(ctx, stmtInitBalance, o.UserID); err != nil {
		return fmt.Errorf("初始化余额失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, userBalancesAddSQL(s.dialect), creditUSD, o.UserID); err != nil {
		return fmt.Errorf("入账失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) CancelTopupOrderByUser(ctx context.Context, userID int64, orderID int64) error {
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
	qStatus := `
SELECT status
FROM topup_orders
WHERE id=? AND user_id=?
` + forUpdateClause(s.dialect)
	if err := tx.QueryRowContext(ctx, qStatus, orderID, userID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("查询订单失败: %w", err)
	}

	switch status {
	case TopupOrderStatusPending:
		if _, err := tx.ExecContext(ctx, `
UPDATE topup_orders
SET status=?, updated_at=CURRENT_TIMESTAMP
WHERE id=? AND user_id=?
`, TopupOrderStatusCanceled, orderID, userID); err != nil {
			return fmt.Errorf("更新订单失败: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交事务失败: %w", err)
		}
		return nil
	case TopupOrderStatusCanceled:
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交事务失败: %w", err)
		}
		return nil
	default:
		return errors.New("订单状态不可取消")
	}
}
