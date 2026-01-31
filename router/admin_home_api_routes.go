package router

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type adminHomeStatsView struct {
	UsersCount     int64 `json:"users_count"`
	ChannelsCount  int64 `json:"channels_count"`
	EndpointsCount int64 `json:"endpoints_count"`

	RequestsToday     int64 `json:"requests_today"`
	TokensToday       int64 `json:"tokens_today"`
	InputTokensToday  int64 `json:"input_tokens_today"`
	OutputTokensToday int64 `json:"output_tokens_today"`

	CostToday string `json:"cost_today"`
}

type adminHomeResponse struct {
	AdminTimeZone string             `json:"admin_time_zone"`
	Stats         adminHomeStatsView `json:"stats"`
}

func setAdminHomeAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/home", adminHomeHandler(opts))
}

func adminHomeHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		ctx := c.Request.Context()
		usersCount, _ := opts.Store.CountUsers(ctx)
		channelsCount, _ := opts.Store.CountUpstreamChannels(ctx)
		endpointsCount, _ := opts.Store.CountUpstreamEndpoints(ctx)

		loc, tzName := adminTimeLocation(ctx, opts)
		nowUTC := time.Now().UTC()
		now := nowUTC.In(loc)
		todayStartLocal := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		todayStartUTC := todayStartLocal.UTC()

		usageStats, _ := opts.Store.GetGlobalUsageStats(ctx, todayStartUTC)

		out := adminHomeResponse{
			AdminTimeZone: tzName,
			Stats: adminHomeStatsView{
				UsersCount:        usersCount,
				ChannelsCount:     channelsCount,
				EndpointsCount:    endpointsCount,
				RequestsToday:     usageStats.Requests,
				TokensToday:       usageStats.Tokens,
				InputTokensToday:  usageStats.InputTokens,
				OutputTokensToday: usageStats.OutputTokens,
				CostToday:         formatUSDPlain(usageStats.CostUSD),
			},
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}
