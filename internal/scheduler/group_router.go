package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"realms/internal/store"
)

var errGroupExhausted = errors.New("group exhausted")

type ChannelGroupStore interface {
	GetChannelGroupByName(ctx context.Context, name string) (store.ChannelGroup, error)
	GetChannelGroupByID(ctx context.Context, id int64) (store.ChannelGroup, error)
	ListChannelGroupMembers(ctx context.Context, parentGroupID int64) ([]store.ChannelGroupMemberDetail, error)
}

type GroupRouter struct {
	st    ChannelGroupStore
	sched *Scheduler

	cons        Constraints
	userID      int64
	routeKeyHash string

	cursors          map[int64]*groupCursor
	activePath       map[int64]struct{}
	excludedChannels map[int64]struct{}
}

type groupCursor struct {
	group store.ChannelGroup

	loaded  bool
	members []store.ChannelGroupMemberDetail

	// attemptsUsed 统计该组已经返回给上层的“叶子选择”次数；达到 MaxAttempts 后视为耗尽。
	attemptsUsed int
}

func NewGroupRouter(st ChannelGroupStore, sched *Scheduler, userID int64, routeKeyHash string, cons Constraints) *GroupRouter {
	return &GroupRouter{
		st:              st,
		sched:           sched,
		userID:          userID,
		routeKeyHash:    routeKeyHash,
		cons:            cons,
		cursors:         make(map[int64]*groupCursor),
		activePath:      make(map[int64]struct{}),
		excludedChannels: make(map[int64]struct{}),
	}
}

func (r *GroupRouter) Next(ctx context.Context) (Selection, error) {
	if r.st == nil || r.sched == nil {
		return Selection{}, errors.New("group router 未配置")
	}

	root, err := r.st.GetChannelGroupByName(ctx, store.DefaultGroupName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Selection{}, errors.New("default 分组不存在")
		}
		return Selection{}, err
	}
	if root.Status != 1 {
		return Selection{}, errors.New("default 分组已禁用")
	}

	sel, err := r.nextFromGroup(ctx, root.ID)
	if err != nil {
		if errors.Is(err, errGroupExhausted) {
			return Selection{}, errors.New("上游不可用")
		}
		return Selection{}, err
	}
	return sel, nil
}

func (r *GroupRouter) cursorForGroup(ctx context.Context, groupID int64) (*groupCursor, error) {
	if groupID == 0 {
		return nil, errors.New("groupID 不能为空")
	}
	if c, ok := r.cursors[groupID]; ok {
		return c, nil
	}
	g, err := r.st.GetChannelGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	c := &groupCursor{group: g}
	r.cursors[groupID] = c
	return c, nil
}

func (r *GroupRouter) loadMembers(ctx context.Context, c *groupCursor) error {
	if c.loaded {
		return nil
	}
	ms, err := r.st.ListChannelGroupMembers(ctx, c.group.ID)
	if err != nil {
		return err
	}
	c.members = ms
	c.loaded = true
	return nil
}

func (r *GroupRouter) nextFromGroup(ctx context.Context, groupID int64) (Selection, error) {
	if groupID == 0 {
		return Selection{}, errGroupExhausted
	}
	c, err := r.cursorForGroup(ctx, groupID)
	if err != nil {
		return Selection{}, err
	}
	if c.group.Status != 1 {
		return Selection{}, errGroupExhausted
	}

	maxAttempts := c.group.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if c.attemptsUsed >= maxAttempts {
		return Selection{}, errGroupExhausted
	}

	cands := make(map[int64]channelCandidate)
	if err := r.collectCandidates(ctx, groupID, cands); err != nil {
		return Selection{}, err
	}
	if len(cands) == 0 {
		return Selection{}, errGroupExhausted
	}
	forcedID := int64(0)
	if r.sched != nil {
		if id, _, ok := r.sched.ForcedChannel(time.Now()); ok {
			forcedID = id
		}
	}
	ordered := sortCandidates(cands, forcedID, func(channelID int64) int {
		if r.sched == nil || r.sched.state == nil {
			return 0
		}
		return r.sched.state.ChannelFailScore(channelID)
	})

	for _, cand := range ordered {
		cons := r.cons
		cons.RequireChannelID = cand.ChannelID
		sel, err := r.sched.SelectWithConstraints(ctx, r.userID, r.routeKeyHash, cons)
		if err != nil {
			r.excludedChannels[cand.ChannelID] = struct{}{}
			continue
		}
		c.attemptsUsed++
		if cand.SourceGroupID != 0 && cand.SourceGroupID != groupID {
			if sc, err := r.cursorForGroup(ctx, cand.SourceGroupID); err == nil {
				sc.attemptsUsed++
			}
		}
		return sel, nil
	}
	return Selection{}, errGroupExhausted
}

