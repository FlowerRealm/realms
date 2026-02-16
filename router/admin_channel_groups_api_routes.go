package router

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"realms/internal/store"
)

type adminChannelGroupView struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	Description        *string `json:"description,omitempty"`
	PriceMultiplier    string  `json:"price_multiplier"`
	MaxAttempts        int     `json:"max_attempts"`
	Status             int     `json:"status"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
	IsDefault          bool    `json:"is_default"`
	PointerChannelID   int64   `json:"pointer_channel_id,omitempty"`
	PointerChannelName string  `json:"pointer_channel_name,omitempty"`
	PointerPinned      bool    `json:"pointer_pinned,omitempty"`
}

func setAdminChannelGroupAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/channel-groups", adminListChannelGroupsHandler(opts))
	r.POST("/channel-groups", adminCreateChannelGroupHandler(opts))
	r.GET("/channel-groups/:group_id", adminGetChannelGroupHandler(opts))
	r.GET("/channel-groups/:group_id/detail", adminGetChannelGroupDetailHandler(opts))
	r.GET("/channel-groups/:group_id/pointer", adminGetChannelGroupPointerHandler(opts))
	r.GET("/channel-groups/:group_id/pointer/candidates", adminListChannelGroupPointerCandidatesHandler(opts))
	r.PUT("/channel-groups/:group_id", adminUpdateChannelGroupHandler(opts))
	r.DELETE("/channel-groups/:group_id", adminDeleteChannelGroupHandler(opts))
	r.PUT("/channel-groups/:group_id/default", adminSetDefaultChannelGroupHandler(opts))
	r.PUT("/channel-groups/:group_id/pointer", adminUpsertChannelGroupPointerHandler(opts))

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
		defaultID, _, _ := opts.Store.GetDefaultChannelGroupID(c.Request.Context())
		groups, err := opts.Store.ListChannelGroups(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		groupIDs := make([]int64, 0, len(groups))
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID)
		}
		pointers, err := opts.Store.GetChannelGroupPointerSnapshots(c.Request.Context(), groupIDs)
		if err != nil {
			pointers = nil
		}

		out := make([]adminChannelGroupView, 0, len(groups))
		for _, g := range groups {
			var ptrChannelID int64
			var ptrChannelName string
			var ptrPinned bool
			if pointers != nil {
				if ptr, ok := pointers[g.ID]; ok && ptr.ChannelID > 0 {
					ptrChannelID = ptr.ChannelID
					ptrChannelName = strings.TrimSpace(ptr.ChannelName)
					ptrPinned = ptr.Pinned
				}
			}
			if ptrChannelID <= 0 {
				id, name, ok, err := defaultChannelGroupPointerCandidate(c.Request.Context(), opts.Store, g.ID)
				if err == nil && ok && id > 0 {
					ptrChannelID = id
					ptrChannelName = strings.TrimSpace(name)
					ptrPinned = false
				}
			}

			out = append(out, adminChannelGroupView{
				ID:                 g.ID,
				Name:               g.Name,
				Description:        g.Description,
				PriceMultiplier:    formatDecimalPlain(g.PriceMultiplier, store.PriceMultiplierScale),
				MaxAttempts:        g.MaxAttempts,
				Status:             g.Status,
				CreatedAt:          g.CreatedAt.Format("2006-01-02 15:04"),
				UpdatedAt:          g.UpdatedAt.Format("2006-01-02 15:04"),
				IsDefault:          defaultID > 0 && g.ID == defaultID,
				PointerChannelID:   ptrChannelID,
				PointerChannelName: ptrChannelName,
				PointerPinned:      ptrPinned,
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
		defaultID, _, _ := opts.Store.GetDefaultChannelGroupID(c.Request.Context())
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
				IsDefault:       defaultID > 0 && g.ID == defaultID,
			},
		})
	}
}

func adminSetDefaultChannelGroupHandler(opts Options) gin.HandlerFunc {
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
		if err := opts.Store.SetDefaultChannelGroupID(c.Request.Context(), groupID); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已设置默认渠道组"})
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
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败（可能渠道组已存在）"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已创建", "data": gin.H{"id": id}})
	}
}

func adminUpdateChannelGroupHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Name            *string `json:"name,omitempty"`
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

		if _, err := opts.Store.UpdateChannelGroupWithRename(c.Request.Context(), g.ID, req.Name, req.Description, status, priceMult, maxAttempts); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
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

		sum, err := opts.Store.ForceDeleteChannelGroup(c.Request.Context(), g.ID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
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
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败（可能渠道组已存在）"})
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
		if err := opts.Store.AddChannelGroupMemberChannel(c.Request.Context(), parentID, req.ChannelID, 0, ch.Promotion); err != nil {
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

type adminChannelGroupPointerView struct {
	GroupID     int64  `json:"group_id"`
	ChannelID   int64  `json:"channel_id"`
	ChannelName string `json:"channel_name,omitempty"`
	Pinned      bool   `json:"pinned"`
	MovedAt     string `json:"moved_at,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Note        string `json:"note,omitempty"`
}

