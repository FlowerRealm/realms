package router

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/icons"
	"realms/internal/store"
)

type managedModelView struct {
	ID                  int64           `json:"id"`
	PublicID            string          `json:"public_id"`
	OwnedBy             *string         `json:"owned_by,omitempty"`
	InputUSDPer1M       decimal.Decimal `json:"input_usd_per_1m"`
	OutputUSDPer1M      decimal.Decimal `json:"output_usd_per_1m"`
	CacheInputUSDPer1M  decimal.Decimal `json:"cache_input_usd_per_1m"`
	CacheOutputUSDPer1M decimal.Decimal `json:"cache_output_usd_per_1m"`
	Status              int             `json:"status"`
	IconURL             *string         `json:"icon_url,omitempty"`
}

type pageInfo[T any] struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
	Items    []T `json:"items"`
}

func setModelAPIRoutes(r gin.IRoutes, opts Options) {
	userAuthn := requireUserSession(opts)

	// new-api: GET /api/models (dashboardListModels)
	r.GET("/models", userAuthn, dashboardModelsHandler(opts))

	// new-api: GET /api/user/models
	r.GET("/user/models", userAuthn, userModelsHandler(opts))
	// Legacy UI: model list with pricing/owned_by (used by SPA to fully replicate SSR page).
	r.GET("/user/models/detail", userAuthn, userModelsDetailHandler(opts))

	// Admin models CRUD: /api/models/*
	// 对齐 new-api：管理侧使用 trailing slash 的集合接口（/api/models/?p=1&page_size=...）。
	models := r.(*gin.RouterGroup).Group("/models")
	models.Use(requireRootSession(opts))
	{
		models.GET("/", adminListManagedModelsHandler(opts))
		models.GET("/:model_id", adminGetManagedModelHandler(opts))
		models.POST("/", adminCreateManagedModelHandler(opts))
		models.POST("/library-lookup", adminModelLibraryLookupHandler(opts))
		models.POST("/import-pricing", adminImportModelPricingHandler(opts))
		models.PUT("/", adminUpdateManagedModelHandler(opts))
		models.DELETE("/:model_id", adminDeleteManagedModelHandler(opts))
	}

	// Channel model bindings: /api/channel/:id/models
	ch := r.(*gin.RouterGroup).Group("/channel")
	ch.Use(requireRootSession(opts))
	{
		ch.GET("/:channel_id/models", adminListChannelModelsHandler(opts))
		ch.POST("/:channel_id/models", adminCreateChannelModelHandler(opts))
		ch.PUT("/:channel_id/models", adminUpdateChannelModelHandler(opts))
		ch.DELETE("/:channel_id/models/:binding_id", adminDeleteChannelModelHandler(opts))
	}
}

type userManagedModelView struct {
	ID                  int64           `json:"id"`
	PublicID            string          `json:"public_id"`
	OwnedBy             *string         `json:"owned_by,omitempty"`
	InputUSDPer1M       decimal.Decimal `json:"input_usd_per_1m"`
	OutputUSDPer1M      decimal.Decimal `json:"output_usd_per_1m"`
	CacheInputUSDPer1M  decimal.Decimal `json:"cache_input_usd_per_1m"`
	CacheOutputUSDPer1M decimal.Decimal `json:"cache_output_usd_per_1m"`
	Status              int             `json:"status"`
	IconURL             *string         `json:"icon_url,omitempty"`
}

func dashboardModelsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		channels, err := opts.Store.ListUpstreamChannels(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询渠道失败"})
			return
		}

		out := make(map[int64][]string, len(channels))
		for _, ch := range channels {
			ms, err := opts.Store.ListChannelModelsByChannelID(c.Request.Context(), ch.ID)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询渠道模型失败"})
				return
			}
			var ids []string
			for _, m := range ms {
				if m.Status != 1 {
					continue
				}
				pid := strings.TrimSpace(m.PublicID)
				if pid == "" {
					continue
				}
				ids = append(ids, pid)
			}
			sort.Strings(ids)
			out[ch.ID] = ids
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func userModelsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		u, err := opts.Store.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "用户查询失败"})
			return
		}

		uniq := make(map[string]struct{})
		for _, g := range u.Groups {
			ms, err := opts.Store.ListEnabledManagedModelsWithBindingsForGroup(c.Request.Context(), g)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "模型查询失败"})
				return
			}
			for _, m := range ms {
				pid := strings.TrimSpace(m.PublicID)
				if pid == "" {
					continue
				}
				uniq[pid] = struct{}{}
			}
		}

		out := make([]string, 0, len(uniq))
		for id := range uniq {
			out = append(out, id)
		}
		sort.Strings(out)
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func userModelsDetailHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		u, err := opts.Store.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "用户查询失败"})
			return
		}

		ms, err := opts.Store.ListEnabledManagedModelsWithBindingsForGroups(c.Request.Context(), u.Groups)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "模型查询失败"})
			return
		}

		out := make([]userManagedModelView, 0, len(ms))
		for _, m := range ms {
			icon := strings.TrimSpace(icons.ModelIconURL(m.PublicID, derefString(m.OwnedBy)))
			var iconPtr *string
			if icon != "" {
				iconPtr = &icon
			}
			out = append(out, userManagedModelView{
				ID:                  m.ID,
				PublicID:            m.PublicID,
				OwnedBy:             m.OwnedBy,
				InputUSDPer1M:       m.InputUSDPer1M,
				OutputUSDPer1M:      m.OutputUSDPer1M,
				CacheInputUSDPer1M:  m.CacheInputUSDPer1M,
				CacheOutputUSDPer1M: m.CacheOutputUSDPer1M,
				Status:              m.Status,
				IconURL:             iconPtr,
			})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func adminListManagedModelsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		page := 1
		if v := strings.TrimSpace(c.Query("p")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				page = n
			}
		}
		pageSize := 10
		if v := strings.TrimSpace(c.Query("page_size")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				pageSize = n
			}
		}
		if pageSize > 1000 {
			pageSize = 1000
		}

		all, err := opts.Store.ListManagedModels(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询模型失败"})
			return
		}

		total := len(all)
		start := (page - 1) * pageSize
		if start < 0 {
			start = 0
		}
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}

		items := make([]managedModelView, 0, end-start)
		for _, m := range all[start:end] {
			icon := strings.TrimSpace(icons.ModelIconURL(m.PublicID, derefString(m.OwnedBy)))
			var iconPtr *string
			if icon != "" {
				iconPtr = &icon
			}
			items = append(items, managedModelView{
				ID:                  m.ID,
				PublicID:            m.PublicID,
				OwnedBy:             m.OwnedBy,
				InputUSDPer1M:       m.InputUSDPer1M,
				OutputUSDPer1M:      m.OutputUSDPer1M,
				CacheInputUSDPer1M:  m.CacheInputUSDPer1M,
				CacheOutputUSDPer1M: m.CacheOutputUSDPer1M,
				Status:              m.Status,
				IconURL:             iconPtr,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": pageInfo[managedModelView]{
				Page:     page,
				PageSize: pageSize,
				Total:    total,
				Items:    items,
			},
		})
	}
}

func adminGetManagedModelHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		id, err := strconv.ParseInt(strings.TrimSpace(c.Param("model_id")), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 不合法"})
			return
		}
		m, err := opts.Store.GetManagedModelByID(c.Request.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "模型不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": managedModelView{
			ID:                  m.ID,
			PublicID:            m.PublicID,
			OwnedBy:             m.OwnedBy,
			InputUSDPer1M:       m.InputUSDPer1M,
			OutputUSDPer1M:      m.OutputUSDPer1M,
			CacheInputUSDPer1M:  m.CacheInputUSDPer1M,
			CacheOutputUSDPer1M: m.CacheOutputUSDPer1M,
			Status:              m.Status,
			IconURL: func() *string {
				icon := strings.TrimSpace(icons.ModelIconURL(m.PublicID, derefString(m.OwnedBy)))
				if icon == "" {
					return nil
				}
				return &icon
			}(),
		}})
	}
}

func adminCreateManagedModelHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		PublicID            string          `json:"public_id"`
		OwnedBy             *string         `json:"owned_by"`
		InputUSDPer1M       decimal.Decimal `json:"input_usd_per_1m"`
		OutputUSDPer1M      decimal.Decimal `json:"output_usd_per_1m"`
		CacheInputUSDPer1M  decimal.Decimal `json:"cache_input_usd_per_1m"`
		CacheOutputUSDPer1M decimal.Decimal `json:"cache_output_usd_per_1m"`
		Status              int             `json:"status"`
	}

	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		publicID := strings.TrimSpace(req.PublicID)
		if publicID == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "public_id 不能为空"})
			return
		}
		if req.Status == 0 {
			req.Status = 1
		}

		id, err := opts.Store.CreateManagedModel(c.Request.Context(), store.ManagedModelCreate{
			PublicID:            publicID,
			OwnedBy:             req.OwnedBy,
			InputUSDPer1M:       req.InputUSDPer1M,
			OutputUSDPer1M:      req.OutputUSDPer1M,
			CacheInputUSDPer1M:  req.CacheInputUSDPer1M,
			CacheOutputUSDPer1M: req.CacheOutputUSDPer1M,
			Status:              req.Status,
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"id": id}})
	}
}

func adminUpdateManagedModelHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		ID                  int64           `json:"id"`
		PublicID            string          `json:"public_id"`
		OwnedBy             *string         `json:"owned_by"`
		InputUSDPer1M       decimal.Decimal `json:"input_usd_per_1m"`
		OutputUSDPer1M      decimal.Decimal `json:"output_usd_per_1m"`
		CacheInputUSDPer1M  decimal.Decimal `json:"cache_input_usd_per_1m"`
		CacheOutputUSDPer1M decimal.Decimal `json:"cache_output_usd_per_1m"`
		Status              int             `json:"status"`
	}

	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		statusOnly := strings.TrimSpace(c.Query("status_only")) == "true"

		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if req.ID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 不合法"})
			return
		}

		current, err := opts.Store.GetManagedModelByID(c.Request.Context(), req.ID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "模型不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		up := store.ManagedModelUpdate{
			ID:                  req.ID,
			PublicID:            strings.TrimSpace(req.PublicID),
			OwnedBy:             req.OwnedBy,
			InputUSDPer1M:       req.InputUSDPer1M,
			OutputUSDPer1M:      req.OutputUSDPer1M,
			CacheInputUSDPer1M:  req.CacheInputUSDPer1M,
			CacheOutputUSDPer1M: req.CacheOutputUSDPer1M,
			Status:              req.Status,
		}
		if statusOnly {
			up.PublicID = current.PublicID
			up.OwnedBy = current.OwnedBy
			up.InputUSDPer1M = current.InputUSDPer1M
			up.OutputUSDPer1M = current.OutputUSDPer1M
			up.CacheInputUSDPer1M = current.CacheInputUSDPer1M
			up.CacheOutputUSDPer1M = current.CacheOutputUSDPer1M
		}
		if up.PublicID == "" {
			up.PublicID = current.PublicID
		}
		if up.Status == 0 {
			up.Status = current.Status
		}

		if err := opts.Store.UpdateManagedModel(c.Request.Context(), up); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}

func adminDeleteManagedModelHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		id, err := strconv.ParseInt(strings.TrimSpace(c.Param("model_id")), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 不合法"})
			return
		}
		if err := opts.Store.DeleteManagedModel(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}

type channelModelView struct {
	ID            int64  `json:"id"`
	ChannelID     int64  `json:"channel_id"`
	PublicID      string `json:"public_id"`
	UpstreamModel string `json:"upstream_model"`
	Status        int    `json:"status"`
}

func adminListChannelModelsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}
		ms, err := opts.Store.ListChannelModelsByChannelID(c.Request.Context(), channelID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}
		out := make([]channelModelView, 0, len(ms))
		for _, m := range ms {
			out = append(out, channelModelView{
				ID:            m.ID,
				ChannelID:     m.ChannelID,
				PublicID:      m.PublicID,
				UpstreamModel: m.UpstreamModel,
				Status:        m.Status,
			})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func adminCreateChannelModelHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		PublicID      string `json:"public_id"`
		UpstreamModel string `json:"upstream_model"`
		Status        int    `json:"status"`
	}

	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		publicID := strings.TrimSpace(req.PublicID)
		upstreamModel := strings.TrimSpace(req.UpstreamModel)
		if publicID == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "public_id 不能为空"})
			return
		}
		if upstreamModel == "" {
			upstreamModel = publicID
		}
		if req.Status == 0 {
			req.Status = 1
		}
		id, err := opts.Store.CreateChannelModel(c.Request.Context(), store.ChannelModelCreate{
			ChannelID:     channelID,
			PublicID:      publicID,
			UpstreamModel: upstreamModel,
			Status:        req.Status,
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"id": id}})
	}
}

func adminUpdateChannelModelHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		ID            int64  `json:"id"`
		PublicID      string `json:"public_id"`
		UpstreamModel string `json:"upstream_model"`
		Status        int    `json:"status"`
	}

	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if req.ID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 不合法"})
			return
		}
		publicID := strings.TrimSpace(req.PublicID)
		upstreamModel := strings.TrimSpace(req.UpstreamModel)
		if publicID == "" || upstreamModel == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "public_id/upstream_model 不能为空"})
			return
		}
		if req.Status == 0 {
			req.Status = 1
		}
		if err := opts.Store.UpdateChannelModel(c.Request.Context(), store.ChannelModelUpdate{
			ID:            req.ID,
			ChannelID:     channelID,
			PublicID:      publicID,
			UpstreamModel: upstreamModel,
			Status:        req.Status,
		}); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}

func adminDeleteChannelModelHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		_, err := strconv.ParseInt(strings.TrimSpace(c.Param("channel_id")), 10, 64)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}
		id, err := strconv.ParseInt(strings.TrimSpace(c.Param("binding_id")), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "binding_id 不合法"})
			return
		}
		if err := opts.Store.DeleteChannelModel(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}
