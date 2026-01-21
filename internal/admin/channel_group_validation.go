package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"realms/internal/store"
)

func validateChannelGroupsSelectable(ctx context.Context, st *store.Store, groupsCSV string) error {
	for _, name := range splitGroups(groupsCSV) {
		g, err := st.GetChannelGroupByName(ctx, name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("分组不存在：%s", name)
			}
			return err
		}
		if g.Status != 1 {
			return fmt.Errorf("分组已禁用：%s", name)
		}
	}
	return nil
}