func adminGetChannelGroupPointerHandler(opts Options) gin.HandlerFunc {
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
		if _, err := opts.Store.GetChannelGroupByID(c.Request.Context(), groupID); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		rec, ok, err := opts.Store.GetChannelGroupPointer(c.Request.Context(), groupID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}
		if !ok {
			rec = store.ChannelGroupPointer{GroupID: groupID}
		}

		chName := ""
		if rec.ChannelID <= 0 {
			id, name, ok, err := defaultChannelGroupPointerCandidate(c.Request.Context(), opts.Store, groupID)
			if err == nil && ok && id > 0 {
				rec.ChannelID = id
				rec.Pinned = false
				chName = strings.TrimSpace(name)
			}
		}
		if rec.ChannelID > 0 {
			if ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), rec.ChannelID); err == nil {
				chName = strings.TrimSpace(ch.Name)
			}
		}

		loc, _ := adminTimeLocation(c.Request.Context(), opts)
		movedAt := rec.MovedAt()
		movedAtText := ""
		if !movedAt.IsZero() {
			movedAtText = formatTimeIn(movedAt, "2006-01-02 15:04:05", loc)
		}

		reason := strings.TrimSpace(rec.Reason)
		reasonText := groupPointerReasonText(reason)
		note := ""
		switch {
		case movedAtText != "" && reasonText != "":
			note = "更新时间：" + movedAtText + "；原因：" + reasonText
		case movedAtText != "":
			note = "更新时间：" + movedAtText
		case reasonText != "":
			note = "原因：" + reasonText
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": adminChannelGroupPointerView{
				GroupID:     groupID,
				ChannelID:   rec.ChannelID,
				ChannelName: chName,
				Pinned:      rec.Pinned,
				MovedAt:     movedAtText,
				Reason:      reason,
				Note:        note,
			},
		})
	}
}

func adminUpsertChannelGroupPointerHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		ChannelID int64 `json:"channel_id"`
		Pinned    *bool `json:"pinned,omitempty"`
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
		if _, err := opts.Store.GetChannelGroupByID(c.Request.Context(), groupID); err != nil {
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

		channelID := req.ChannelID
		if channelID < 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}
		pinned := true
		if req.Pinned != nil {
			pinned = *req.Pinned
		}
		if channelID == 0 {
			pinned = false
		}

		if channelID > 0 {
			ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
			if err != nil || ch.ID <= 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			inGroup, err := channelBelongsToGroup(c.Request.Context(), opts.Store, groupID, channelID)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "校验失败"})
				return
			}
			if !inGroup {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不属于该渠道组"})
				return
			}
			if opts.Sched != nil {
				opts.Sched.ClearChannelBan(channelID)
			}
		}

		reason := "manual"
		if channelID == 0 {
			reason = "clear"
		}
		if err := opts.Store.UpsertChannelGroupPointer(c.Request.Context(), store.ChannelGroupPointer{
			GroupID:       groupID,
			ChannelID:     channelID,
			Pinned:        pinned,
			MovedAtUnixMS: time.Now().UnixMilli(),
			Reason:        reason,
		}); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已更新"})
	}
}

func groupPointerReasonText(raw string) string {
	switch strings.TrimSpace(raw) {
	case "manual":
		return "手动设置"
	case "ban":
		return "因封禁轮转"
	case "invalid":
		return "指针无效修正"
	case "route":
		return "路由选中"
	case "clear":
		return "清除"
	default:
		return strings.TrimSpace(raw)
	}
}

