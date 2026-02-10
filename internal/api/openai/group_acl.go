package openai

import (
	"strings"

	"realms/internal/auth"
	"realms/internal/store"
)

type allowGroups struct {
	Order []string
	Set   map[string]struct{}
}

func allowGroupsFromPrincipal(p auth.Principal) allowGroups {
	rawOrder := p.Groups
	if len(rawOrder) == 0 {
		rawOrder = []string{store.DefaultGroupName}
	}

	set := make(map[string]struct{}, len(rawOrder))
	order := make([]string, 0, len(rawOrder))
	for _, raw := range rawOrder {
		g := strings.TrimSpace(raw)
		if g == "" {
			continue
		}
		if _, ok := set[g]; ok {
			continue
		}
		set[g] = struct{}{}
		order = append(order, g)
	}
	if len(order) == 0 {
		order = []string{store.DefaultGroupName}
		set = map[string]struct{}{store.DefaultGroupName: {}}
	}
	return allowGroups{Order: order, Set: set}
}

func managedModelGroupName(m store.ManagedModel) string {
	g := strings.TrimSpace(m.GroupName)
	if g == "" {
		return store.DefaultGroupName
	}
	return g
}

