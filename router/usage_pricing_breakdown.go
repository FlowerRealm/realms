package router

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

type usageEventGroupMultiplierAPI struct {
	GroupName  string          `json:"group_name"`
	Multiplier decimal.Decimal `json:"multiplier"`
}

type usageEventPricingBreakdownAPI struct {
	CostSource    string          `json:"cost_source"`
	CostSourceUSD decimal.Decimal `json:"cost_source_usd"`

	ModelPublicID string `json:"model_public_id,omitempty"`
	ModelFound    bool   `json:"model_found"`

	InputTokensTotal    int64 `json:"input_tokens_total"`
	InputTokensCached   int64 `json:"input_tokens_cached"`
	InputTokensBillable int64 `json:"input_tokens_billable"`

	OutputTokensTotal    int64 `json:"output_tokens_total"`
	OutputTokensCached   int64 `json:"output_tokens_cached"`
	OutputTokensBillable int64 `json:"output_tokens_billable"`

	InputUSDPer1M       decimal.Decimal `json:"input_usd_per_1m"`
	OutputUSDPer1M      decimal.Decimal `json:"output_usd_per_1m"`
	CacheInputUSDPer1M  decimal.Decimal `json:"cache_input_usd_per_1m"`
	CacheOutputUSDPer1M decimal.Decimal `json:"cache_output_usd_per_1m"`

	InputCostUSD       decimal.Decimal `json:"input_cost_usd"`
	OutputCostUSD      decimal.Decimal `json:"output_cost_usd"`
	CacheInputCostUSD  decimal.Decimal `json:"cache_input_cost_usd"`
	CacheOutputCostUSD decimal.Decimal `json:"cache_output_cost_usd"`
	BaseCostUSD        decimal.Decimal `json:"base_cost_usd"`

	UserGroups          []string                       `json:"user_groups"`
	UserGroupFactors    []usageEventGroupMultiplierAPI `json:"user_group_factors"`
	UserMultiplier      decimal.Decimal                `json:"user_multiplier"`
	SubscriptionGroup   string                         `json:"subscription_group,omitempty"`
	EffectiveMultiplier decimal.Decimal                `json:"effective_multiplier"`

	FinalCostUSD      decimal.Decimal `json:"final_cost_usd"`
	DiffFromSourceUSD decimal.Decimal `json:"diff_from_source_usd"`
}

func buildUsageEventPricingBreakdown(ctx context.Context, st *store.Store, ev store.UsageEvent) (usageEventPricingBreakdownAPI, error) {
	out := usageEventPricingBreakdownAPI{
		CostSource:          usageEventCostSource(ev),
		CostSourceUSD:       usageEventCostSourceAmount(ev),
		ModelFound:          false,
		UserGroups:          []string{},
		UserGroupFactors:    []usageEventGroupMultiplierAPI{},
		UserMultiplier:      store.DefaultGroupPriceMultiplier,
		EffectiveMultiplier: store.DefaultGroupPriceMultiplier,
	}
	if ev.Model != nil {
		modelPublicID := strings.TrimSpace(*ev.Model)
		if modelPublicID != "" {
			out.ModelPublicID = modelPublicID
			mm, err := st.GetManagedModelByPublicID(ctx, modelPublicID)
			if err != nil {
				if !errors.Is(err, sql.ErrNoRows) {
					return usageEventPricingBreakdownAPI{}, err
				}
			} else {
				out.ModelFound = true
				out.InputUSDPer1M = mm.InputUSDPer1M.Truncate(store.USDScale)
				out.OutputUSDPer1M = mm.OutputUSDPer1M.Truncate(store.USDScale)
				out.CacheInputUSDPer1M = mm.CacheInputUSDPer1M.Truncate(store.USDScale)
				out.CacheOutputUSDPer1M = mm.CacheOutputUSDPer1M.Truncate(store.USDScale)
			}
		}
	}

	out.InputTokensTotal = usageTokensValue(ev.InputTokens)
	out.InputTokensCached = usageClampCachedTokens(out.InputTokensTotal, usageTokensValue(ev.CachedInputTokens))
	out.InputTokensBillable = out.InputTokensTotal - out.InputTokensCached
	if out.InputTokensBillable < 0 {
		out.InputTokensBillable = 0
	}

	out.OutputTokensTotal = usageTokensValue(ev.OutputTokens)
	out.OutputTokensCached = usageClampCachedTokens(out.OutputTokensTotal, usageTokensValue(ev.CachedOutputTokens))
	out.OutputTokensBillable = out.OutputTokensTotal - out.OutputTokensCached
	if out.OutputTokensBillable < 0 {
		out.OutputTokensBillable = 0
	}

	out.InputCostUSD = usageCostUSD(out.InputTokensBillable, out.InputUSDPer1M)
	out.OutputCostUSD = usageCostUSD(out.OutputTokensBillable, out.OutputUSDPer1M)
	out.CacheInputCostUSD = usageCostUSD(out.InputTokensCached, out.CacheInputUSDPer1M)
	out.CacheOutputCostUSD = usageCostUSD(out.OutputTokensCached, out.CacheOutputUSDPer1M)
	out.BaseCostUSD = out.InputCostUSD.Add(out.OutputCostUSD).Add(out.CacheInputCostUSD).Add(out.CacheOutputCostUSD).Truncate(store.USDScale)

	groupMultiplierByName, err := usageListGroupMultiplierMap(ctx, st)
	if err != nil {
		return usageEventPricingBreakdownAPI{}, err
	}

	userGroups, err := st.ListUserGroups(ctx, ev.UserID)
	if err != nil {
		return usageEventPricingBreakdownAPI{}, err
	}
	userSeen := make(map[string]struct{}, len(userGroups)+1)
	userMultiplier := store.DefaultGroupPriceMultiplier
	for _, raw := range userGroups {
		groupName := usageNormalizeGroupName(raw)
		if groupName == "" {
			continue
		}
		if _, ok := userSeen[groupName]; ok {
			continue
		}
		userSeen[groupName] = struct{}{}
		factor := usageGroupMultiplierByName(groupName, groupMultiplierByName)
		out.UserGroups = append(out.UserGroups, groupName)
		out.UserGroupFactors = append(out.UserGroupFactors, usageEventGroupMultiplierAPI{
			GroupName:  groupName,
			Multiplier: factor,
		})
		userMultiplier = userMultiplier.Mul(factor)
	}
	out.UserMultiplier = userMultiplier.Truncate(store.PriceMultiplierScale)

	effectiveMultiplier := out.UserMultiplier
	if ev.SubscriptionID != nil && *ev.SubscriptionID > 0 {
		sub, err := st.GetSubscriptionWithPlanByID(ctx, *ev.SubscriptionID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return usageEventPricingBreakdownAPI{}, err
			}
		} else {
			groupName := usageNormalizeGroupName(sub.Plan.GroupName)
			out.SubscriptionGroup = groupName
		}
	}

	out.EffectiveMultiplier = effectiveMultiplier.Truncate(store.PriceMultiplierScale)
	out.FinalCostUSD = usageApplyMultiplier(out.BaseCostUSD, out.EffectiveMultiplier)
	out.DiffFromSourceUSD = out.CostSourceUSD.Sub(out.FinalCostUSD).Truncate(store.USDScale)
	return out, nil
}

