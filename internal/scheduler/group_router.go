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

	cons         Constraints
	userID       int64
	routeKeyHash string

	cursors          map[int64]*groupCursor
	activePath       map[int64]struct{}
	excludedChannels map[int64]struct{}

	lastSelectedChannelID int64
	lastSelectedStreak    int
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
		st:               st,
		sched:            sched,
		userID:           userID,
		routeKeyHash:     routeKeyHash,
		cons:             cons,
		cursors:          make(map[int64]*groupCursor),
		activePath:       make(map[int64]struct{}),
		excludedChannels: make(map[int64]struct{}),
	}
}

func (r *GroupRouter) Next(ctx context.Context) (Selection, error) {
	if r.st == nil || r.sched == nil {
		return Selection{}, errors.New("group router 未配置")
	}

	if len(r.cons.AllowGroupOrder) > 0 {
		sel, err := r.nextFromOrderedGroups(ctx)
		if err != nil {
			if errors.Is(err, errGroupExhausted) {
				return Selection{}, errors.New("上游不可用")
			}
			return Selection{}, err
		}
		return sel, nil
	}
		return Selection{}, errors.New("未指定渠道组")
}

func (r *GroupRouter) nextFromOrderedGroups(ctx context.Context) (Selection, error) {
	if r.st == nil {
		return Selection{}, errGroupExhausted
	}
	for _, raw := range r.cons.AllowGroupOrder {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		g, err := r.st.GetChannelGroupByName(ctx, name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return Selection{}, err
		}
		if g.Status != 1 {
			continue
		}
		sel, err := r.nextFromGroup(ctx, g.ID)
		if err != nil {
			if errors.Is(err, errGroupExhausted) {
				continue
			}
			return Selection{}, err
		}
		sel.RouteGroup = name
		return sel, nil
	}
	return Selection{}, errGroupExhausted
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
	now := time.Now()
	if r.sched != nil && r.sched.state != nil {
		r.sched.state.SweepExpiredChannelBans(now)
	}

	// 指针模式（按组）：当该组指针 pinned=true 时，从指针位置开始按 ring 遍历一圈。
	// 注意：指针不应绕过 AllowGroupOrder，仅在“当前正在尝试的 groupID”作用域内生效。
	if r.sched != nil && r.sched.state != nil && r.sched.groupPointers != nil {
		rec, _ := r.sched.maybeSyncChannelGroupPointerFromStore(ctx, groupID)
		if rec.Pinned {
			ring := buildCandidateRing(cands)
			if len(ring) > 0 {
				startID := rec.ChannelID
				if startID <= 0 {
					startID = ring[0]
				}
				index := make(map[int64]int, len(ring))
				for i, id := range ring {
					index[id] = i
				}
				startIdx, ok := index[startID]
				if !ok {
					startID = ring[0]
					startIdx = 0
					r.sched.setChannelGroupPointer(groupID, startID, rec.Pinned, "invalid")
				}

				// 若指针当前渠道处于 ban，按 ring 向后轮转到下一个未封禁渠道并持久化。
				if r.sched.state.IsChannelBanned(startID, now) && len(ring) > 1 {
					rotatedID, rotatedIdx, rotatedOK := nextUnbannedInRing(ring, startIdx, func(channelID int64) bool {
						return r.sched.state.IsChannelBanned(channelID, now)
					})
					if rotatedOK && rotatedID != startID {
						startID = rotatedID
						startIdx = rotatedIdx
						r.sched.setChannelGroupPointer(groupID, startID, rec.Pinned, "ban")
					}
				}

				const maxConsecutiveSameChannel = 2
				deferredID := int64(0)
				if r.lastSelectedChannelID != 0 && r.lastSelectedStreak >= maxConsecutiveSameChannel && len(ring) > 1 {
					if _, ok := index[r.lastSelectedChannelID]; ok {
						deferredID = r.lastSelectedChannelID
					}
				}

				try := func(chID int64) (Selection, bool) {
					if chID <= 0 {
						return Selection{}, false
					}
					if _, excluded := r.excludedChannels[chID]; excluded {
						return Selection{}, false
					}
					if r.sched.state.IsChannelBanned(chID, now) {
						return Selection{}, false
					}
					cons := r.cons
					cons.RequireChannelID = chID
					sel, err := r.sched.SelectWithConstraints(ctx, r.userID, r.routeKeyHash, cons)
					if err != nil {
						r.excludedChannels[chID] = struct{}{}
						return Selection{}, false
					}

					c.attemptsUsed++
					if cand, ok := cands[chID]; ok {
						if cand.SourceGroupID != 0 && cand.SourceGroupID != groupID {
							if sc, err := r.cursorForGroup(ctx, cand.SourceGroupID); err == nil {
								sc.attemptsUsed++
							}
						}
					}

					if chID == r.lastSelectedChannelID {
						r.lastSelectedStreak++
					} else {
						r.lastSelectedChannelID = chID
						r.lastSelectedStreak = 1
					}

					r.sched.touchChannelGroupPointer(ctx, groupID, chID, "route")
					return sel, true
				}

				// 第 1 轮：跳过 deferredID（若存在），优先尝试其他渠道。
				for step := 0; step < len(ring); step++ {
					chID := ring[(startIdx+step)%len(ring)]
					if deferredID != 0 && chID == deferredID {
						continue
					}
					if sel, ok := try(chID); ok {
						return sel, nil
					}
				}
				// 第 2 轮：最后再尝试 deferredID（避免同渠道无限重试导致 failover 无法前进）。
				if deferredID != 0 {
					if sel, ok := try(deferredID); ok {
						return sel, nil
					}
				}
				return Selection{}, errGroupExhausted
			}
		}
	}
	ordered := sortCandidates(cands, func(channelID int64) bool {
		if r.sched == nil || r.sched.state == nil {
			return false
		}
		return r.sched.state.IsChannelProbePending(channelID, now)
	}, func(channelID int64) int {
		if r.sched == nil || r.sched.state == nil {
			return 0
		}
		return r.sched.state.ChannelFailScore(channelID)
	})

	// failover 时给同一渠道一定重试机会，然后再切换到“下一个”渠道（若存在）。
	// 典型场景：同渠道多 key/账号可接管；或短暂抖动下重试可恢复。
	const maxConsecutiveSameChannel = 2
	if r.lastSelectedChannelID != 0 && r.lastSelectedStreak >= maxConsecutiveSameChannel && len(ordered) > 1 {
		reordered := make([]channelCandidate, 0, len(ordered))
		deferred := channelCandidate{}
		hasDeferred := false
		for _, cand := range ordered {
			if cand.ChannelID == r.lastSelectedChannelID {
				deferred = cand
				hasDeferred = true
				continue
			}
			reordered = append(reordered, cand)
		}
		if hasDeferred {
			reordered = append(reordered, deferred)
		}
		ordered = reordered
	}

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
		if cand.ChannelID == r.lastSelectedChannelID {
			r.lastSelectedStreak++
		} else {
			r.lastSelectedChannelID = cand.ChannelID
			r.lastSelectedStreak = 1
		}
		if r.sched != nil {
			r.sched.touchChannelGroupPointer(ctx, groupID, cand.ChannelID, "route")
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

func buildCandidateRing(in map[int64]channelCandidate) []int64 {
	if len(in) == 0 {
		return nil
	}
	out := make([]channelCandidate, 0, len(in))
	for _, c := range in {
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Promotion != out[j].Promotion {
			return out[i].Promotion
		}
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].ChannelID > out[j].ChannelID
	})
	ids := make([]int64, 0, len(out))
	for _, c := range out {
		if c.ChannelID <= 0 {
			continue
		}
		ids = append(ids, c.ChannelID)
	}
	return ids
}

func nextUnbannedInRing(ring []int64, startIdx int, isBanned func(channelID int64) bool) (int64, int, bool) {
	if len(ring) == 0 {
		return 0, 0, false
	}
	if startIdx < 0 || startIdx >= len(ring) {
		startIdx = 0
	}
	for step := 1; step <= len(ring); step++ {
		idx := (startIdx + step) % len(ring)
		id := ring[idx]
		if id <= 0 {
			continue
		}
		if isBanned != nil && isBanned(id) {
			continue
		}
		return id, idx, true
	}
	return 0, 0, false
}

func sortCandidates(in map[int64]channelCandidate, isProbePending func(channelID int64) bool, failScore func(channelID int64) int) []channelCandidate {
	out := make([]channelCandidate, 0, len(in))
	for _, c := range in {
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if isProbePending != nil {
			pi := isProbePending(out[i].ChannelID)
			pj := isProbePending(out[j].ChannelID)
			if pi != pj {
				return pi
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
