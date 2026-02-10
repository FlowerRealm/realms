// Package store 封装用量事件（预留→结算→作废/过期）状态机，避免配额并发穿透。
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
	UsageStateReserved  = "reserved"
	UsageStateCommitted = "committed"
	UsageStateVoid      = "void"
	UsageStateExpired   = "expired"
)

type ReserveUsageInput struct {
	RequestID        string
	UserID           int64
	SubscriptionID   *int64
	TokenID          int64
	Model            *string
	ReservedUSD      decimal.Decimal
	ReserveExpiresAt time.Time
}

func (s *Store) ReserveUsage(ctx context.Context, in ReserveUsageInput) (int64, error) {
	if in.UserID <= 0 {
		return 0, errors.New("user_id 不能为空")
	}
	if in.TokenID <= 0 {
		return 0, errors.New("token_id 不能为空")
	}
	if in.ReserveExpiresAt.IsZero() {
		return 0, errors.New("reserve_expires_at 不能为空")
	}
	if in.ReservedUSD.IsNegative() {
		return 0, errors.New("reserved_usd 不合法")
	}
	reservedUSD := in.ReservedUSD.Truncate(USDScale)
	res, err := s.db.ExecContext(ctx, `
INSERT INTO usage_events(
  time, request_id, user_id, subscription_id, token_id, state, model,
  reserved_usd, committed_usd, reserve_expires_at, created_at, updated_at
) VALUES(
  CURRENT_TIMESTAMP, ?, ?, ?, ?, ?, ?,
  ?, 0, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
)
`, in.RequestID, in.UserID, in.SubscriptionID, in.TokenID, UsageStateReserved, in.Model, reservedUSD, in.ReserveExpiresAt)
	if err != nil {
		return 0, fmt.Errorf("写入 usage_events(reserved) 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取 usage_event id 失败: %w", err)
	}
	return id, nil
}

type CommitUsageInput struct {
	UsageEventID             int64
	UpstreamChannelID        *int64
	InputTokens              *int64
	CachedInputTokens        *int64
	OutputTokens             *int64
	CachedOutputTokens       *int64
	CommittedUSD             decimal.Decimal
	PriceMultiplier          decimal.Decimal
	PriceMultiplierGroup     decimal.Decimal
	PriceMultiplierPayment   decimal.Decimal
	PriceMultiplierGroupName *string
}

func (s *Store) CommitUsage(ctx context.Context, in CommitUsageInput) error {
	if in.UsageEventID <= 0 {
		return nil
	}
	if in.CommittedUSD.IsNegative() {
		return errors.New("committed_usd 不合法")
	}
	committedUSD := in.CommittedUSD.Truncate(USDScale)
	priceMultiplier := in.PriceMultiplier
	if priceMultiplier.IsNegative() || priceMultiplier.LessThanOrEqual(decimal.Zero) {
		priceMultiplier = DefaultGroupPriceMultiplier
	}
	priceMultiplier = priceMultiplier.Truncate(PriceMultiplierScale)
	priceMultiplierGroup := in.PriceMultiplierGroup
	if priceMultiplierGroup.IsNegative() || priceMultiplierGroup.LessThanOrEqual(decimal.Zero) {
		priceMultiplierGroup = DefaultGroupPriceMultiplier
	}
	priceMultiplierGroup = priceMultiplierGroup.Truncate(PriceMultiplierScale)
	priceMultiplierPayment := in.PriceMultiplierPayment
	if priceMultiplierPayment.IsNegative() || priceMultiplierPayment.LessThanOrEqual(decimal.Zero) {
		priceMultiplierPayment = DefaultGroupPriceMultiplier
	}
	priceMultiplierPayment = priceMultiplierPayment.Truncate(PriceMultiplierScale)
	res, err := s.db.ExecContext(ctx, `
UPDATE usage_events
SET state=?, upstream_channel_id=?, input_tokens=?, cached_input_tokens=?, output_tokens=?, cached_output_tokens=?, committed_usd=?,
    price_multiplier=?, price_multiplier_group=?, price_multiplier_payment=?, price_multiplier_group_name=?,
    updated_at=CURRENT_TIMESTAMP
WHERE id=? AND state=?
`, UsageStateCommitted, in.UpstreamChannelID, in.InputTokens, in.CachedInputTokens, in.OutputTokens, in.CachedOutputTokens, committedUSD,
		priceMultiplier, priceMultiplierGroup, priceMultiplierPayment, in.PriceMultiplierGroupName,
		in.UsageEventID, UsageStateReserved)
	if err != nil {
		return fmt.Errorf("结算 usage_event 失败: %w", err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type FinalizeUsageEventInput struct {
	UsageEventID        int64
	Endpoint            string
	Method              string
	StatusCode          int
	LatencyMS           int
	FirstTokenLatencyMS int
	ErrorClass          *string
	ErrorMessage        *string
	UpstreamChannelID   *int64
	UpstreamEndpointID  *int64
	UpstreamCredID      *int64
	IsStream            bool
	RequestBytes        int64
	ResponseBytes       int64
}

func (s *Store) FinalizeUsageEvent(ctx context.Context, in FinalizeUsageEventInput) error {
	if in.UsageEventID <= 0 {
		return errors.New("usage_event_id 不能为空")
	}

	endpoint := strings.TrimSpace(in.Endpoint)
	if len(endpoint) > 128 {
		endpoint = endpoint[:128]
	}
	var endpointAny any
	if endpoint != "" {
		endpointAny = endpoint
	}

	method := strings.TrimSpace(in.Method)
	if len(method) > 16 {
		method = method[:16]
	}
	var methodAny any
	if method != "" {
		methodAny = method
	}

	statusCode := in.StatusCode
	if statusCode < 0 || statusCode > 999 {
		statusCode = 0
	}
	latencyMS := in.LatencyMS
	if latencyMS < 0 {
		latencyMS = 0
	}
	firstTokenLatencyMS := in.FirstTokenLatencyMS
	if firstTokenLatencyMS < 0 {
		firstTokenLatencyMS = 0
	}
	if firstTokenLatencyMS > latencyMS {
		firstTokenLatencyMS = latencyMS
	}

	var errClassPtr *string
	if in.ErrorClass != nil {
		c := strings.TrimSpace(*in.ErrorClass)
		if len(c) > 64 {
			c = c[:64]
		}
		if c != "" {
			errClassPtr = &c
		}
	}
	var errMsgPtr *string
	if in.ErrorMessage != nil {
		m := strings.TrimSpace(*in.ErrorMessage)
		if len(m) > 255 {
			m = m[:255]
		}
		if m != "" {
			errMsgPtr = &m
		}
	}

	stream := 0
	if in.IsStream {
		stream = 1
	}
	reqBytes := in.RequestBytes
	if reqBytes < 0 {
		reqBytes = 0
	}
	respBytes := in.ResponseBytes
	if respBytes < 0 {
		respBytes = 0
	}

	_, err := s.db.ExecContext(ctx, `
UPDATE usage_events
SET endpoint=?, method=?, status_code=?, latency_ms=?, first_token_latency_ms=?, error_class=?, error_message=?,
    upstream_channel_id=?, upstream_endpoint_id=?, upstream_credential_id=?,
    is_stream=?, request_bytes=?, response_bytes=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, endpointAny, methodAny, statusCode, latencyMS, firstTokenLatencyMS, errClassPtr, errMsgPtr,
		in.UpstreamChannelID, in.UpstreamEndpointID, in.UpstreamCredID,
		stream, reqBytes, respBytes, in.UsageEventID)
	if err != nil {
		return fmt.Errorf("更新 usage_event 明细失败: %w", err)
	}

	return nil
}

func (s *Store) VoidUsage(ctx context.Context, usageEventID int64) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE usage_events
SET state=?, committed_usd=0, updated_at=CURRENT_TIMESTAMP
WHERE id=? AND state=?
`, UsageStateVoid, usageEventID, UsageStateReserved)
	if err != nil {
		return fmt.Errorf("作废 usage_event 失败: %w", err)
	}
	_, _ = res.RowsAffected()
	return nil
}

func (s *Store) ExpireReservedUsage(ctx context.Context, now time.Time) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 先处理按量计费的过期预留：退回余额并标记过期。
	nPayg, err := s.expireReservedUsageRefundBalances(ctx, tx, now)
	if err != nil {
		return 0, err
	}

	// 再处理剩余预留（订阅等）：仅标记过期。
	res, err := tx.ExecContext(ctx, `
UPDATE usage_events
SET state=?, committed_usd=0, updated_at=CURRENT_TIMESTAMP
WHERE state=? AND reserve_expires_at < ?
`, UsageStateExpired, UsageStateReserved, now)
	if err != nil {
		return 0, fmt.Errorf("过期清理 usage_events 失败: %w", err)
	}
	n2, _ := res.RowsAffected()

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交事务失败: %w", err)
	}
	return nPayg + n2, nil
}

type UsageSumInput struct {
	UserID int64
	Since  time.Time
}

func (s *Store) SumCommittedUSD(ctx context.Context, in UsageSumInput) (decimal.Decimal, error) {
	var sum decimal.NullDecimal
	err := s.db.QueryRowContext(ctx, `
SELECT SUM(committed_usd)
FROM usage_events
WHERE state=? AND user_id=? AND time >= ?
`, UsageStateCommitted, in.UserID, in.Since).Scan(&sum)
	if err != nil {
		return decimal.Zero, fmt.Errorf("汇总用量失败: %w", err)
	}
	if !sum.Valid {
		return decimal.Zero, nil
	}
	return sum.Decimal.Truncate(USDScale), nil
}

type UsageSumWithReservedInput struct {
	UserID int64
	Since  time.Time
	Now    time.Time
}

func (s *Store) SumCommittedAndReservedUSD(ctx context.Context, in UsageSumWithReservedInput) (committedUSD decimal.Decimal, reservedUSD decimal.Decimal, err error) {
	var committedSum decimal.NullDecimal
	var reservedSum decimal.NullDecimal
	err = s.db.QueryRowContext(ctx, `
SELECT
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END) AS committed_sum,
  SUM(CASE WHEN state=? AND reserve_expires_at >= ? THEN reserved_usd ELSE 0 END) AS reserved_sum
FROM usage_events
WHERE user_id=? AND time >= ? AND (state=? OR state=?)
	`, UsageStateCommitted, UsageStateReserved, in.Now, in.UserID, in.Since, UsageStateCommitted, UsageStateReserved).Scan(&committedSum, &reservedSum)
	if err != nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("汇总用量失败: %w", err)
	}
	if committedSum.Valid {
		committedUSD = committedSum.Decimal.Truncate(USDScale)
	}
	if reservedSum.Valid {
		reservedUSD = reservedSum.Decimal.Truncate(USDScale)
	}
	return committedUSD, reservedUSD, nil
}