type channelCandidate struct {
	ChannelID     int64
	SourceGroupID int64
	Priority      int
	Promotion     bool
}

func (r *GroupRouter) collectCandidates(ctx context.Context, groupID int64, out map[int64]channelCandidate) error {
	if groupID == 0 {
		return nil
	}
	if _, ok := r.activePath[groupID]; ok {
		return nil
	}
	r.activePath[groupID] = struct{}{}
	defer delete(r.activePath, groupID)

	c, err := r.cursorForGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if c.group.Status != 1 {
		return nil
	}
	maxAttempts := c.group.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if c.attemptsUsed >= maxAttempts {
		return nil
	}
	if err := r.loadMembers(ctx, c); err != nil {
		return err
	}

	for _, m := range c.members {
		// 成员类型校验：必须且只能存在一种 member。
		if m.MemberGroupID != nil && m.MemberChannelID != nil {
			continue
		}
		if m.MemberGroupID == nil && m.MemberChannelID == nil {
			continue
		}

		if m.MemberGroupID != nil {
			if !r.groupAllowed(m.MemberGroupName) {
				continue
			}
			if err := r.collectCandidates(ctx, *m.MemberGroupID, out); err != nil {
				return err
			}
			continue
		}

		chID := *m.MemberChannelID
		if _, ok := r.excludedChannels[chID]; ok {
			continue
		}
		if !r.channelAllowed(m.MemberChannelType, m.MemberChannelGroups, chID) {
			continue
		}
		cand := channelCandidate{
			ChannelID:     chID,
			SourceGroupID: groupID,
			Priority:      m.Priority,
			Promotion:     m.Promotion,
		}
		if prev, ok := out[chID]; ok {
			if cand.Promotion && !prev.Promotion {
				out[chID] = cand
				continue
			}
			if cand.Promotion == prev.Promotion && cand.Priority > prev.Priority {
				out[chID] = cand
				continue
			}
			continue
		}
		out[chID] = cand
	}
	return nil
}

func sortCandidates(in map[int64]channelCandidate, forcedChannelID int64, failScore func(channelID int64) int) []channelCandidate {
	out := make([]channelCandidate, 0, len(in))
	for _, c := range in {
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if forcedChannelID != 0 {
			if out[i].ChannelID == forcedChannelID && out[j].ChannelID != forcedChannelID {
				return true
			}
			if out[j].ChannelID == forcedChannelID && out[i].ChannelID != forcedChannelID {
				return false
			}
		}
		if out[i].Promotion != out[j].Promotion {
			return out[i].Promotion
		}
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		fi := 0
		fj := 0
		if failScore != nil {
			fi = failScore(out[i].ChannelID)
			fj = failScore(out[j].ChannelID)
		}
		if fi != fj {
			return fi < fj
		}
		return out[i].ChannelID > out[j].ChannelID
	})
	return out
}

func (r *GroupRouter) groupAllowed(name *string) bool {
	// 未设置 allowGroups 时，默认允许（仅用于管理面/内部调用）。
	if r.cons.AllowGroups == nil {
		return true
	}
	if name == nil {
		return false
	}
	n := strings.TrimSpace(*name)
	if n == "" {
		return false
	}
	_, ok := r.cons.AllowGroups[n]
	return ok
}

func (r *GroupRouter) channelAllowed(chType *string, chGroups *string, chID int64) bool {
	if r.cons.RequireChannelType != "" {
		if chType == nil || strings.TrimSpace(*chType) != r.cons.RequireChannelType {
			return false
		}
	}
	if r.cons.AllowChannelIDs != nil {
		if _, ok := r.cons.AllowChannelIDs[chID]; !ok {
			return false
		}
	}
	if r.cons.AllowGroups != nil {
		g := ""
		if chGroups != nil {
			g = *chGroups
		}
		if !channelInAnyGroup(g, r.cons.AllowGroups) {
			return false
		}
	}
	return true
}
