package router

import (
	"context"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

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