type UsageSumWithReservedBySubscriptionInput struct {
	UserID         int64
	SubscriptionID int64
	Since          time.Time
	Now            time.Time
}

func (s *Store) SumCommittedAndReservedUSDBySubscription(ctx context.Context, in UsageSumWithReservedBySubscriptionInput) (committedUSD decimal.Decimal, reservedUSD decimal.Decimal, err error) {
	var committedSum decimal.NullDecimal
	var reservedSum decimal.NullDecimal
	err = s.db.QueryRowContext(ctx, `
SELECT
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END) AS committed_sum,
  SUM(CASE WHEN state=? AND reserve_expires_at >= ? THEN reserved_usd ELSE 0 END) AS reserved_sum
FROM usage_events
WHERE user_id=? AND subscription_id=? AND time >= ? AND (state=? OR state=?)
	`, UsageStateCommitted, UsageStateReserved, in.Now, in.UserID, in.SubscriptionID, in.Since, UsageStateCommitted, UsageStateReserved).Scan(&committedSum, &reservedSum)
	if err != nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("汇总用量失败: %w", err)
	}
	if committedSum.Valid {
		committedUSD = committedSum.Decimal.Truncate(USDScale)
	}
	if reservedSum.Valid {
		reservedUSD = reservedSum.Decimal.Truncate(USDScale)
	}
	return committedUSD, reservedUSD, nil
}

type UsageSumWithReservedRangeInput struct {
	UserID int64
	Since  time.Time
	Until  time.Time
	Now    time.Time
}

func (s *Store) SumCommittedAndReservedUSDRange(ctx context.Context, in UsageSumWithReservedRangeInput) (committedUSD decimal.Decimal, reservedUSD decimal.Decimal, err error) {
	var committedSum decimal.NullDecimal
	var reservedSum decimal.NullDecimal
	err = s.db.QueryRowContext(ctx, `
SELECT
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END) AS committed_sum,
  SUM(CASE WHEN state=? AND reserve_expires_at >= ? THEN reserved_usd ELSE 0 END) AS reserved_sum
FROM usage_events
WHERE user_id=? AND time >= ? AND time < ? AND (state=? OR state=?)
	`, UsageStateCommitted, UsageStateReserved, in.Now, in.UserID, in.Since, in.Until, UsageStateCommitted, UsageStateReserved).Scan(&committedSum, &reservedSum)
	if err != nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("汇总用量失败: %w", err)
	}
	if committedSum.Valid {
		committedUSD = committedSum.Decimal.Truncate(USDScale)
	}
	if reservedSum.Valid {
		reservedUSD = reservedSum.Decimal.Truncate(USDScale)
	}
	return committedUSD, reservedUSD, nil
}

func (s *Store) GetUsageEvent(ctx context.Context, id int64) (UsageEvent, error) {
	var e UsageEvent
	var model sql.NullString
	var endpoint sql.NullString
	var method sql.NullString
	var subscriptionID sql.NullInt64
	var inputTokens sql.NullInt64
	var cachedInputTokens sql.NullInt64
	var outputTokens sql.NullInt64
	var cachedOutputTokens sql.NullInt64
	var upstreamChannelID sql.NullInt64
	var upstreamEndpointID sql.NullInt64
	var upstreamCredID sql.NullInt64
	var errClass sql.NullString
	var errMsg sql.NullString
	var multGroupName sql.NullString
	var isStream int
	err := s.db.QueryRowContext(ctx, `
SELECT id, time, request_id, endpoint, method,
       user_id, subscription_id, token_id,
       upstream_channel_id, upstream_endpoint_id, upstream_credential_id,
       state, model,
       input_tokens, cached_input_tokens, output_tokens, cached_output_tokens,
       reserved_usd, committed_usd, price_multiplier, price_multiplier_group, price_multiplier_payment, price_multiplier_group_name, reserve_expires_at,
       status_code, latency_ms, first_token_latency_ms, error_class, error_message,
       is_stream, request_bytes, response_bytes,
       created_at, updated_at
FROM usage_events
WHERE id=?
	`, id).Scan(&e.ID, &e.Time, &e.RequestID, &endpoint, &method,
		&e.UserID, &subscriptionID, &e.TokenID,
		&upstreamChannelID, &upstreamEndpointID, &upstreamCredID,
		&e.State, &model,
		&inputTokens, &cachedInputTokens, &outputTokens, &cachedOutputTokens,
		&e.ReservedUSD, &e.CommittedUSD, &e.PriceMultiplier, &e.PriceMultiplierGroup, &e.PriceMultiplierPayment, &multGroupName, &e.ReserveExpiresAt,
		&e.StatusCode, &e.LatencyMS, &e.FirstTokenLatencyMS, &errClass, &errMsg,
		&isStream, &e.RequestBytes, &e.ResponseBytes,
		&e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UsageEvent{}, sql.ErrNoRows
		}
		return UsageEvent{}, fmt.Errorf("查询 usage_event 失败: %w", err)
	}
	e.ReservedUSD = e.ReservedUSD.Truncate(USDScale)
	e.CommittedUSD = e.CommittedUSD.Truncate(USDScale)
	e.PriceMultiplier = e.PriceMultiplier.Truncate(PriceMultiplierScale)
	e.PriceMultiplierGroup = e.PriceMultiplierGroup.Truncate(PriceMultiplierScale)
	e.PriceMultiplierPayment = e.PriceMultiplierPayment.Truncate(PriceMultiplierScale)
	if multGroupName.Valid {
		v := strings.TrimSpace(multGroupName.String)
		if v != "" {
			e.PriceMultiplierGroupName = &v
		}
	}
	if endpoint.Valid {
		e.Endpoint = &endpoint.String
	}
	if method.Valid {
		e.Method = &method.String
	}
	if model.Valid {
		e.Model = &model.String
	}
	if subscriptionID.Valid {
		e.SubscriptionID = &subscriptionID.Int64
	}
	if inputTokens.Valid {
		e.InputTokens = &inputTokens.Int64
	}
	if cachedInputTokens.Valid {
		e.CachedInputTokens = &cachedInputTokens.Int64
	}
	if outputTokens.Valid {
		e.OutputTokens = &outputTokens.Int64
	}
	if cachedOutputTokens.Valid {
		e.CachedOutputTokens = &cachedOutputTokens.Int64
	}
	if upstreamChannelID.Valid {
		e.UpstreamChannelID = &upstreamChannelID.Int64
	}
	if upstreamEndpointID.Valid {
		e.UpstreamEndpointID = &upstreamEndpointID.Int64
	}
	if upstreamCredID.Valid {
		e.UpstreamCredID = &upstreamCredID.Int64
	}
	if errClass.Valid {
		e.ErrorClass = &errClass.String
	}
	if errMsg.Valid {
		e.ErrorMessage = &errMsg.String
	}
	e.IsStream = isStream != 0
	return e, nil
}

