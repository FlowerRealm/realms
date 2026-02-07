package quota

import (
	"context"
	"strings"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

type userGroupMultiplierSnapshot struct {
	userMultiplier decimal.Decimal
}

func loadUserGroupMultiplierSnapshot(ctx context.Context, st *store.Store, userID int64) (userGroupMultiplierSnapshot, error) {
	snap := userGroupMultiplierSnapshot{
		userMultiplier: store.DefaultGroupPriceMultiplier,
	}
	if st == nil || userID <= 0 {
		return snap, nil
	}

	groupMultiplierByName := map[string]decimal.Decimal{
		store.DefaultGroupName: store.DefaultGroupPriceMultiplier,
	}
	groups, err := st.ListChannelGroups(ctx)
	if err != nil {
		return userGroupMultiplierSnapshot{}, err
	}
	for _, g := range groups {
		name := normalizeGroupForMultiplier(g.Name)
		if name == "" {
			continue
		}
		mult := g.PriceMultiplier
		if mult.IsNegative() {
			mult = store.DefaultGroupPriceMultiplier
		}
		groupMultiplierByName[name] = mult
	}

	userGroups, err := st.ListUserGroups(ctx, userID)
	if err != nil {
		return userGroupMultiplierSnapshot{}, err
	}
	snap.userMultiplier = stackedMultiplierForGroups(userGroups, groupMultiplierByName)
	return snap, nil
}

func stackedMultiplierForGroups(groupNames []string, byGroup map[string]decimal.Decimal) decimal.Decimal {
	seen := make(map[string]struct{}, len(groupNames)+1)
	seen[store.DefaultGroupName] = struct{}{}

	mult := store.DefaultGroupPriceMultiplier
	for _, raw := range groupNames {
		name := normalizeGroupForMultiplier(raw)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		mult = mult.Mul(groupMultiplierByName(name, byGroup))
	}
	return mult
}

func groupMultiplierByName(name string, byGroup map[string]decimal.Decimal) decimal.Decimal {
	if byGroup == nil {
		return store.DefaultGroupPriceMultiplier
	}
	mult, ok := byGroup[name]
	if !ok || mult.IsNegative() {
		return store.DefaultGroupPriceMultiplier
	}
	return mult
}

func normalizeGroupForMultiplier(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return store.DefaultGroupName
	}
	return name
}
