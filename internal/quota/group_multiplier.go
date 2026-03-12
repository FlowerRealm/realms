package quota

import (
	"context"
	"database/sql"
	"strings"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

const routeGroupSeparator = "/"

type tokenGroupMultiplierSnapshot struct {
	maxGroupMultiplier decimal.Decimal
}

type channelGroupLookup interface {
	GetChannelGroupByName(ctx context.Context, name string) (store.ChannelGroup, error)
	GetChannelGroupByID(ctx context.Context, id int64) (store.ChannelGroup, error)
	ListChannelGroupMembers(ctx context.Context, parentGroupID int64) ([]store.ChannelGroupMemberDetail, error)
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

	maxMult, err := maxReachableRouteGroupMultiplier(ctx, st, groups)
	if err != nil {
		return tokenGroupMultiplierSnapshot{}, err
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
	path := normalizeRouteGroupPath(routeGroup)
	if path == "" {
		return store.DefaultGroupPriceMultiplier, nil
	}
	pathPtr := &path
	if st == nil {
		return store.DefaultGroupPriceMultiplier, pathPtr
	}

	mult := store.DefaultGroupPriceMultiplier
	for _, seg := range splitRouteGroupPath(path) {
		g, err := st.GetChannelGroupByName(ctx, seg)
		if err != nil || g.Status != 1 {
			continue
		}
		mult = normalizeMultiplier(mult.Mul(normalizeMultiplier(g.PriceMultiplier)))
	}
	return mult, pathPtr
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

func normalizeRouteGroupPath(routeGroup *string) string {
	if routeGroup == nil {
		return ""
	}
	parts := splitRouteGroupPath(*routeGroup)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, routeGroupSeparator)
}

func splitRouteGroupPath(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, routeGroupSeparator)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func maxReachableRouteGroupMultiplier(ctx context.Context, st channelGroupLookup, groupNames []string) (decimal.Decimal, error) {
	if st == nil || len(groupNames) == 0 {
		return store.DefaultGroupPriceMultiplier, nil
	}
	maxMult := store.DefaultGroupPriceMultiplier
	for _, raw := range groupNames {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		root, err := st.GetChannelGroupByName(ctx, name)
		if err != nil {
			continue
		}
		if root.Status != 1 {
			continue
		}
		groupMax, hasLeaf, err := maxReachableMultiplierFromGroup(ctx, st, root.ID, normalizeMultiplier(root.PriceMultiplier), map[int64]struct{}{})
		if err != nil {
			return decimal.Zero, err
		}
		if !hasLeaf {
			groupMax = normalizeMultiplier(root.PriceMultiplier)
			hasLeaf = true
		}
		if hasLeaf && groupMax.GreaterThan(maxMult) {
			maxMult = groupMax
		}
	}
	return maxMult, nil
}

func maxReachableMultiplierFromGroup(ctx context.Context, st channelGroupLookup, groupID int64, acc decimal.Decimal, active map[int64]struct{}) (decimal.Decimal, bool, error) {
	if st == nil || groupID == 0 {
		return store.DefaultGroupPriceMultiplier, false, nil
	}
	if _, ok := active[groupID]; ok {
		return store.DefaultGroupPriceMultiplier, false, nil
	}
	active[groupID] = struct{}{}
	defer delete(active, groupID)

	members, err := st.ListChannelGroupMembers(ctx, groupID)
	if err != nil {
		return decimal.Zero, false, err
	}
	maxMult := decimal.Zero
	hasLeaf := false
	for _, m := range members {
		if m.MemberGroupID != nil && m.MemberChannelID != nil {
			continue
		}
		if m.MemberGroupID == nil && m.MemberChannelID == nil {
			continue
		}
		if m.MemberGroupID != nil {
			child, err := st.GetChannelGroupByID(ctx, *m.MemberGroupID)
			if err != nil || child.Status != 1 {
				continue
			}
			childAcc := normalizeMultiplier(acc.Mul(normalizeMultiplier(child.PriceMultiplier)))
			childMax, childHasLeaf, err := maxReachableMultiplierFromGroup(ctx, st, child.ID, childAcc, active)
			if err != nil {
				return decimal.Zero, false, err
			}
			if childHasLeaf {
				hasLeaf = true
				if childMax.GreaterThan(maxMult) {
					maxMult = childMax
				}
			}
			continue
		}
		chID := int64(0)
		if m.MemberChannelID != nil {
			chID = *m.MemberChannelID
		}
		if chID <= 0 {
			continue
		}
		hasLeaf = true
		if acc.GreaterThan(maxMult) {
			maxMult = acc
		}
	}
	if !hasLeaf {
		return store.DefaultGroupPriceMultiplier, false, nil
	}
	return normalizeMultiplier(maxMult), true, nil
}
