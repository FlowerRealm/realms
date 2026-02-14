package quota

import (
	"context"
	"database/sql"
	"strings"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

type tokenGroupMultiplierSnapshot struct {
	maxGroupMultiplier decimal.Decimal
}

func loadTokenGroupMultiplierSnapshot(ctx context.Context, st *store.Store, tokenID int64) (tokenGroupMultiplierSnapshot, error) {
	snap := tokenGroupMultiplierSnapshot{
		maxGroupMultiplier: store.DefaultGroupPriceMultiplier,
	}
	if st == nil || tokenID <= 0 {
		return snap, nil
	}

	groups, err := st.ListEffectiveTokenChannelGroups(ctx, tokenID)
	if err != nil {
		if err == sql.ErrNoRows {
			return snap, nil
		}
		return tokenGroupMultiplierSnapshot{}, err
	}

	maxMult := store.DefaultGroupPriceMultiplier
	for _, raw := range groups {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		g, err := st.GetChannelGroupByName(ctx, name)
		if err != nil {
			continue
		}
		if g.Status != 1 {
			continue
		}
		m := g.PriceMultiplier
		if m.IsNegative() || m.LessThanOrEqual(decimal.Zero) {
			m = store.DefaultGroupPriceMultiplier
		}
		m = m.Truncate(store.PriceMultiplierScale)
		if m.GreaterThan(maxMult) {
			maxMult = m
		}
	}
	snap.maxGroupMultiplier = maxMult
	return snap, nil
}

func normalizeMultiplier(v decimal.Decimal) decimal.Decimal {
	if v.IsNegative() || v.LessThanOrEqual(decimal.Zero) {
		return store.DefaultGroupPriceMultiplier
	}
	return v.Truncate(store.PriceMultiplierScale)
}

func groupMultiplierForRouteGroup(ctx context.Context, st *store.Store, routeGroup *string) (decimal.Decimal, *string) {
	groupName := ""
	if routeGroup != nil {
		groupName = strings.TrimSpace(*routeGroup)
	}
	if groupName == "" {
		return store.DefaultGroupPriceMultiplier, nil
	}
	// 用于落库/展示：始终记录规范化后的 group name。
	groupNamePtr := &groupName

	if st == nil {
		return store.DefaultGroupPriceMultiplier, groupNamePtr
	}
	g, err := st.GetChannelGroupByName(ctx, groupName)
	if err != nil || g.Status != 1 {
		return store.DefaultGroupPriceMultiplier, groupNamePtr
	}
	return normalizeMultiplier(g.PriceMultiplier), groupNamePtr
}

func paygoPriceMultiplier(ctx context.Context, st *store.Store) decimal.Decimal {
	if st == nil {
		return store.DefaultGroupPriceMultiplier
	}
	v, ok, err := st.GetDecimalAppSetting(ctx, store.SettingBillingPayAsYouGoPriceMultiplier)
	if err != nil || !ok {
		return store.DefaultGroupPriceMultiplier
	}
	return normalizeMultiplier(v)
}
