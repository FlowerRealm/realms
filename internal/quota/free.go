package quota

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

// FreeProvider 实现“无限用量但仍记录 usage_events”的策略：
// - Reserve 永远成功（不检查订阅/余额）
// - 不做余额扣减与返还（reserved_usd 固定为 0）
// - Commit 尝试估算成本；缺少模型定价时记为 0
type FreeProvider struct {
	st *store.Store

	reserveTTL time.Duration
}

func NewFreeProvider(st *store.Store, reserveTTL time.Duration) *FreeProvider {
	if reserveTTL <= 0 {
		reserveTTL = 2 * time.Minute
	}
	return &FreeProvider{
		st:         st,
		reserveTTL: reserveTTL,
	}
}

func (p *FreeProvider) Reserve(ctx context.Context, in ReserveInput) (ReserveResult, error) {
	if p.st == nil {
		return ReserveResult{}, errors.New("store 为空")
	}
	id, err := p.st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        in.RequestID,
		UserID:           in.UserID,
		SubscriptionID:   nil,
		TokenID:          in.TokenID,
		Model:            in.Model,
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().Add(p.reserveTTL),
	})
	if err != nil {
		return ReserveResult{}, err
	}
	return ReserveResult{UsageEventID: id}, nil
}

func (p *FreeProvider) Commit(ctx context.Context, in CommitInput) error {
	if in.UsageEventID == 0 {
		return nil
	}
	if p.st == nil {
		return errors.New("store 为空")
	}

	usd, err := estimateCostUSD(ctx, p.st, in.Model, in.InputTokens, in.CachedInputTokens, in.OutputTokens, in.CachedOutputTokens, nil)
	if err != nil {
		if errors.Is(err, ErrModelPricingMissing) {
			usd = decimal.Zero
		} else {
			return err
		}
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

func (p *FreeProvider) Void(ctx context.Context, usageEventID int64) error {
	if usageEventID == 0 {
		return nil
	}
	if p.st == nil {
		return errors.New("store 为空")
	}
	return p.st.VoidUsage(ctx, usageEventID)
}
