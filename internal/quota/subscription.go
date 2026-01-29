package quota

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

var (
	ErrSubscriptionRequired = errors.New("订阅未激活")
	ErrQuotaExceeded        = errors.New("订阅额度不足")
)

type SubscriptionProvider struct {
	st *store.Store

	reserveTTL time.Duration
}

func NewSubscriptionProvider(st *store.Store, reserveTTL time.Duration) *SubscriptionProvider {
	if reserveTTL <= 0 {
		reserveTTL = 2 * time.Minute
	}
	return &SubscriptionProvider{
		st:         st,
		reserveTTL: reserveTTL,
	}
}

func (p *SubscriptionProvider) Reserve(ctx context.Context, in ReserveInput) (ReserveResult, error) {
	now := time.Now()
	subs, err := p.st.ListActiveSubscriptionsWithPlans(ctx, in.UserID, now)
	if err != nil {
		return ReserveResult{}, err
	}
	if len(subs) == 0 {
		return ReserveResult{}, ErrSubscriptionRequired
	}

	var baseReservedUSD decimal.Decimal
	if in.Model != nil && ((in.InputTokens != nil && *in.InputTokens > 0) || (in.MaxOutputTokens != nil && *in.MaxOutputTokens > 0)) {
		c, err := estimateCostUSD(ctx, p.st, in.Model, in.InputTokens, nil, nil, nil, in.MaxOutputTokens)
		if err != nil {
			return ReserveResult{}, err
		}
		baseReservedUSD = c
	}

	type win struct {
		dur   time.Duration
		limit decimal.Decimal
	}
	winsFor := func(plan store.SubscriptionPlan) []win {
		return []win{
			{dur: 5 * time.Hour, limit: plan.Limit5HUSD},
			{dur: 24 * time.Hour, limit: plan.Limit1DUSD},
			{dur: 7 * 24 * time.Hour, limit: plan.Limit7DUSD},
			{dur: 30 * 24 * time.Hour, limit: plan.Limit30DUSD},
		}
	}

	var chosen *store.SubscriptionWithPlan
	var chosenReservedUSD decimal.Decimal

	groupMult := make(map[string]decimal.Decimal)
	if baseReservedUSD.GreaterThan(decimal.Zero) {
		gs, err := p.st.ListChannelGroups(ctx)
		if err != nil {
			return ReserveResult{}, err
		}
		groupMult = make(map[string]decimal.Decimal, len(gs)+1)
		for _, g := range gs {
			name := strings.TrimSpace(g.Name)
			if name == "" {
				continue
			}
			groupMult[name] = g.PriceMultiplier
		}
	}

	for i := range subs {
		row := subs[i]
		ok := true
		reservedUSD := baseReservedUSD
		if baseReservedUSD.GreaterThan(decimal.Zero) {
			group := strings.TrimSpace(row.Plan.GroupName)
			if group == "" {
				group = store.DefaultGroupName
			}
			mult := store.DefaultGroupPriceMultiplier
			if v, ok := groupMult[group]; ok {
				mult = v
			}
			usd, err := applyPriceMultiplierUSD(baseReservedUSD, mult)
			if err != nil {
				return ReserveResult{}, err
			}
			reservedUSD = usd
		}
		for _, w := range winsFor(row.Plan) {
			if w.limit.LessThanOrEqual(decimal.Zero) {
				continue
			}
			since := now.Add(-w.dur)
			if row.Subscription.StartAt.After(since) {
				since = row.Subscription.StartAt
			}
			committed, reserved, err := p.st.SumCommittedAndReservedUSDBySubscription(ctx, store.UsageSumWithReservedBySubscriptionInput{
				UserID:         in.UserID,
				SubscriptionID: row.Subscription.ID,
				Since:          since,
				Now:            now,
			})
			if err != nil {
				return ReserveResult{}, err
			}
			if committed.Add(reserved).Add(reservedUSD).GreaterThan(w.limit) {
				ok = false
				break
			}
		}
		if ok {
			chosen = &subs[i]
			chosenReservedUSD = reservedUSD
			break
		}
	}
	if chosen == nil {
		return ReserveResult{}, ErrQuotaExceeded
	}

	id, err := p.st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        in.RequestID,
		UserID:           in.UserID,
		SubscriptionID:   &chosen.Subscription.ID,
		TokenID:          in.TokenID,
		Model:            in.Model,
		ReservedUSD:      chosenReservedUSD,
		ReserveExpiresAt: now.Add(p.reserveTTL),
	})
	if err != nil {
		return ReserveResult{}, err
	}
	return ReserveResult{UsageEventID: id}, nil
}

func (p *SubscriptionProvider) Commit(ctx context.Context, in CommitInput) error {
	if in.UsageEventID == 0 {
		return nil
	}
	ev, err := p.st.GetUsageEvent(ctx, in.UsageEventID)
	if err != nil {
		return err
	}

	model := in.Model
	if model == nil {
		model = ev.Model
	}

	mult := store.DefaultGroupPriceMultiplier
	if ev.SubscriptionID != nil && *ev.SubscriptionID > 0 {
		sub, err := p.st.GetSubscriptionWithPlanByID(ctx, *ev.SubscriptionID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		group := strings.TrimSpace(sub.Plan.GroupName)
		if group == "" {
			group = store.DefaultGroupName
		}
		g, err := p.st.GetChannelGroupByName(ctx, group)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			mult = g.PriceMultiplier
		}
	}

	usd, err := estimateCostUSD(ctx, p.st, model, in.InputTokens, in.CachedInputTokens, in.OutputTokens, in.CachedOutputTokens, nil)
	if err != nil {
		return err
	}
	usd, err = applyPriceMultiplierUSD(usd, mult)
	if err != nil {
		return err
	}
	if usd.Equal(decimal.Zero) {
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

func (p *SubscriptionProvider) Void(ctx context.Context, usageEventID int64) error {
	return p.st.VoidUsage(ctx, usageEventID)
}
