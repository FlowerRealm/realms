// Package quota 定义配额/计费对接点（QuotaProvider），并提供基于 usage_events 的最小实现。
package quota

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

var ErrModelPricingMissing = errors.New("模型不存在，无法计费")

type Provider interface {
	Reserve(ctx context.Context, in ReserveInput) (ReserveResult, error)
	Commit(ctx context.Context, in CommitInput) error
	Void(ctx context.Context, usageEventID int64) error
}

type ReserveInput struct {
	RequestID       string
	UserID          int64
	TokenID         int64
	Model           *string
	InputTokens     *int64
	MaxOutputTokens *int64
}

type ReserveResult struct {
	UsageEventID int64
}

type CommitInput struct {
	UsageEventID       int64
	Model              *string
	UpstreamChannelID  *int64
	InputTokens        *int64
	CachedInputTokens  *int64
	OutputTokens       *int64
	CachedOutputTokens *int64
}

type UsageProvider struct {
	st *store.Store

	reserveTTL time.Duration

	defaultReserveUSD decimal.Decimal
}

func NewUsageProvider(st *store.Store, reserveTTL time.Duration, defaultReserveUSD decimal.Decimal) *UsageProvider {
	if reserveTTL <= 0 {
		reserveTTL = 2 * time.Minute
	}
	return &UsageProvider{
		st:                st,
		reserveTTL:        reserveTTL,
		defaultReserveUSD: defaultReserveUSD,
	}
}

func (p *UsageProvider) Reserve(ctx context.Context, in ReserveInput) (ReserveResult, error) {
	reservedUSD := p.defaultReserveUSD
	if reservedUSD.LessThanOrEqual(decimal.Zero) {
		reservedUSD = decimal.NewFromInt(1).Div(decimal.NewFromInt(1000)) // 0.001 USD
	}
	if in.Model != nil && ((in.InputTokens != nil && *in.InputTokens > 0) || (in.MaxOutputTokens != nil && *in.MaxOutputTokens > 0)) {
		c, err := estimateCostUSD(ctx, p.st, in.Model, in.InputTokens, nil, nil, nil, in.MaxOutputTokens)
		if err != nil {
			return ReserveResult{}, err
		}
		if c.GreaterThan(decimal.Zero) {
			reservedUSD = c
		}
	}
	id, err := p.st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        in.RequestID,
		UserID:           in.UserID,
		SubscriptionID:   nil,
		TokenID:          in.TokenID,
		Model:            in.Model,
		ReservedUSD:      reservedUSD,
		ReserveExpiresAt: time.Now().Add(p.reserveTTL),
	})
	if err != nil {
		return ReserveResult{}, err
	}
	return ReserveResult{UsageEventID: id}, nil
}

func (p *UsageProvider) Commit(ctx context.Context, in CommitInput) error {
	if in.UsageEventID == 0 {
		return nil
	}
	usd, err := estimateCostUSD(ctx, p.st, in.Model, in.InputTokens, in.CachedInputTokens, in.OutputTokens, in.CachedOutputTokens, nil)
	if err != nil {
		return err
	}
	if usd.Equal(decimal.Zero) {
		ev, err := p.st.GetUsageEvent(ctx, in.UsageEventID)
		if err != nil {
			return err
		}
		usd = ev.ReservedUSD
	}
	return p.st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID:       in.UsageEventID,
		UpstreamChannelID:  in.UpstreamChannelID,
		InputTokens:        in.InputTokens,
		CachedInputTokens:  in.CachedInputTokens,
		OutputTokens:       in.OutputTokens,
		CachedOutputTokens: in.CachedOutputTokens,
		CommittedUSD:       usd,
	})
}

func (p *UsageProvider) Void(ctx context.Context, usageEventID int64) error {
	return p.st.VoidUsage(ctx, usageEventID)
}