func channelBelongsToGroup(ctx context.Context, st *store.Store, groupID int64, channelID int64) (bool, error) {
	if st == nil || groupID <= 0 || channelID <= 0 {
		return false, nil
	}
	visited := make(map[int64]struct{})
	var walk func(gid int64) (bool, error)
	walk = func(gid int64) (bool, error) {
		if gid <= 0 {
			return false, nil
		}
		if _, ok := visited[gid]; ok {
			return false, nil
		}
		visited[gid] = struct{}{}
		members, err := st.ListChannelGroupMembers(ctx, gid)
		if err != nil {
			return false, err
		}
		for _, m := range members {
			if m.MemberChannelID != nil && *m.MemberChannelID == channelID {
				return true, nil
			}
			if m.MemberGroupID != nil {
				ok, err := walk(*m.MemberGroupID)
				if err != nil {
					return false, err
				}
				if ok {
					return true, nil
				}
			}
		}
		return false, nil
	}
	return walk(groupID)
}

type channelGroupPointerCandidate struct {
	channelID int64
	name      string
	promotion bool
	priority  int
	enabled   bool
}

func defaultChannelGroupPointerCandidate(ctx context.Context, st *store.Store, groupID int64) (int64, string, bool, error) {
	if st == nil || groupID <= 0 {
		return 0, "", false, nil
	}

	visited := make(map[int64]struct{}, 64)
	cands := make(map[int64]channelGroupPointerCandidate, 128)

	var walk func(gid int64) error
	walk = func(gid int64) error {
		if gid <= 0 {
			return nil
		}
		if _, ok := visited[gid]; ok {
			return nil
		}
		visited[gid] = struct{}{}

		members, err := st.ListChannelGroupMembers(ctx, gid)
		if err != nil {
			return err
		}
		for _, m := range members {
			// 成员类型校验：必须且只能存在一种 member。
			if m.MemberGroupID != nil && m.MemberChannelID != nil {
				continue
			}
			if m.MemberGroupID == nil && m.MemberChannelID == nil {
				continue
			}

			if m.MemberGroupID != nil {
				if m.MemberGroupStatus != nil && *m.MemberGroupStatus != 1 {
					continue
				}
				if err := walk(*m.MemberGroupID); err != nil {
					return err
				}
				continue
			}

			if m.MemberChannelID == nil || *m.MemberChannelID <= 0 {
				continue
			}
			chID := *m.MemberChannelID
			name := ""
			if m.MemberChannelName != nil {
				name = strings.TrimSpace(*m.MemberChannelName)
			}
			enabled := m.MemberChannelStatus != nil && *m.MemberChannelStatus == 1
			cand := channelGroupPointerCandidate{
				channelID: chID,
				name:      name,
				promotion: m.Promotion,
				priority:  m.Priority,
				enabled:   enabled,
			}
			if prev, ok := cands[chID]; ok {
				if cand.promotion && !prev.promotion {
					cands[chID] = cand
					continue
				}
				if cand.promotion == prev.promotion && cand.priority > prev.priority {
					cands[chID] = cand
					continue
				}
				if prev.name == "" && cand.name != "" {
					prev.name = cand.name
					cands[chID] = prev
				}
				continue
			}
			cands[chID] = cand
		}
		return nil
	}

	if err := walk(groupID); err != nil {
		return 0, "", false, err
	}
	if len(cands) == 0 {
		return 0, "", false, nil
	}

	better := func(a, b channelGroupPointerCandidate) bool {
		if a.promotion != b.promotion {
			return a.promotion && !b.promotion
		}
		if a.priority != b.priority {
			return a.priority > b.priority
		}
		return a.channelID > b.channelID
	}

	var bestEnabled channelGroupPointerCandidate
	bestEnabledOK := false
	var bestAny channelGroupPointerCandidate
	bestAnyOK := false
	for _, v := range cands {
		if v.channelID <= 0 {
			continue
		}
		if !bestAnyOK || better(v, bestAny) {
			bestAny = v
			bestAnyOK = true
		}
		if v.enabled {
			if !bestEnabledOK || better(v, bestEnabled) {
				bestEnabled = v
				bestEnabledOK = true
			}
		}
	}

	if bestEnabledOK {
		return bestEnabled.channelID, bestEnabled.name, true, nil
	}
	if bestAnyOK {
		return bestAny.channelID, bestAny.name, true, nil
	}
	return 0, "", false, nil
}
