package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// UserMainGroupAllowsSubgroup returns whether the user's main_group allows selecting the given subgroup.
// Subgroups are implemented as channel_groups names, configured via main_group_subgroups.
func (s *Store) UserMainGroupAllowsSubgroup(ctx context.Context, userID int64, subgroup string) (bool, error) {
	if userID <= 0 {
		return false, errors.New("userID 不能为空")
	}
	subgroup = strings.TrimSpace(subgroup)
	if subgroup == "" {
		return false, errors.New("subgroup 不能为空")
	}
	normSubgroup, err := normalizeGroupName(subgroup)
	if err != nil {
		return false, err
	}

	var mainGroup sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT main_group FROM users WHERE id=? LIMIT 1`, userID).Scan(&mainGroup); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, sql.ErrNoRows
		}
		return false, fmt.Errorf("查询 users.main_group 失败: %w", err)
	}
	mainGroupName := ""
	if mainGroup.Valid {
		mainGroupName = strings.TrimSpace(mainGroup.String)
	}
	if mainGroupName == "" {
		return false, errors.New("用户未配置用户分组")
	}

	rows, err := s.ListMainGroupSubgroups(ctx, mainGroupName)
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	for _, row := range rows {
		if strings.TrimSpace(row.Subgroup) == normSubgroup {
			return true, nil
		}
	}
	return false, nil
}
