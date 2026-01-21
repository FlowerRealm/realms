package quota

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

var ErrInsufficientBalance = errors.New("余额不足")

// HybridProvider 实现“订阅优先 + 余额兜底”的配额策略：
// - 优先按订阅额度窗口扣费
// - 无订阅或订阅额度不足时，尝试使用余额按量计费
type HybridProvider struct {
	st *store.Store

	sub *SubscriptionProvider

	reserveTTL time.Duration

	paygEnabledDefault bool

	defaultReserveUSD decimal.Decimal
}

func NewHybridProvider(st *store.Store, reserveTTL time.Duration, paygEnabledDefault bool) *HybridProvider {
	if reserveTTL <= 0 {
		reserveTTL = 2 * time.Minute
	}
	return &HybridProvider{
		st:                     st,
		sub:                    NewSubscriptionProvider(st, reserveTTL),
		reserveTTL:             reserveTTL,
		paygEnabledDefault:     paygEnabledDefault,
		defaultReserveUSD:      decimal.NewFromInt(1).Div(decimal.NewFromInt(1000)), // 0.001 USD
	}
}

func (p *HybridProvider) paygEnabled(ctx context.Context) bool {
	v, ok, err := p.st.GetBoolAppSetting(ctx, store.SettingBillingEnablePayAsYouGo)
	if err != nil {
		return p.paygEnabledDefault
	}
	if ok {
		return v
	}
	return p.paygEnabledDefault
}

func (p *HybridProvider) Reserve(ctx context.Context, in ReserveInput) (ReserveResult, error) {
	if p.sub != nil {
		res, err := p.sub.Reserve(ctx, in)
		if err == nil {
			return res, nil
		}
		// 非“无订阅/额度不足”错误，直接返回（避免掩盖真实错误）。
		if !errors.Is(err, ErrSubscriptionRequired) && !errors.Is(err, ErrQuotaExceeded) {
			return ReserveResult{}, err
		}

		// 订阅不可用：尝试按量计费。
		if !p.paygEnabled(ctx) {
			return ReserveResult{}, err
		}
	}

	reservedUSD := decimal.Zero
	if in.Model != nil && in.MaxOutputTokens != nil && *in.MaxOutputTokens > 0 {
		c, err := estimateCostUSD(ctx, p.st, in.Model, nil, nil, nil, nil, in.MaxOutputTokens)
		if err != nil {
			return ReserveResult{}, err
		}
		reservedUSD = c
	}
	if reservedUSD.LessThanOrEqual(decimal.Zero) {
		reservedUSD = p.defaultReserveUSD
	}

	now := time.Now()
	id, err := p.st.ReserveUsageAndDebitBalance(ctx, store.ReserveUsageInput{
		RequestID:         in.RequestID,
		UserID:            in.UserID,
		SubscriptionID:    nil,
		TokenID:           in.TokenID,
		Model:             in.Model,
		ReservedUSD:       reservedUSD,
		ReserveExpiresAt:  now.Add(p.reserveTTL),
	})
	if err != nil {
		if errors.Is(err, store.ErrInsufficientBalance) {
			return ReserveResult{}, ErrInsufficientBalance
		}
		return ReserveResult{}, err
	}
	return ReserveResult{UsageEventID: id}, nil
}

func (p *HybridProvider) Commit(ctx context.Context, in CommitInput) error {
	if in.UsageEventID == 0 {
		return nil
	}
	ev, err := p.st.GetUsageEvent(ctx, in.UsageEventID)
	if err != nil {
		return err
	}

	// 订阅场景交给 SubscriptionProvider：确保与订阅逻辑一致（含分组倍率等规则）。
	if ev.SubscriptionID != nil {
		if p.sub != nil {
			return p.sub.Commit(ctx, in)
		}
	}

	model := in.Model
	if model == nil {
		model = ev.Model
	}
	usd, err := estimateCostUSD(ctx, p.st, model, in.InputTokens, in.CachedInputTokens, in.OutputTokens, in.CachedOutputTokens, nil)
	if err != nil {
		return err
	}
	if usd.Equal(decimal.Zero) {
		usd = ev.ReservedUSD
	}

	if ev.SubscriptionID == nil {
		return p.st.CommitUsageAndRefundBalance(ctx, store.CommitUsageInput{
			UsageEventID:       in.UsageEventID,
			UpstreamChannelID:  in.UpstreamChannelID,
			InputTokens:        in.InputTokens,
			CachedInputTokens:  in.CachedInputTokens,
			OutputTokens:       in.OutputTokens,
			CachedOutputTokens: in.CachedOutputTokens,
			CommittedUSD:       usd,
		})
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

func (p *HybridProvider) Void(ctx context.Context, usageEventID int64) error {
	return p.st.VoidUsageAndRefundBalance(ctx, usageEventID)
}
