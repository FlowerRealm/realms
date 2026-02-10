package router

import (
	"context"
	"strings"

	"realms/internal/store"
)

type allowedSubgroups struct {
	Order []string
	Set   map[string]struct{}
}

func allowedSubgroupsForMainGroup(ctx context.Context, st *store.Store, mainGroup string) (allowedSubgroups, error) {
	if st == nil {
		return allowedSubgroups{}, nil
	}
	mainGroup = strings.TrimSpace(mainGroup)
	if mainGroup == "" {
		return allowedSubgroups{}, nil
	}
	rows, err := st.ListMainGroupSubgroups(ctx, mainGroup)
	if err != nil {
		return allowedSubgroups{}, err
	}

	set := make(map[string]struct{}, len(rows)+1)
	order := make([]string, 0, len(rows)+1)
	for _, row := range rows {
		name := strings.TrimSpace(row.Subgroup)
		if name == "" {
			continue
		}
		if _, ok := set[name]; ok {
			continue
		}
		set[name] = struct{}{}
		order = append(order, name)
	}
	return allowedSubgroups{Order: order, Set: set}, nil
}