func applyPriceMultiplierUSD(baseUSD decimal.Decimal, multiplier decimal.Decimal) (decimal.Decimal, error) {
	if baseUSD.Equal(decimal.Zero) {
		return decimal.Zero, nil
	}
	if baseUSD.IsNegative() {
		return decimal.Zero, errors.New("成本计算为负数")
	}
	if multiplier.IsNegative() {
		return decimal.Zero, errors.New("倍率为负数")
	}
	if multiplier.Equal(store.DefaultGroupPriceMultiplier) {
		return baseUSD, nil
	}
	if multiplier.Equal(decimal.Zero) {
		return decimal.Zero, nil
	}
	return baseUSD.Mul(multiplier).Truncate(6), nil
}

func estimateCostUSD(ctx context.Context, st *store.Store, model *string, inputTokens, cachedInputTokens, outputTokens, cachedOutputTokens, maxOutputTokens *int64) (decimal.Decimal, error) {
	if model == nil || *model == "" {
		return decimal.Zero, nil
	}
	if inputTokens == nil && cachedInputTokens == nil && outputTokens == nil && cachedOutputTokens == nil && maxOutputTokens == nil {
		return decimal.Zero, nil
	}

	mm, err := st.GetManagedModelByPublicID(ctx, *model)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return decimal.Zero, ErrModelPricingMissing
		}
		return decimal.Zero, err
	}
	return estimateCostUSDWithPricing(mm.InputUSDPer1M, mm.OutputUSDPer1M, mm.CacheInputUSDPer1M, mm.CacheOutputUSDPer1M, inputTokens, cachedInputTokens, outputTokens, cachedOutputTokens, maxOutputTokens)
}

func estimateCostUSDWithPricing(inUSDPer1M, outUSDPer1M, cacheInUSDPer1M, cacheOutUSDPer1M decimal.Decimal, inputTokens, cachedInputTokens, outputTokens, cachedOutputTokens, maxOutputTokens *int64) (decimal.Decimal, error) {
	var inTok int64
	var cachedInTok int64
	var outTok int64
	var cachedOutTok int64
	if inputTokens != nil {
		inTok = *inputTokens
	}
	if cachedInputTokens != nil {
		cachedInTok = *cachedInputTokens
	}
	if outputTokens != nil {
		outTok = *outputTokens
	} else if maxOutputTokens != nil {
		outTok = *maxOutputTokens
	}
	if cachedOutputTokens != nil {
		cachedOutTok = *cachedOutputTokens
	}

	if inTok < 0 || outTok < 0 || cachedInTok < 0 || cachedOutTok < 0 {
		return decimal.Zero, errors.New("token 统计为负数")
	}
	if cachedInTok > inTok {
		cachedInTok = inTok
	}
	if cachedOutTok > outTok {
		cachedOutTok = outTok
	}

	nonCachedInTok := inTok - cachedInTok
	nonCachedOutTok := outTok - cachedOutTok

	cost := func(tokens int64, usdPer1M decimal.Decimal) (decimal.Decimal, error) {
		if tokens == 0 || usdPer1M.Equal(decimal.Zero) {
			return decimal.Zero, nil
		}
		if tokens < 0 || usdPer1M.IsNegative() {
			return decimal.Zero, errors.New("成本计算参数为负数")
		}
		return decimal.NewFromInt(tokens).Mul(usdPer1M).Div(decimal.NewFromInt(1_000_000)).Truncate(6), nil
	}

	inCost, err := cost(nonCachedInTok, inUSDPer1M)
	if err != nil {
		return decimal.Zero, err
	}
	outCost, err := cost(nonCachedOutTok, outUSDPer1M)
	if err != nil {
		return decimal.Zero, err
	}
	cacheInCost, err := cost(cachedInTok, cacheInUSDPer1M)
	if err != nil {
		return decimal.Zero, err
	}
	cacheOutCost, err := cost(cachedOutTok, cacheOutUSDPer1M)
	if err != nil {
		return decimal.Zero, err
	}

	sum := inCost.Add(outCost).Add(cacheInCost).Add(cacheOutCost)
	if sum.IsNegative() {
		return decimal.Zero, errors.New("成本计算为负数")
	}
	// 与 DB 精度对齐：最终仍截断到 6 位小数。
	return sum.Truncate(6), nil
}
