package router

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type adminMainGroupView struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Status      int     `json:"status"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type adminMainGroupSubgroupView struct {
	Subgroup  string `json:"subgroup"`
	Priority  int    `json:"priority"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func setAdminMainGroupAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/main-groups", adminListMainGroupsHandler(opts))
	r.POST("/main-groups", adminCreateMainGroupHandler(opts))
	r.GET("/main-groups/:group_name", adminGetMainGroupHandler(opts))
	r.PUT("/main-groups/:group_name", adminUpdateMainGroupHandler(opts))
	r.DELETE("/main-groups/:group_name", adminDeleteMainGroupHandler(opts))

	r.GET("/main-groups/:group_name/subgroups", adminListMainGroupSubgroupsHandler(opts))
	r.PUT("/main-groups/:group_name/subgroups", adminReplaceMainGroupSubgroupsHandler(opts))
}

func adminListMainGroupsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsersFeatureDisabled(c, opts) {
			return
		}

		rows, err := opts.Store.ListMainGroups(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		out := make([]adminMainGroupView, 0, len(rows))
		for _, row := range rows {
			out = append(out, adminMainGroupView{
				Name:        row.Name,
				Description: row.Description,
				Status:      row.Status,
				CreatedAt:   row.CreatedAt.Format("2006-01-02 15:04"),
				UpdatedAt:   row.UpdatedAt.Format("2006-01-02 15:04"),
			})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func adminCreateMainGroupHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Status      int     `json:"status"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsersFeatureDisabled(c, opts) {
			return
		}
		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "name 不能为空"})
			return
		}
		if req.Status != 0 && req.Status != 1 {
			req.Status = 1
		}
		if err := opts.Store.CreateMainGroup(c.Request.Context(), name, req.Description, req.Status); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已创建"})
	}
}

func adminGetMainGroupHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsersFeatureDisabled(c, opts) {
			return
		}
		name := strings.TrimSpace(c.Param("group_name"))
		if name == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_name 不能为空"})
			return
		}
		row, err := opts.Store.GetMainGroupByName(c.Request.Context(), name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": adminMainGroupView{
			Name:        row.Name,
			Description: row.Description,
			Status:      row.Status,
			CreatedAt:   row.CreatedAt.Format("2006-01-02 15:04"),
			UpdatedAt:   row.UpdatedAt.Format("2006-01-02 15:04"),
		}})
	}
}

func adminUpdateMainGroupHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Description *string `json:"description"`
		Status      int     `json:"status"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsersFeatureDisabled(c, opts) {
			return
		}
		name := strings.TrimSpace(c.Param("group_name"))
		if name == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_name 不能为空"})
			return
		}
		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if req.Status != 0 && req.Status != 1 {
			req.Status = 1
		}
		if err := opts.Store.UpdateMainGroup(c.Request.Context(), name, req.Description, req.Status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func adminDeleteMainGroupHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsersFeatureDisabled(c, opts) {
			return
		}
		name := strings.TrimSpace(c.Param("group_name"))
		if name == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_name 不能为空"})
			return
		}
		if err := opts.Store.DeleteMainGroup(c.Request.Context(), name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
	}
}

func adminListMainGroupSubgroupsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsersFeatureDisabled(c, opts) {
			return
		}
		name := strings.TrimSpace(c.Param("group_name"))
		if name == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_name 不能为空"})
			return
		}
		rows, err := opts.Store.ListMainGroupSubgroups(c.Request.Context(), name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		out := make([]adminMainGroupSubgroupView, 0, len(rows))
		for _, row := range rows {
			out = append(out, adminMainGroupSubgroupView{
				Subgroup:  row.Subgroup,
				Priority:  row.Priority,
				CreatedAt: row.CreatedAt.Format("2006-01-02 15:04"),
				UpdatedAt: row.UpdatedAt.Format("2006-01-02 15:04"),
			})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func adminReplaceMainGroupSubgroupsHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Subgroups []string `json:"subgroups"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsersFeatureDisabled(c, opts) {
			return
		}
		name := strings.TrimSpace(c.Param("group_name"))
		if name == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_name 不能为空"})
			return
		}
		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if err := opts.Store.ReplaceMainGroupSubgroups(c.Request.Context(), name, req.Subgroups); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}
