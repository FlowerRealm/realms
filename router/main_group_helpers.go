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
		return allowedSubgroups{
			Order: []string{store.DefaultGroupName},
			Set:   map[string]struct{}{store.DefaultGroupName: {}},
		}, nil
	}
	mainGroup = strings.TrimSpace(mainGroup)
	if mainGroup == "" {
		mainGroup = store.DefaultGroupName
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
	if len(order) == 0 {
		return allowedSubgroups{
			Order: []string{store.DefaultGroupName},
			Set:   map[string]struct{}{store.DefaultGroupName: {}},
		}, nil
	}
	if _, ok := set[store.DefaultGroupName]; !ok {
		set[store.DefaultGroupName] = struct{}{}
		order = append(order, store.DefaultGroupName)
	}
	return allowedSubgroups{Order: order, Set: set}, nil
}
