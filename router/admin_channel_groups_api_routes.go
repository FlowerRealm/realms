package router

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"realms/internal/store"
)

type adminChannelGroupView struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Description     *string `json:"description,omitempty"`
	PriceMultiplier string  `json:"price_multiplier"`
	MaxAttempts     int     `json:"max_attempts"`
	Status          int     `json:"status"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

func setAdminChannelGroupAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/channel-groups", adminListChannelGroupsHandler(opts))
	r.POST("/channel-groups", adminCreateChannelGroupHandler(opts))
	r.GET("/channel-groups/:group_id", adminGetChannelGroupHandler(opts))
	r.GET("/channel-groups/:group_id/detail", adminGetChannelGroupDetailHandler(opts))
	r.PUT("/channel-groups/:group_id", adminUpdateChannelGroupHandler(opts))
	r.DELETE("/channel-groups/:group_id", adminDeleteChannelGroupHandler(opts))

	r.POST("/channel-groups/:group_id/children/groups", adminCreateChildChannelGroupHandler(opts))
	r.POST("/channel-groups/:group_id/children/channels", adminAddChannelGroupChannelMemberHandler(opts))
	r.DELETE("/channel-groups/:group_id/children/groups/:child_group_id", adminDeleteChannelGroupGroupMemberHandler(opts))
	r.DELETE("/channel-groups/:group_id/children/channels/:channel_id", adminDeleteChannelGroupChannelMemberHandler(opts))
	r.POST("/channel-groups/:group_id/children/reorder", adminReorderChannelGroupMembersHandler(opts))
}

func adminListChannelGroupsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		groups, err := opts.Store.ListChannelGroups(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}
		out := make([]adminChannelGroupView, 0, len(groups))
		for _, g := range groups {
			out = append(out, adminChannelGroupView{
				ID:              g.ID,
				Name:            g.Name,
				Description:     g.Description,
				PriceMultiplier: formatDecimalPlain(g.PriceMultiplier, store.PriceMultiplierScale),
				MaxAttempts:     g.MaxAttempts,
				Status:          g.Status,
				CreatedAt:       g.CreatedAt.Format("2006-01-02 15:04"),
				UpdatedAt:       g.UpdatedAt.Format("2006-01-02 15:04"),
			})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func adminGetChannelGroupHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		groupID, err := strconv.ParseInt(strings.TrimSpace(c.Param("group_id")), 10, 64)
		if err != nil || groupID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_id 不合法"})
			return
		}
		g, err := opts.Store.GetChannelGroupByID(c.Request.Context(), groupID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": adminChannelGroupView{
				ID:              g.ID,
				Name:            g.Name,
				Description:     g.Description,
				PriceMultiplier: formatDecimalPlain(g.PriceMultiplier, store.PriceMultiplierScale),
				MaxAttempts:     g.MaxAttempts,
				Status:          g.Status,
				CreatedAt:       g.CreatedAt.Format("2006-01-02 15:04"),
				UpdatedAt:       g.UpdatedAt.Format("2006-01-02 15:04"),
			},
		})
	}
}

func adminCreateChannelGroupHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Name            string  `json:"name"`
		Description     *string `json:"description,omitempty"`
		PriceMultiplier string  `json:"price_multiplier"`
		MaxAttempts     int     `json:"max_attempts"`
		Status          int     `json:"status"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		name, err := normalizeGroupName(strings.TrimSpace(req.Name))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		if name == store.DefaultGroupName {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "default 分组已存在"})
			return
		}

		priceMult := store.DefaultGroupPriceMultiplier
		if strings.TrimSpace(req.PriceMultiplier) != "" {
			m, err := parseDecimalNonNeg(req.PriceMultiplier, store.PriceMultiplierScale)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "price_multiplier 不合法"})
				return
			}
			priceMult = m
		}

		status := req.Status
		if status != 0 && status != 1 {
			status = 1
		}
		maxAttempts := req.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = 5
		}

		id, err := opts.Store.CreateChannelGroup(c.Request.Context(), name, req.Description, status, priceMult, maxAttempts)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败（可能分组已存在）"})
			return
		}

		// 新建根分组默认挂载到 default 根组（与 SSR 行为一致）。
		def, err := opts.Store.GetChannelGroupByName(c.Request.Context(), store.DefaultGroupName)
		if err != nil || def.ID <= 0 {
			_ = opts.Store.DeleteChannelGroup(c.Request.Context(), id)
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败"})
			return
		}
		if err := opts.Store.AddChannelGroupMemberGroup(c.Request.Context(), def.ID, id, 0, false); err != nil {
			_ = opts.Store.DeleteChannelGroup(c.Request.Context(), id)
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已创建", "data": gin.H{"id": id}})
	}
}

func adminUpdateChannelGroupHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Description     *string `json:"description,omitempty"`
		PriceMultiplier string  `json:"price_multiplier"`
		MaxAttempts     int     `json:"max_attempts"`
		Status          int     `json:"status"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		groupID, err := strconv.ParseInt(strings.TrimSpace(c.Param("group_id")), 10, 64)
		if err != nil || groupID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_id 不合法"})
			return
		}
		g, err := opts.Store.GetChannelGroupByID(c.Request.Context(), groupID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		status := req.Status
		if status != 0 && status != 1 {
			status = g.Status
		}
		if strings.TrimSpace(g.Name) == store.DefaultGroupName && status != 1 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "default 分组不允许禁用"})
			return
		}

		priceMult := g.PriceMultiplier
		if strings.TrimSpace(req.PriceMultiplier) != "" {
			m, err := parseDecimalNonNeg(req.PriceMultiplier, store.PriceMultiplierScale)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "price_multiplier 不合法"})
				return
			}
			priceMult = m
		}

		maxAttempts := req.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = g.MaxAttempts
		}

		if err := opts.Store.UpdateChannelGroup(c.Request.Context(), g.ID, req.Description, status, priceMult, maxAttempts); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}
		// best-effort: refresh updated_at for response message consistency.
		_, _ = opts.Store.GetChannelGroupByID(c.Request.Context(), g.ID)

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func adminDeleteChannelGroupHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		groupID, err := strconv.ParseInt(strings.TrimSpace(c.Param("group_id")), 10, 64)
		if err != nil || groupID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_id 不合法"})
			return
		}
		g, err := opts.Store.GetChannelGroupByID(c.Request.Context(), groupID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}
		if strings.TrimSpace(g.Name) == store.DefaultGroupName {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "default 分组不允许删除"})
			return
		}

		sum, err := opts.Store.ForceDeleteChannelGroup(c.Request.Context(), g.ID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
			return
		}
		msg := "已删除"
		if sum.UsersUnbound > 0 || sum.ChannelsUpdated > 0 {
			msg += "（已解绑 users=" + strconv.FormatInt(sum.UsersUnbound, 10)
			if sum.ChannelsUpdated > 0 {
				msg += ", channels=" + strconv.FormatInt(sum.ChannelsUpdated, 10)
			}
			if sum.ChannelsDisabled > 0 {
				msg += ", disabled=" + strconv.FormatInt(sum.ChannelsDisabled, 10)
			}
			msg += "）"
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": msg})
	}
}

func normalizeGroupName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", errFieldRequired("name")
	}
	if len(name) > 64 {
		return "", errFieldTooLong("name", 64)
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			continue
		}
		return "", errFieldInvalid("name", "仅允许字母/数字/下划线/连字符")
	}
	return name, nil
}

func errFieldRequired(field string) error {
	return &fieldError{field: field, msg: "不能为空"}
}

func errFieldTooLong(field string, max int) error {
	return &fieldError{field: field, msg: "过长（最多 " + strconv.Itoa(max) + " 字符）"}
}

func errFieldInvalid(field string, msg string) error {
	return &fieldError{field: field, msg: msg}
}

type fieldError struct {
	field string
	msg   string
}

func (e *fieldError) Error() string {
	return e.field + " " + e.msg
}

type adminChannelGroupMemberView struct {
	MemberID      int64 `json:"member_id"`
	ParentGroupID int64 `json:"parent_group_id"`

	MemberGroupID          *int64  `json:"member_group_id,omitempty"`
	MemberGroupName        *string `json:"member_group_name,omitempty"`
	MemberGroupStatus      *int    `json:"member_group_status,omitempty"`
	MemberGroupMaxAttempts *int    `json:"member_group_max_attempts,omitempty"`

	MemberChannelID     *int64  `json:"member_channel_id,omitempty"`
	MemberChannelName   *string `json:"member_channel_name,omitempty"`
	MemberChannelType   *string `json:"member_channel_type,omitempty"`
	MemberChannelGroups *string `json:"member_channel_groups,omitempty"`
	MemberChannelStatus *int    `json:"member_channel_status,omitempty"`

	Priority  int  `json:"priority"`
	Promotion bool `json:"promotion"`
}

type adminChannelRefView struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type adminChannelGroupDetailResponse struct {
	Group      adminChannelGroupView         `json:"group"`
	Breadcrumb []adminChannelGroupView       `json:"breadcrumb"`
	Members    []adminChannelGroupMemberView `json:"members"`
	Channels   []adminChannelRefView         `json:"channels"`
}

func adminGetChannelGroupDetailHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		groupID, err := strconv.ParseInt(strings.TrimSpace(c.Param("group_id")), 10, 64)
		if err != nil || groupID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_id 不合法"})
			return
		}

		g, err := opts.Store.GetChannelGroupByID(c.Request.Context(), groupID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		breadcrumb, err := channelGroupBreadcrumb(c.Request.Context(), opts.Store, groupID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		members, err := opts.Store.ListChannelGroupMembers(c.Request.Context(), groupID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		channels, err := opts.Store.ListUpstreamChannels(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		resp := adminChannelGroupDetailResponse{
			Group: adminChannelGroupView{
				ID:              g.ID,
				Name:            g.Name,
				Description:     g.Description,
				PriceMultiplier: formatDecimalPlain(g.PriceMultiplier, store.PriceMultiplierScale),
				MaxAttempts:     g.MaxAttempts,
				Status:          g.Status,
				CreatedAt:       g.CreatedAt.Format("2006-01-02 15:04"),
				UpdatedAt:       g.UpdatedAt.Format("2006-01-02 15:04"),
			},
			Breadcrumb: breadcrumb,
			Members:    make([]adminChannelGroupMemberView, 0, len(members)),
			Channels:   make([]adminChannelRefView, 0, len(channels)),
		}

		for _, m := range members {
			resp.Members = append(resp.Members, adminChannelGroupMemberView{
				MemberID:      m.MemberID,
				ParentGroupID: m.ParentGroupID,

				MemberGroupID:          m.MemberGroupID,
				MemberGroupName:        m.MemberGroupName,
				MemberGroupStatus:      m.MemberGroupStatus,
				MemberGroupMaxAttempts: m.MemberGroupMaxAttempts,

				MemberChannelID:     m.MemberChannelID,
				MemberChannelName:   m.MemberChannelName,
				MemberChannelType:   m.MemberChannelType,
				MemberChannelGroups: m.MemberChannelGroups,
				MemberChannelStatus: m.MemberChannelStatus,

				Priority:  m.Priority,
				Promotion: m.Promotion,
			})
		}

		for _, ch := range channels {
			resp.Channels = append(resp.Channels, adminChannelRefView{
				ID:   ch.ID,
				Name: ch.Name,
				Type: ch.Type,
			})
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": resp})
	}
}

func adminCreateChildChannelGroupHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Name            string  `json:"name"`
		Description     *string `json:"description,omitempty"`
		PriceMultiplier string  `json:"price_multiplier"`
		MaxAttempts     int     `json:"max_attempts"`
		Status          int     `json:"status"`
	}

	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		parentID, err := strconv.ParseInt(strings.TrimSpace(c.Param("group_id")), 10, 64)
		if err != nil || parentID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_id 不合法"})
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		name, err := normalizeGroupName(strings.TrimSpace(req.Name))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		if name == store.DefaultGroupName {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "default 分组已存在"})
			return
		}

		priceMult := store.DefaultGroupPriceMultiplier
		if strings.TrimSpace(req.PriceMultiplier) != "" {
			m, err := parseDecimalNonNeg(req.PriceMultiplier, store.PriceMultiplierScale)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "price_multiplier 不合法"})
				return
			}
			priceMult = m
		}

		status := req.Status
		if status != 0 && status != 1 {
			status = 1
		}
		maxAttempts := req.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = 5
		}

		id, err := opts.Store.CreateChannelGroup(c.Request.Context(), name, req.Description, status, priceMult, maxAttempts)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败（可能分组已存在）"})
			return
		}
		if err := opts.Store.AddChannelGroupMemberGroup(c.Request.Context(), parentID, id, 0, false); err != nil {
			_ = opts.Store.DeleteChannelGroup(c.Request.Context(), id)
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已创建", "data": gin.H{"id": id}})
	}
}

func adminAddChannelGroupChannelMemberHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		ChannelID int64 `json:"channel_id"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		parentID, err := strconv.ParseInt(strings.TrimSpace(c.Param("group_id")), 10, 64)
		if err != nil || parentID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_id 不合法"})
			return
		}
		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if req.ChannelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), req.ChannelID)
		if err != nil || ch.ID <= 0 {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "channel 不存在"})
			return
		}
		if err := opts.Store.AddChannelGroupMemberChannel(c.Request.Context(), parentID, req.ChannelID, ch.Priority, ch.Promotion); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已添加"})
	}
}

func adminDeleteChannelGroupGroupMemberHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		parentID, err := strconv.ParseInt(strings.TrimSpace(c.Param("group_id")), 10, 64)
		if err != nil || parentID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_id 不合法"})
			return
		}
		childID, err := strconv.ParseInt(strings.TrimSpace(c.Param("child_group_id")), 10, 64)
		if err != nil || childID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "child_group_id 不合法"})
			return
		}
		if err := opts.Store.RemoveChannelGroupMemberGroup(c.Request.Context(), parentID, childID); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已移除"})
	}
}

func adminDeleteChannelGroupChannelMemberHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		parentID, err := strconv.ParseInt(strings.TrimSpace(c.Param("group_id")), 10, 64)
		if err != nil || parentID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_id 不合法"})
			return
		}
		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}
		if err := opts.Store.RemoveChannelGroupMemberChannel(c.Request.Context(), parentID, channelID); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已移除"})
	}
}

func adminReorderChannelGroupMembersHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		groupID, err := strconv.ParseInt(strings.TrimSpace(c.Param("group_id")), 10, 64)
		if err != nil || groupID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_id 不合法"})
			return
		}
		var ids []int64
		if err := c.ShouldBindJSON(&ids); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if len(ids) > 0 {
			if err := opts.Store.ReorderChannelGroupMembers(c.Request.Context(), groupID, ids); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "排序保存失败"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func channelGroupBreadcrumb(ctx context.Context, st *store.Store, groupID int64) ([]adminChannelGroupView, error) {
	if groupID == 0 {
		return nil, errors.New("groupID 不能为空")
	}
	var chain []store.ChannelGroup
	cur := groupID
	for i := 0; i < 32; i++ {
		g, err := st.GetChannelGroupByID(ctx, cur)
		if err != nil {
			return nil, err
		}
		chain = append(chain, g)
		if strings.TrimSpace(g.Name) == store.DefaultGroupName {
			break
		}
		parentID, ok, err := st.GetChannelGroupParentID(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		if !ok || parentID == 0 {
			break
		}
		cur = parentID
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}

	out := make([]adminChannelGroupView, 0, len(chain))
	for _, g := range chain {
		out = append(out, adminChannelGroupView{
			ID:              g.ID,
			Name:            g.Name,
			Description:     g.Description,
			PriceMultiplier: formatDecimalPlain(g.PriceMultiplier, store.PriceMultiplierScale),
			MaxAttempts:     g.MaxAttempts,
			Status:          g.Status,
			CreatedAt:       g.CreatedAt.Format("2006-01-02 15:04"),
			UpdatedAt:       g.UpdatedAt.Format("2006-01-02 15:04"),
		})
	}
	return out, nil
}