func usageEventCostSource(ev store.UsageEvent) string {
	switch ev.State {
	case store.UsageStateCommitted:
		return "committed"
	case store.UsageStateReserved:
		return "reserved"
	default:
		return "none"
	}
}

func usageEventCostSourceAmount(ev store.UsageEvent) decimal.Decimal {
	switch ev.State {
	case store.UsageStateCommitted:
		return ev.CommittedUSD.Truncate(store.USDScale)
	case store.UsageStateReserved:
		return ev.ReservedUSD.Truncate(store.USDScale)
	default:
		return decimal.Zero
	}
}

func usageTokensValue(v *int64) int64 {
	if v == nil || *v <= 0 {
		return 0
	}
	return *v
}

func usageClampCachedTokens(total int64, cached int64) int64 {
	if total <= 0 || cached <= 0 {
		return 0
	}
	if cached > total {
		return total
	}
	return cached
}

func usageCostUSD(tokens int64, usdPer1M decimal.Decimal) decimal.Decimal {
	if tokens <= 0 || usdPer1M.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	return decimal.NewFromInt(tokens).Mul(usdPer1M).Div(decimal.NewFromInt(1_000_000)).Truncate(store.USDScale)
}

func usageApplyMultiplier(baseUSD decimal.Decimal, multiplier decimal.Decimal) decimal.Decimal {
	if baseUSD.LessThanOrEqual(decimal.Zero) || multiplier.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	return baseUSD.Mul(multiplier).Truncate(store.USDScale)
}

func usageListGroupMultiplierMap(ctx context.Context, st *store.Store) (map[string]decimal.Decimal, error) {
	groupMultiplierByName := map[string]decimal.Decimal{
		store.DefaultGroupName: store.DefaultGroupPriceMultiplier,
	}
	groups, err := st.ListChannelGroups(ctx)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		groupName := usageNormalizeGroupName(group.Name)
		if groupName == "" {
			continue
		}
		multiplier := group.PriceMultiplier
		if multiplier.IsNegative() {
			multiplier = store.DefaultGroupPriceMultiplier
		}
		groupMultiplierByName[groupName] = multiplier
	}
	return groupMultiplierByName, nil
}

func usageNormalizeGroupName(raw string) string {
	groupName := strings.TrimSpace(raw)
	if groupName == "" {
		return store.DefaultGroupName
	}
	return groupName
}

func usageGroupMultiplierByName(groupName string, multiplierByName map[string]decimal.Decimal) decimal.Decimal {
	if multiplierByName == nil {
		return store.DefaultGroupPriceMultiplier
	}
	multiplier, ok := multiplierByName[groupName]
	if !ok || multiplier.IsNegative() {
		return store.DefaultGroupPriceMultiplier
	}
	return multiplier
}
