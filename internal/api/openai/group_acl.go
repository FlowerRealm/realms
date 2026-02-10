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
	return allowGroups{Order: order, Set: set}
}

func managedModelGroupName(m store.ManagedModel) string {
	return strings.TrimSpace(m.GroupName)
}