func (s *Store) ListUsageEventsByUser(ctx context.Context, userID int64, limit int, beforeID *int64) ([]UsageEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{userID, UsageStateReserved}
	q := `
SELECT id, time, request_id, endpoint, method,
       user_id, subscription_id, token_id,
       upstream_channel_id, upstream_endpoint_id, upstream_credential_id,
       state, model,
       input_tokens, cached_input_tokens, output_tokens, cached_output_tokens,
       reserved_usd, committed_usd, price_multiplier, price_multiplier_group, price_multiplier_payment, price_multiplier_group_name, reserve_expires_at,
       status_code, latency_ms, first_token_latency_ms, error_class, error_message,
       is_stream, request_bytes, response_bytes,
       created_at, updated_at
FROM usage_events
WHERE user_id=? AND state<>?
`
	if beforeID != nil && *beforeID > 0 {
		q += " AND id < ?\n"
		args = append(args, *beforeID)
	}
	q += "ORDER BY id DESC\nLIMIT ?\n"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("查询 usage_events 失败: %w", err)
	}
	defer rows.Close()

	var out []UsageEvent
	for rows.Next() {
		var e UsageEvent
		var model sql.NullString
		var endpoint sql.NullString
		var method sql.NullString
		var subscriptionID sql.NullInt64
		var inputTokens sql.NullInt64
		var cachedInputTokens sql.NullInt64
		var outputTokens sql.NullInt64
		var cachedOutputTokens sql.NullInt64
		var upstreamChannelID sql.NullInt64
		var upstreamEndpointID sql.NullInt64
		var upstreamCredID sql.NullInt64
		var errClass sql.NullString
		var errMsg sql.NullString
		var multGroupName sql.NullString
		var isStream int
		if err := rows.Scan(&e.ID, &e.Time, &e.RequestID, &endpoint, &method,
			&e.UserID, &subscriptionID, &e.TokenID,
			&upstreamChannelID, &upstreamEndpointID, &upstreamCredID,
			&e.State, &model,
			&inputTokens, &cachedInputTokens, &outputTokens, &cachedOutputTokens,
			&e.ReservedUSD, &e.CommittedUSD, &e.PriceMultiplier, &e.PriceMultiplierGroup, &e.PriceMultiplierPayment, &multGroupName, &e.ReserveExpiresAt,
			&e.StatusCode, &e.LatencyMS, &e.FirstTokenLatencyMS, &errClass, &errMsg,
			&isStream, &e.RequestBytes, &e.ResponseBytes,
			&e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 usage_events 失败: %w", err)
		}
		e.ReservedUSD = e.ReservedUSD.Truncate(USDScale)
		e.CommittedUSD = e.CommittedUSD.Truncate(USDScale)
		e.PriceMultiplier = e.PriceMultiplier.Truncate(PriceMultiplierScale)
		e.PriceMultiplierGroup = e.PriceMultiplierGroup.Truncate(PriceMultiplierScale)
		e.PriceMultiplierPayment = e.PriceMultiplierPayment.Truncate(PriceMultiplierScale)
		if multGroupName.Valid {
			v := strings.TrimSpace(multGroupName.String)
			if v != "" {
				e.PriceMultiplierGroupName = &v
			}
		}
		if endpoint.Valid {
			e.Endpoint = &endpoint.String
		}
		if method.Valid {
			e.Method = &method.String
		}
		if model.Valid {
			e.Model = &model.String
		}
		if subscriptionID.Valid {
			e.SubscriptionID = &subscriptionID.Int64
		}
		if inputTokens.Valid {
			e.InputTokens = &inputTokens.Int64
		}
		if cachedInputTokens.Valid {
			e.CachedInputTokens = &cachedInputTokens.Int64
		}
		if outputTokens.Valid {
			e.OutputTokens = &outputTokens.Int64
		}
		if cachedOutputTokens.Valid {
			e.CachedOutputTokens = &cachedOutputTokens.Int64
		}
		if upstreamChannelID.Valid {
			e.UpstreamChannelID = &upstreamChannelID.Int64
		}
		if upstreamEndpointID.Valid {
			e.UpstreamEndpointID = &upstreamEndpointID.Int64
		}
		if upstreamCredID.Valid {
			e.UpstreamCredID = &upstreamCredID.Int64
		}
		if errClass.Valid {
			e.ErrorClass = &errClass.String
		}
		if errMsg.Valid {
			e.ErrorMessage = &errMsg.String
		}
		e.IsStream = isStream != 0
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 usage_events 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListUsageEventsByUserRange(ctx context.Context, userID int64, since, until time.Time, limit int, beforeID, afterID *int64) ([]UsageEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if beforeID != nil && afterID != nil {
		return nil, errors.New("before_id 与 after_id 不能同时使用")
	}
	args := []any{userID, since, until, UsageStateReserved}
	q := `
SELECT id, time, request_id, endpoint, method,
       user_id, subscription_id, token_id,
       upstream_channel_id, upstream_endpoint_id, upstream_credential_id,
       state, model,
       input_tokens, cached_input_tokens, output_tokens, cached_output_tokens,
       reserved_usd, committed_usd, price_multiplier, price_multiplier_group, price_multiplier_payment, price_multiplier_group_name, reserve_expires_at,
       status_code, latency_ms, first_token_latency_ms, error_class, error_message,
       is_stream, request_bytes, response_bytes,
       created_at, updated_at
FROM usage_events
WHERE user_id=? AND time >= ? AND time < ? AND state<>?
`
	if beforeID != nil && *beforeID > 0 {
		q += " AND id < ?\n"
		args = append(args, *beforeID)
	}
	if afterID != nil && *afterID > 0 {
		q += " AND id > ?\n"
		args = append(args, *afterID)
		q += "ORDER BY id ASC\nLIMIT ?\n"
	} else {
		q += "ORDER BY id DESC\nLIMIT ?\n"
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("查询 usage_events 失败: %w", err)
	}
	defer rows.Close()

	var out []UsageEvent
	for rows.Next() {
		var e UsageEvent
		var endpoint sql.NullString
		var method sql.NullString
		var model sql.NullString
		var subscriptionID sql.NullInt64
		var inputTokens sql.NullInt64
		var cachedInputTokens sql.NullInt64
		var outputTokens sql.NullInt64
		var cachedOutputTokens sql.NullInt64
		var upstreamChannelID sql.NullInt64
		var upstreamEndpointID sql.NullInt64
		var upstreamCredID sql.NullInt64
		var errClass sql.NullString
		var errMsg sql.NullString
		var multGroupName sql.NullString
		var isStream int
		if err := rows.Scan(&e.ID, &e.Time, &e.RequestID, &endpoint, &method,
			&e.UserID, &subscriptionID, &e.TokenID,
			&upstreamChannelID, &upstreamEndpointID, &upstreamCredID,
			&e.State, &model,
			&inputTokens, &cachedInputTokens, &outputTokens, &cachedOutputTokens,
			&e.ReservedUSD, &e.CommittedUSD, &e.PriceMultiplier, &e.PriceMultiplierGroup, &e.PriceMultiplierPayment, &multGroupName, &e.ReserveExpiresAt,
			&e.StatusCode, &e.LatencyMS, &e.FirstTokenLatencyMS, &errClass, &errMsg,
			&isStream, &e.RequestBytes, &e.ResponseBytes,
			&e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 usage_events 失败: %w", err)
		}
		e.ReservedUSD = e.ReservedUSD.Truncate(USDScale)
		e.CommittedUSD = e.CommittedUSD.Truncate(USDScale)
		e.PriceMultiplier = e.PriceMultiplier.Truncate(PriceMultiplierScale)
		e.PriceMultiplierGroup = e.PriceMultiplierGroup.Truncate(PriceMultiplierScale)
		e.PriceMultiplierPayment = e.PriceMultiplierPayment.Truncate(PriceMultiplierScale)
		if multGroupName.Valid {
			v := strings.TrimSpace(multGroupName.String)
			if v != "" {
				e.PriceMultiplierGroupName = &v
			}
		}
		if endpoint.Valid {
			e.Endpoint = &endpoint.String
		}
		if method.Valid {
			e.Method = &method.String
		}
		if model.Valid {
			e.Model = &model.String
		}
		if subscriptionID.Valid {
			e.SubscriptionID = &subscriptionID.Int64
		}
		if inputTokens.Valid {
			e.InputTokens = &inputTokens.Int64
		}
		if cachedInputTokens.Valid {
			e.CachedInputTokens = &cachedInputTokens.Int64
		}
		if outputTokens.Valid {
			e.OutputTokens = &outputTokens.Int64
		}
		if cachedOutputTokens.Valid {
			e.CachedOutputTokens = &cachedOutputTokens.Int64
		}
		if upstreamChannelID.Valid {
			e.UpstreamChannelID = &upstreamChannelID.Int64
		}
		if upstreamEndpointID.Valid {
			e.UpstreamEndpointID = &upstreamEndpointID.Int64
		}
		if upstreamCredID.Valid {
			e.UpstreamCredID = &upstreamCredID.Int64
		}
		if errClass.Valid {
			e.ErrorClass = &errClass.String
		}
		if errMsg.Valid {
			e.ErrorMessage = &errMsg.String
		}
		e.IsStream = isStream != 0
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 usage_events 失败: %w", err)
	}
	if afterID != nil {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, nil
}

type UsageEventWithUser struct {
	Event     UsageEvent
	UserEmail string
}

func (s *Store) ListUsageEventsWithUserRange(ctx context.Context, since, until time.Time, limit int, beforeID, afterID *int64) ([]UsageEventWithUser, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if beforeID != nil && afterID != nil {
		return nil, errors.New("before_id 与 after_id 不能同时使用")
	}
	args := []any{since, until, UsageStateReserved}
	q := `
SELECT ue.id, ue.time, ue.request_id, ue.endpoint, ue.method,
       ue.user_id, ue.subscription_id, ue.token_id,
       ue.upstream_channel_id, ue.upstream_endpoint_id, ue.upstream_credential_id,
       ue.state, ue.model,
       ue.input_tokens, ue.cached_input_tokens, ue.output_tokens, ue.cached_output_tokens,
       ue.reserved_usd, ue.committed_usd, ue.reserve_expires_at,
       ue.status_code, ue.latency_ms, ue.first_token_latency_ms, ue.error_class, ue.error_message,
       ue.is_stream, ue.request_bytes, ue.response_bytes,
       ue.created_at, ue.updated_at,
       u.email
FROM usage_events ue
JOIN users u ON u.id=ue.user_id
WHERE ue.time >= ? AND ue.time < ? AND ue.state<>?
`
	if beforeID != nil && *beforeID > 0 {
		q += " AND ue.id < ?\n"
		args = append(args, *beforeID)
	}
	if afterID != nil && *afterID > 0 {
		q += " AND ue.id > ?\n"
		args = append(args, *afterID)
		q += "ORDER BY ue.id ASC\nLIMIT ?\n"
	} else {
		q += "ORDER BY ue.id DESC\nLIMIT ?\n"
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("查询 usage_events 失败: %w", err)
	}
	defer rows.Close()

	var out []UsageEventWithUser
	for rows.Next() {
		var e UsageEvent
		var endpoint sql.NullString
		var method sql.NullString
		var model sql.NullString
		var subscriptionID sql.NullInt64
		var inputTokens sql.NullInt64
		var cachedInputTokens sql.NullInt64
		var outputTokens sql.NullInt64
		var cachedOutputTokens sql.NullInt64
		var upstreamChannelID sql.NullInt64
		var upstreamEndpointID sql.NullInt64
		var upstreamCredID sql.NullInt64
		var errClass sql.NullString
		var errMsg sql.NullString
		var isStream int
		var email string
		if err := rows.Scan(&e.ID, &e.Time, &e.RequestID, &endpoint, &method,
			&e.UserID, &subscriptionID, &e.TokenID,
			&upstreamChannelID, &upstreamEndpointID, &upstreamCredID,
			&e.State, &model,
			&inputTokens, &cachedInputTokens, &outputTokens, &cachedOutputTokens,
			&e.ReservedUSD, &e.CommittedUSD, &e.ReserveExpiresAt,
			&e.StatusCode, &e.LatencyMS, &e.FirstTokenLatencyMS, &errClass, &errMsg,
			&isStream, &e.RequestBytes, &e.ResponseBytes,
			&e.CreatedAt, &e.UpdatedAt,
			&email); err != nil {
			return nil, fmt.Errorf("扫描 usage_events 失败: %w", err)
		}
		e.ReservedUSD = e.ReservedUSD.Truncate(USDScale)
		e.CommittedUSD = e.CommittedUSD.Truncate(USDScale)
		if endpoint.Valid {
			e.Endpoint = &endpoint.String
		}
		if method.Valid {
			e.Method = &method.String
		}
		if model.Valid {
			e.Model = &model.String
		}
		if subscriptionID.Valid {
			e.SubscriptionID = &subscriptionID.Int64
		}
		if inputTokens.Valid {
			e.InputTokens = &inputTokens.Int64
		}
		if cachedInputTokens.Valid {
			e.CachedInputTokens = &cachedInputTokens.Int64
		}
		if outputTokens.Valid {
			e.OutputTokens = &outputTokens.Int64
		}
		if cachedOutputTokens.Valid {
			e.CachedOutputTokens = &cachedOutputTokens.Int64
		}
		if upstreamChannelID.Valid {
			e.UpstreamChannelID = &upstreamChannelID.Int64
		}
		if upstreamEndpointID.Valid {
			e.UpstreamEndpointID = &upstreamEndpointID.Int64
		}
		if upstreamCredID.Valid {
			e.UpstreamCredID = &upstreamCredID.Int64
		}
		if errClass.Valid {
			e.ErrorClass = &errClass.String
		}
		if errMsg.Valid {
			e.ErrorMessage = &errMsg.String
		}
		e.IsStream = isStream != 0

		out = append(out, UsageEventWithUser{
			Event:     e,
			UserEmail: email,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 usage_events 失败: %w", err)
	}
	if afterID != nil {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, nil
}

type UsageSumAllWithReservedInput struct {
	Since time.Time
	Now   time.Time
}

func (s *Store) SumCommittedAndReservedUSDAll(ctx context.Context, in UsageSumAllWithReservedInput) (committedUSD decimal.Decimal, reservedUSD decimal.Decimal, err error) {
	var committedSum decimal.NullDecimal
	var reservedSum decimal.NullDecimal
	err = s.db.QueryRowContext(ctx, `
SELECT
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END) AS committed_sum,
  SUM(CASE WHEN state=? AND reserve_expires_at >= ? THEN reserved_usd ELSE 0 END) AS reserved_sum
FROM usage_events
WHERE time >= ? AND (state=? OR state=?)
`, UsageStateCommitted, UsageStateReserved, in.Now, in.Since, UsageStateCommitted, UsageStateReserved).Scan(&committedSum, &reservedSum)
	if err != nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("汇总用量失败: %w", err)
	}
	if committedSum.Valid {
		committedUSD = committedSum.Decimal.Truncate(USDScale)
	}
	if reservedSum.Valid {
		reservedUSD = reservedSum.Decimal.Truncate(USDScale)
	}
	return committedUSD, reservedUSD, nil
}

type UsageSumAllWithReservedRangeInput struct {
	Since time.Time
	Until time.Time
	Now   time.Time
}

func (s *Store) SumCommittedAndReservedUSDAllRange(ctx context.Context, in UsageSumAllWithReservedRangeInput) (committedUSD decimal.Decimal, reservedUSD decimal.Decimal, err error) {
	var committedSum decimal.NullDecimal
	var reservedSum decimal.NullDecimal
	err = s.db.QueryRowContext(ctx, `
SELECT
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END) AS committed_sum,
  SUM(CASE WHEN state=? AND reserve_expires_at >= ? THEN reserved_usd ELSE 0 END) AS reserved_sum
FROM usage_events
WHERE time >= ? AND time < ? AND (state=? OR state=?)
`, UsageStateCommitted, UsageStateReserved, in.Now, in.Since, in.Until, UsageStateCommitted, UsageStateReserved).Scan(&committedSum, &reservedSum)
	if err != nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("汇总用量失败: %w", err)
	}
	if committedSum.Valid {
		committedUSD = committedSum.Decimal.Truncate(USDScale)
	}
	if reservedSum.Valid {
		reservedUSD = reservedSum.Decimal.Truncate(USDScale)
	}
	return committedUSD, reservedUSD, nil
}

type UsageUserSum struct {
	UserID       int64
	Email        string
	Role         string
	Status       int
	CommittedUSD decimal.Decimal
	ReservedUSD  decimal.Decimal
}

type UsageTopUsersInput struct {
	Since time.Time
	Until time.Time
	Now   time.Time
	Limit int
}

func (s *Store) ListUsageTopUsers(ctx context.Context, in UsageTopUsersInput) ([]UsageUserSum, error) {
	if in.Limit <= 0 || in.Limit > 200 {
		in.Limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT u.id, u.email, u.role, u.status, x.committed_sum, x.reserved_sum
FROM (
  SELECT user_id,
         SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END) AS committed_sum,
         SUM(CASE WHEN state=? AND reserve_expires_at >= ? THEN reserved_usd ELSE 0 END) AS reserved_sum
  FROM usage_events
  WHERE time >= ? AND time < ? AND (state=? OR state=?)
  GROUP BY user_id
) x
JOIN users u ON u.id=x.user_id
ORDER BY x.committed_sum DESC
LIMIT ?
`, UsageStateCommitted, UsageStateReserved, in.Now, in.Since, in.Until, UsageStateCommitted, UsageStateReserved, in.Limit)
	if err != nil {
		return nil, fmt.Errorf("查询用户用量汇总失败: %w", err)
	}
	defer rows.Close()

	var out []UsageUserSum
	for rows.Next() {
		var row UsageUserSum
		var committedSum decimal.NullDecimal
		var reservedSum decimal.NullDecimal
		if err := rows.Scan(&row.UserID, &row.Email, &row.Role, &row.Status, &committedSum, &reservedSum); err != nil {
			return nil, fmt.Errorf("扫描用户用量汇总失败: %w", err)
		}
		if committedSum.Valid {
			row.CommittedUSD = committedSum.Decimal.Truncate(USDScale)
		}
		if reservedSum.Valid {
			row.ReservedUSD = reservedSum.Decimal.Truncate(USDScale)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历用户用量汇总失败: %w", err)
	}
	return out, nil
}

func computeOutputTokensPerSecond(outputTokens int64, decodeLatencyMS int64) float64 {
	if outputTokens <= 0 || decodeLatencyMS <= 0 {
		return 0
	}
	return float64(outputTokens) * 1000 / float64(decodeLatencyMS)
}

type GlobalUsageStats struct {
	Requests           int64
	Tokens             int64
	InputTokens        int64
	OutputTokens       int64
	CachedInputTokens  int64
	CachedOutputTokens int64
	CacheRatio         float64
	FirstTokenSamples  int64
	AvgFirstTokenMS    float64
	OutputTokensPerSec float64
	CostUSD            decimal.Decimal
}

func (s *Store) GetGlobalUsageStats(ctx context.Context, since time.Time) (GlobalUsageStats, error) {
	var stats GlobalUsageStats
	var committedUSD decimal.NullDecimal
	var inputTokens sql.NullInt64
	var outputTokens sql.NullInt64
	var cachedInputTokens sql.NullInt64
	var cachedOutputTokens sql.NullInt64
	var firstTokenLatencySum sql.NullInt64
	var firstTokenSamples sql.NullInt64
	var decodeLatencyMS sql.NullInt64

	// 统计请求数
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1)
FROM usage_events
WHERE time >= ?
`, since).Scan(&stats.Requests)
	if err != nil {
		return GlobalUsageStats{}, fmt.Errorf("统计请求数失败: %w", err)
	}

	// 统计 Token 和费用
	err = s.db.QueryRowContext(ctx, `
SELECT
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END),
  SUM(input_tokens),
  SUM(output_tokens),
  SUM(cached_input_tokens),
  SUM(cached_output_tokens),
  SUM(CASE WHEN first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END),
  SUM(CASE WHEN first_token_latency_ms > 0 THEN 1 ELSE 0 END),
  SUM(CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END)
FROM usage_events
WHERE time >= ?
`, UsageStateCommitted, since).Scan(&committedUSD, &inputTokens, &outputTokens, &cachedInputTokens, &cachedOutputTokens, &firstTokenLatencySum, &firstTokenSamples, &decodeLatencyMS)
	if err != nil {
		return GlobalUsageStats{}, fmt.Errorf("统计用量失败: %w", err)
	}

	if committedUSD.Valid {
		stats.CostUSD = committedUSD.Decimal.Truncate(USDScale)
	}
	if inputTokens.Valid {
		stats.InputTokens = inputTokens.Int64
	}
	if outputTokens.Valid {
		stats.OutputTokens = outputTokens.Int64
	}
	if cachedInputTokens.Valid {
		stats.CachedInputTokens = cachedInputTokens.Int64
	}
	if cachedOutputTokens.Valid {
		stats.CachedOutputTokens = cachedOutputTokens.Int64
	}
	if firstTokenSamples.Valid {
		stats.FirstTokenSamples = firstTokenSamples.Int64
	}
	stats.Tokens = stats.InputTokens + stats.OutputTokens
	if stats.Tokens > 0 {
		stats.CacheRatio = float64(stats.CachedInputTokens+stats.CachedOutputTokens) / float64(stats.Tokens)
	}
	if firstTokenLatencySum.Valid && stats.FirstTokenSamples > 0 {
		stats.AvgFirstTokenMS = float64(firstTokenLatencySum.Int64) / float64(stats.FirstTokenSamples)
	}
	if decodeLatencyMS.Valid {
		stats.OutputTokensPerSec = computeOutputTokensPerSecond(stats.OutputTokens, decodeLatencyMS.Int64)
	}

	return stats, nil
}

func (s *Store) GetGlobalUsageStatsRange(ctx context.Context, since, until time.Time) (GlobalUsageStats, error) {
	var stats GlobalUsageStats
	var committedUSD decimal.NullDecimal
	var inputTokens sql.NullInt64
	var outputTokens sql.NullInt64
	var cachedInputTokens sql.NullInt64
	var cachedOutputTokens sql.NullInt64
	var firstTokenLatencySum sql.NullInt64
	var firstTokenSamples sql.NullInt64
	var decodeLatencyMS sql.NullInt64

	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1)
FROM usage_events
WHERE time >= ? AND time < ?
`, since, until).Scan(&stats.Requests)
	if err != nil {
		return GlobalUsageStats{}, fmt.Errorf("统计请求数失败: %w", err)
	}

	err = s.db.QueryRowContext(ctx, `
SELECT
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END),
  SUM(input_tokens),
  SUM(output_tokens),
  SUM(cached_input_tokens),
  SUM(cached_output_tokens),
  SUM(CASE WHEN first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END),
  SUM(CASE WHEN first_token_latency_ms > 0 THEN 1 ELSE 0 END),
  SUM(CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END)
FROM usage_events
WHERE time >= ? AND time < ?
`, UsageStateCommitted, since, until).Scan(&committedUSD, &inputTokens, &outputTokens, &cachedInputTokens, &cachedOutputTokens, &firstTokenLatencySum, &firstTokenSamples, &decodeLatencyMS)
	if err != nil {
		return GlobalUsageStats{}, fmt.Errorf("统计用量失败: %w", err)
	}

	if committedUSD.Valid {
		stats.CostUSD = committedUSD.Decimal.Truncate(USDScale)
	}
	if inputTokens.Valid {
		stats.InputTokens = inputTokens.Int64
	}
	if outputTokens.Valid {
		stats.OutputTokens = outputTokens.Int64
	}
	if cachedInputTokens.Valid {
		stats.CachedInputTokens = cachedInputTokens.Int64
	}
	if cachedOutputTokens.Valid {
		stats.CachedOutputTokens = cachedOutputTokens.Int64
	}
	if firstTokenSamples.Valid {
		stats.FirstTokenSamples = firstTokenSamples.Int64
	}
	stats.Tokens = stats.InputTokens + stats.OutputTokens
	if stats.Tokens > 0 {
		stats.CacheRatio = float64(stats.CachedInputTokens+stats.CachedOutputTokens) / float64(stats.Tokens)
	}
	if firstTokenLatencySum.Valid && stats.FirstTokenSamples > 0 {
		stats.AvgFirstTokenMS = float64(firstTokenLatencySum.Int64) / float64(stats.FirstTokenSamples)
	}
	if decodeLatencyMS.Valid {
		stats.OutputTokensPerSec = computeOutputTokensPerSecond(stats.OutputTokens, decodeLatencyMS.Int64)
	}

	return stats, nil
}

type ChannelUsageStats struct {
	ChannelID          int64
	Tokens             int64
	InputTokens        int64
	OutputTokens       int64
	CachedInputTokens  int64
	CachedOutputTokens int64
	CacheRatio         float64
	FirstTokenSamples  int64
	AvgFirstTokenMS    float64
	OutputTokensPerSec float64
	CommittedUSD       decimal.Decimal
}

type CredentialUsageStats struct {
	CredentialID int64
	Requests     int64
	Success      int64
	Failure      int64
	InputTokens  int64
	OutputTokens int64
	LastSeenAt   time.Time
}

func (s *Store) GetUsageStatsByChannelRange(ctx context.Context, since, until time.Time) ([]ChannelUsageStats, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  upstream_channel_id,
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END),
  SUM(input_tokens),
  SUM(output_tokens),
  SUM(cached_input_tokens),
  SUM(cached_output_tokens),
  SUM(CASE WHEN first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END),
  SUM(CASE WHEN first_token_latency_ms > 0 THEN 1 ELSE 0 END),
  SUM(CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END)
FROM usage_events
WHERE upstream_channel_id IS NOT NULL AND time >= ? AND time < ?
GROUP BY upstream_channel_id
`, UsageStateCommitted, since, until)
	if err != nil {
		return nil, fmt.Errorf("按渠道统计用量失败: %w", err)
	}
	defer rows.Close()

	var out []ChannelUsageStats
	for rows.Next() {
		var row ChannelUsageStats
		var committedUSD decimal.NullDecimal
		var inputTokens sql.NullInt64
		var outputTokens sql.NullInt64
		var cachedInputTokens sql.NullInt64
		var cachedOutputTokens sql.NullInt64
		var firstTokenLatencySum sql.NullInt64
		var firstTokenSamples sql.NullInt64
		var decodeLatencyMS sql.NullInt64
		if err := rows.Scan(&row.ChannelID, &committedUSD, &inputTokens, &outputTokens, &cachedInputTokens, &cachedOutputTokens, &firstTokenLatencySum, &firstTokenSamples, &decodeLatencyMS); err != nil {
			return nil, fmt.Errorf("扫描渠道用量失败: %w", err)
		}
		if committedUSD.Valid {
			row.CommittedUSD = committedUSD.Decimal.Truncate(USDScale)
		}
		if inputTokens.Valid {
			row.InputTokens = inputTokens.Int64
		}
		if outputTokens.Valid {
			row.OutputTokens = outputTokens.Int64
		}
		if cachedInputTokens.Valid {
			row.CachedInputTokens = cachedInputTokens.Int64
		}
		if cachedOutputTokens.Valid {
			row.CachedOutputTokens = cachedOutputTokens.Int64
		}
		if firstTokenSamples.Valid {
			row.FirstTokenSamples = firstTokenSamples.Int64
		}
		row.Tokens = row.InputTokens + row.OutputTokens
		if row.Tokens > 0 {
			row.CacheRatio = float64(row.CachedInputTokens+row.CachedOutputTokens) / float64(row.Tokens)
		}
		if firstTokenLatencySum.Valid && row.FirstTokenSamples > 0 {
			row.AvgFirstTokenMS = float64(firstTokenLatencySum.Int64) / float64(row.FirstTokenSamples)
		}
		if decodeLatencyMS.Valid {
			row.OutputTokensPerSec = computeOutputTokensPerSecond(row.OutputTokens, decodeLatencyMS.Int64)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历渠道用量失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetUsageStatsByCredentialForChannelRange(ctx context.Context, channelID int64, since, until time.Time) ([]CredentialUsageStats, error) {
	if channelID <= 0 {
		return nil, fmt.Errorf("channelID 不合法")
	}

	lastSeenExpr := "MAX(UNIX_TIMESTAMP(time))"
	if s.dialect == DialectSQLite {
		// SQLite: time 通常以 `YYYY-MM-DD HH:MM:SS` 存储，strftime('%s', time) 返回 epoch seconds（TEXT）。
		lastSeenExpr = "MAX(CAST(strftime('%s', time) AS INTEGER))"
	}

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT
  upstream_credential_id,
  COUNT(1),
  SUM(CASE WHEN status_code >= 200 AND status_code < 300 THEN 1 ELSE 0 END),
  SUM(CASE WHEN status_code >= 200 AND status_code < 300 THEN 0 ELSE 1 END),
  SUM(CASE WHEN input_tokens IS NOT NULL THEN input_tokens ELSE 0 END),
  SUM(CASE WHEN output_tokens IS NOT NULL THEN output_tokens ELSE 0 END),
  %s
FROM usage_events
WHERE upstream_channel_id=? AND upstream_credential_id IS NOT NULL AND time >= ? AND time < ?
GROUP BY upstream_credential_id
ORDER BY COUNT(1) DESC, upstream_credential_id DESC
`, lastSeenExpr), channelID, since, until)
	if err != nil {
		return nil, fmt.Errorf("按凭证统计用量失败: %w", err)
	}
	defer rows.Close()

	var out []CredentialUsageStats
	for rows.Next() {
		var row CredentialUsageStats
		var success sql.NullInt64
		var failure sql.NullInt64
		var inputTokens sql.NullInt64
		var outputTokens sql.NullInt64
		var lastSeenUnix sql.NullInt64
		if err := rows.Scan(&row.CredentialID, &row.Requests, &success, &failure, &inputTokens, &outputTokens, &lastSeenUnix); err != nil {
			return nil, fmt.Errorf("扫描凭证用量失败: %w", err)
		}
		if success.Valid {
			row.Success = success.Int64
		}
		if failure.Valid {
			row.Failure = failure.Int64
		}
		if inputTokens.Valid {
			row.InputTokens = inputTokens.Int64
		}
		if outputTokens.Valid {
			row.OutputTokens = outputTokens.Int64
		}
		if lastSeenUnix.Valid && lastSeenUnix.Int64 > 0 {
			row.LastSeenAt = time.Unix(lastSeenUnix.Int64, 0).UTC()
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历凭证用量失败: %w", err)
	}
	return out, nil
}

type UsageTokenStats struct {
	Requests           int64
	Tokens             int64
	InputTokens        int64
	OutputTokens       int64
	CachedInputTokens  int64
	CachedOutputTokens int64
	CacheRatio         float64
}

type ModelUsageStats struct {
	Model        string
	Requests     int64
	Tokens       int64
	CommittedUSD decimal.Decimal
}

func (s *Store) GetUsageStatsByModelRange(ctx context.Context, userID int64, since, until time.Time) ([]ModelUsageStats, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  model,
  COUNT(1),
  SUM(input_tokens + output_tokens),
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END)
FROM usage_events
WHERE user_id=? AND time >= ? AND time < ? AND model IS NOT NULL
GROUP BY model
ORDER BY SUM(committed_usd) DESC
`, UsageStateCommitted, userID, since, until)
	if err != nil {
		return nil, fmt.Errorf("按模型统计失败: %w", err)
	}
	defer rows.Close()

	var out []ModelUsageStats
	for rows.Next() {
		var row ModelUsageStats
		var tokens sql.NullInt64
		var cost decimal.NullDecimal
		if err := rows.Scan(&row.Model, &row.Requests, &tokens, &cost); err != nil {
			return nil, err
		}
		if tokens.Valid {
			row.Tokens = tokens.Int64
		}
		if cost.Valid {
			row.CommittedUSD = cost.Decimal.Truncate(USDScale)
		}
		out = append(out, row)
	}
	return out, nil
}

type TimeSeriesUsageStats struct {
	Time         time.Time
	Requests     int64
	Tokens       int64
	CommittedUSD decimal.Decimal
}

type ChannelTimeSeriesUsageStats struct {
	Time               time.Time
	Requests           int64
	Tokens             int64
	CommittedUSD       decimal.Decimal
	CacheRatio         float64
	FirstTokenSamples  int64
	AvgFirstTokenMS    float64
	OutputTokensPerSec float64
}

func (s *Store) GetUsageTimeSeriesRange(ctx context.Context, userID int64, since, until time.Time) ([]TimeSeriesUsageStats, error) {
	query := `
SELECT
  DATE_FORMAT(time, '%Y-%m-%d %H:00:00') as hr,
  COUNT(1),
  SUM(input_tokens + output_tokens),
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END)
FROM usage_events
WHERE user_id=? AND time >= ? AND time < ?
GROUP BY hr
ORDER BY hr ASC
`
	if s.dialect == DialectSQLite {
		query = `
SELECT
  STRFTIME('%Y-%m-%d %H:00:00', time) as hr,
  COUNT(1),
  SUM(input_tokens + output_tokens),
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END)
FROM usage_events
WHERE user_id=? AND time >= ? AND time < ?
GROUP BY hr
ORDER BY hr ASC
`
	}

	rows, err := s.db.QueryContext(ctx, query, UsageStateCommitted, userID, since, until)
	if err != nil {
		return nil, fmt.Errorf("查询时间序列失败: %w", err)
	}
	defer rows.Close()

	var out []TimeSeriesUsageStats
	for rows.Next() {
		var row TimeSeriesUsageStats
		var hr string
		var tokens sql.NullInt64
		var cost decimal.NullDecimal
		if err := rows.Scan(&hr, &row.Requests, &tokens, &cost); err != nil {
			return nil, err
		}
		row.Time, _ = time.Parse("2006-01-02 15:04:05", hr)
		if tokens.Valid {
			row.Tokens = tokens.Int64
		}
		if cost.Valid {
			row.CommittedUSD = cost.Decimal.Truncate(USDScale)
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *Store) GetUserUsageTimeSeriesRange(ctx context.Context, userID int64, since, until time.Time, granularity string) ([]ChannelTimeSeriesUsageStats, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("userID 不合法")
	}
	bucketExprMySQL := "DATE_FORMAT(time, '%Y-%m-%d %H:00:00')"
	bucketExprSQLite := "STRFTIME('%Y-%m-%d %H:00:00', time)"
	switch granularity {
	case "", "hour":
		granularity = "hour"
	case "day":
		bucketExprMySQL = "DATE_FORMAT(time, '%Y-%m-%d 00:00:00')"
		bucketExprSQLite = "STRFTIME('%Y-%m-%d 00:00:00', time)"
	default:
		return nil, fmt.Errorf("granularity 不合法")
	}
	query := `
SELECT
  ` + bucketExprMySQL + ` as hr,
  COUNT(1),
  SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0)),
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END),
  SUM(COALESCE(cached_input_tokens, 0) + COALESCE(cached_output_tokens, 0)),
  SUM(CASE WHEN first_token_latency_ms IS NOT NULL AND first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END),
  SUM(CASE WHEN first_token_latency_ms IS NOT NULL AND first_token_latency_ms > 0 THEN 1 ELSE 0 END),
  SUM(COALESCE(output_tokens, 0)),
  SUM(CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END)
FROM usage_events
WHERE user_id=? AND time >= ? AND time < ?
GROUP BY hr
ORDER BY hr ASC
`
	if s.dialect == DialectSQLite {
		query = `
SELECT
  ` + bucketExprSQLite + ` as hr,
  COUNT(1),
  SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0)),
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END),
  SUM(COALESCE(cached_input_tokens, 0) + COALESCE(cached_output_tokens, 0)),
  SUM(CASE WHEN first_token_latency_ms IS NOT NULL AND first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END),
  SUM(CASE WHEN first_token_latency_ms IS NOT NULL AND first_token_latency_ms > 0 THEN 1 ELSE 0 END),
  SUM(COALESCE(output_tokens, 0)),
  SUM(CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END)
FROM usage_events
WHERE user_id=? AND time >= ? AND time < ?
GROUP BY hr
ORDER BY hr ASC
`
	}

	rows, err := s.db.QueryContext(ctx, query, UsageStateCommitted, userID, since, until)
	if err != nil {
		return nil, fmt.Errorf("查询用户时间序列失败: %w", err)
	}
	defer rows.Close()

	var out []ChannelTimeSeriesUsageStats
	for rows.Next() {
		var row ChannelTimeSeriesUsageStats
		var hr string
		var tokens sql.NullInt64
		var committedUSD decimal.NullDecimal
		var cachedTokens sql.NullInt64
		var firstTokenLatencySum sql.NullInt64
		var firstTokenSamples sql.NullInt64
		var outputTokens sql.NullInt64
		var decodeLatencyMS sql.NullInt64
		if err := rows.Scan(&hr, &row.Requests, &tokens, &committedUSD, &cachedTokens, &firstTokenLatencySum, &firstTokenSamples, &outputTokens, &decodeLatencyMS); err != nil {
			return nil, fmt.Errorf("扫描用户时间序列失败: %w", err)
		}
		row.Time, _ = time.Parse("2006-01-02 15:04:05", hr)
		if tokens.Valid {
			row.Tokens = tokens.Int64
		}
		if committedUSD.Valid {
			row.CommittedUSD = committedUSD.Decimal.Truncate(USDScale)
		}
		if cachedTokens.Valid && row.Tokens > 0 {
			row.CacheRatio = float64(cachedTokens.Int64) / float64(row.Tokens)
		}
		if firstTokenSamples.Valid {
			row.FirstTokenSamples = firstTokenSamples.Int64
		}
		if firstTokenLatencySum.Valid && row.FirstTokenSamples > 0 {
			row.AvgFirstTokenMS = float64(firstTokenLatencySum.Int64) / float64(row.FirstTokenSamples)
		}
		if outputTokens.Valid && decodeLatencyMS.Valid {
			row.OutputTokensPerSec = computeOutputTokensPerSecond(outputTokens.Int64, decodeLatencyMS.Int64)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历用户时间序列失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetGlobalUsageTimeSeriesRange(ctx context.Context, since, until time.Time, granularity string) ([]ChannelTimeSeriesUsageStats, error) {
	bucketExprMySQL := "DATE_FORMAT(time, '%Y-%m-%d %H:00:00')"
	bucketExprSQLite := "STRFTIME('%Y-%m-%d %H:00:00', time)"
	switch granularity {
	case "", "hour":
		granularity = "hour"
	case "day":
		bucketExprMySQL = "DATE_FORMAT(time, '%Y-%m-%d 00:00:00')"
		bucketExprSQLite = "STRFTIME('%Y-%m-%d 00:00:00', time)"
	default:
		return nil, fmt.Errorf("granularity 不合法")
	}
	query := `
SELECT
  ` + bucketExprMySQL + ` as hr,
  COUNT(1),
  SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0)),
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END),
  SUM(COALESCE(cached_input_tokens, 0) + COALESCE(cached_output_tokens, 0)),
  SUM(CASE WHEN first_token_latency_ms IS NOT NULL AND first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END),
  SUM(CASE WHEN first_token_latency_ms IS NOT NULL AND first_token_latency_ms > 0 THEN 1 ELSE 0 END),
  SUM(COALESCE(output_tokens, 0)),
  SUM(CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END)
FROM usage_events
WHERE time >= ? AND time < ?
GROUP BY hr
ORDER BY hr ASC
`
	if s.dialect == DialectSQLite {
		query = `
SELECT
  ` + bucketExprSQLite + ` as hr,
  COUNT(1),
  SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0)),
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END),
  SUM(COALESCE(cached_input_tokens, 0) + COALESCE(cached_output_tokens, 0)),
  SUM(CASE WHEN first_token_latency_ms IS NOT NULL AND first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END),
  SUM(CASE WHEN first_token_latency_ms IS NOT NULL AND first_token_latency_ms > 0 THEN 1 ELSE 0 END),
  SUM(COALESCE(output_tokens, 0)),
  SUM(CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END)
FROM usage_events
WHERE time >= ? AND time < ?
GROUP BY hr
ORDER BY hr ASC
`
	}

	rows, err := s.db.QueryContext(ctx, query, UsageStateCommitted, since, until)
	if err != nil {
		return nil, fmt.Errorf("查询全站时间序列失败: %w", err)
	}
	defer rows.Close()

	var out []ChannelTimeSeriesUsageStats
	for rows.Next() {
		var row ChannelTimeSeriesUsageStats
		var hr string
		var tokens sql.NullInt64
		var committedUSD decimal.NullDecimal
		var cachedTokens sql.NullInt64
		var firstTokenLatencySum sql.NullInt64
		var firstTokenSamples sql.NullInt64
		var outputTokens sql.NullInt64
		var decodeLatencyMS sql.NullInt64
		if err := rows.Scan(&hr, &row.Requests, &tokens, &committedUSD, &cachedTokens, &firstTokenLatencySum, &firstTokenSamples, &outputTokens, &decodeLatencyMS); err != nil {
			return nil, fmt.Errorf("扫描全站时间序列失败: %w", err)
		}
		row.Time, _ = time.Parse("2006-01-02 15:04:05", hr)
		if tokens.Valid {
			row.Tokens = tokens.Int64
		}
		if committedUSD.Valid {
			row.CommittedUSD = committedUSD.Decimal.Truncate(USDScale)
		}
		if cachedTokens.Valid && row.Tokens > 0 {
			row.CacheRatio = float64(cachedTokens.Int64) / float64(row.Tokens)
		}
		if firstTokenSamples.Valid {
			row.FirstTokenSamples = firstTokenSamples.Int64
		}
		if firstTokenLatencySum.Valid && row.FirstTokenSamples > 0 {
			row.AvgFirstTokenMS = float64(firstTokenLatencySum.Int64) / float64(row.FirstTokenSamples)
		}
		if outputTokens.Valid && decodeLatencyMS.Valid {
			row.OutputTokensPerSec = computeOutputTokensPerSecond(outputTokens.Int64, decodeLatencyMS.Int64)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历全站时间序列失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetChannelUsageTimeSeriesRange(ctx context.Context, channelID int64, since, until time.Time, granularity string) ([]ChannelTimeSeriesUsageStats, error) {
	if channelID <= 0 {
		return nil, fmt.Errorf("channelID 不合法")
	}
	bucketExprMySQL := "DATE_FORMAT(time, '%Y-%m-%d %H:00:00')"
	bucketExprSQLite := "STRFTIME('%Y-%m-%d %H:00:00', time)"
	switch granularity {
	case "", "hour":
		granularity = "hour"
	case "day":
		bucketExprMySQL = "DATE_FORMAT(time, '%Y-%m-%d 00:00:00')"
		bucketExprSQLite = "STRFTIME('%Y-%m-%d 00:00:00', time)"
	default:
		return nil, fmt.Errorf("granularity 不合法")
	}
	query := `
SELECT
  ` + bucketExprMySQL + ` as hr,
  COUNT(1),
  SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0)),
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END),
  SUM(COALESCE(cached_input_tokens, 0) + COALESCE(cached_output_tokens, 0)),
  SUM(CASE WHEN first_token_latency_ms IS NOT NULL AND first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END),
  SUM(CASE WHEN first_token_latency_ms IS NOT NULL AND first_token_latency_ms > 0 THEN 1 ELSE 0 END),
  SUM(COALESCE(output_tokens, 0)),
  SUM(CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END)
FROM usage_events
WHERE upstream_channel_id=? AND time >= ? AND time < ?
GROUP BY hr
ORDER BY hr ASC
`
	if s.dialect == DialectSQLite {
		query = `
SELECT
  ` + bucketExprSQLite + ` as hr,
  COUNT(1),
  SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0)),
  SUM(CASE WHEN state=? THEN committed_usd ELSE 0 END),
  SUM(COALESCE(cached_input_tokens, 0) + COALESCE(cached_output_tokens, 0)),
  SUM(CASE WHEN first_token_latency_ms IS NOT NULL AND first_token_latency_ms > 0 THEN first_token_latency_ms ELSE 0 END),
  SUM(CASE WHEN first_token_latency_ms IS NOT NULL AND first_token_latency_ms > 0 THEN 1 ELSE 0 END),
  SUM(COALESCE(output_tokens, 0)),
  SUM(CASE WHEN latency_ms > first_token_latency_ms THEN latency_ms - first_token_latency_ms ELSE 0 END)
FROM usage_events
WHERE upstream_channel_id=? AND time >= ? AND time < ?
GROUP BY hr
ORDER BY hr ASC
`
	}

	rows, err := s.db.QueryContext(ctx, query, UsageStateCommitted, channelID, since, until)
	if err != nil {
		return nil, fmt.Errorf("查询渠道时间序列失败: %w", err)
	}
	defer rows.Close()

	var out []ChannelTimeSeriesUsageStats
	for rows.Next() {
		var row ChannelTimeSeriesUsageStats
		var hr string
		var tokens sql.NullInt64
		var committedUSD decimal.NullDecimal
		var cachedTokens sql.NullInt64
		var firstTokenLatencySum sql.NullInt64
		var firstTokenSamples sql.NullInt64
		var outputTokens sql.NullInt64
		var decodeLatencyMS sql.NullInt64
		if err := rows.Scan(&hr, &row.Requests, &tokens, &committedUSD, &cachedTokens, &firstTokenLatencySum, &firstTokenSamples, &outputTokens, &decodeLatencyMS); err != nil {
			return nil, fmt.Errorf("扫描渠道时间序列失败: %w", err)
		}
		row.Time, _ = time.Parse("2006-01-02 15:04:05", hr)
		if tokens.Valid {
			row.Tokens = tokens.Int64
		}
		if committedUSD.Valid {
			row.CommittedUSD = committedUSD.Decimal.Truncate(USDScale)
		}
		if cachedTokens.Valid && row.Tokens > 0 {
			row.CacheRatio = float64(cachedTokens.Int64) / float64(row.Tokens)
		}
		if firstTokenSamples.Valid {
			row.FirstTokenSamples = firstTokenSamples.Int64
		}
		if firstTokenLatencySum.Valid && row.FirstTokenSamples > 0 {
			row.AvgFirstTokenMS = float64(firstTokenLatencySum.Int64) / float64(row.FirstTokenSamples)
		}
		if outputTokens.Valid && decodeLatencyMS.Valid {
			row.OutputTokensPerSec = computeOutputTokensPerSecond(outputTokens.Int64, decodeLatencyMS.Int64)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历渠道时间序列失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetUsageTokenStatsByUser(ctx context.Context, userID int64, since time.Time) (UsageTokenStats, error) {
	var stats UsageTokenStats
	var inputTokens sql.NullInt64
	var outputTokens sql.NullInt64
	var cachedInputTokens sql.NullInt64
	var cachedOutputTokens sql.NullInt64

	err := s.db.QueryRowContext(ctx, `
SELECT
  COUNT(1),
  SUM(input_tokens),
  SUM(output_tokens),
  SUM(cached_input_tokens),
  SUM(cached_output_tokens)
FROM usage_events
WHERE user_id=? AND time >= ?
`, userID, since).Scan(&stats.Requests, &inputTokens, &outputTokens, &cachedInputTokens, &cachedOutputTokens)
	if err != nil {
		return UsageTokenStats{}, fmt.Errorf("统计用量失败: %w", err)
	}
	if inputTokens.Valid {
		stats.InputTokens = inputTokens.Int64
	}
	if outputTokens.Valid {
		stats.OutputTokens = outputTokens.Int64
	}
	if cachedInputTokens.Valid {
		stats.CachedInputTokens = cachedInputTokens.Int64
	}
	if cachedOutputTokens.Valid {
		stats.CachedOutputTokens = cachedOutputTokens.Int64
	}
	stats.Tokens = stats.InputTokens + stats.OutputTokens
	if stats.Tokens > 0 {
		stats.CacheRatio = float64(stats.CachedInputTokens+stats.CachedOutputTokens) / float64(stats.Tokens)
	}
	return stats, nil
}

func (s *Store) GetUsageTokenStatsByUserRange(ctx context.Context, userID int64, since, until time.Time) (UsageTokenStats, error) {
	var stats UsageTokenStats
	var inputTokens sql.NullInt64
	var outputTokens sql.NullInt64
	var cachedInputTokens sql.NullInt64
	var cachedOutputTokens sql.NullInt64

	err := s.db.QueryRowContext(ctx, `
SELECT
  COUNT(1),
  SUM(input_tokens),
  SUM(output_tokens),
  SUM(cached_input_tokens),
  SUM(cached_output_tokens)
FROM usage_events
WHERE user_id=? AND time >= ? AND time < ?
`, userID, since, until).Scan(&stats.Requests, &inputTokens, &outputTokens, &cachedInputTokens, &cachedOutputTokens)
	if err != nil {
		return UsageTokenStats{}, fmt.Errorf("统计用量失败: %w", err)
	}
	if inputTokens.Valid {
		stats.InputTokens = inputTokens.Int64
	}
	if outputTokens.Valid {
		stats.OutputTokens = outputTokens.Int64
	}
	if cachedInputTokens.Valid {
		stats.CachedInputTokens = cachedInputTokens.Int64
	}
	if cachedOutputTokens.Valid {
		stats.CachedOutputTokens = cachedOutputTokens.Int64
	}
	stats.Tokens = stats.InputTokens + stats.OutputTokens
	if stats.Tokens > 0 {
		stats.CacheRatio = float64(stats.CachedInputTokens+stats.CachedOutputTokens) / float64(stats.Tokens)
	}
	return stats, nil
}
