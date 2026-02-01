package router

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/icons"
	"realms/internal/metrics"
	"realms/internal/store"
)

type dashboardSubscriptionWindow struct {
	Window      string `json:"window"`
	UsedUSD     string `json:"used_usd"`
	LimitUSD    string `json:"limit_usd"`
	UsedPercent int    `json:"used_percent"`
}

type dashboardSubscription struct {
	Active       bool                          `json:"active"`
	PlanName     string                        `json:"plan_name,omitempty"`
	EndAt        string                        `json:"end_at,omitempty"`
	UsageWindows []dashboardSubscriptionWindow `json:"usage_windows,omitempty"`
}

type dashboardModelUsage struct {
	Model        string `json:"model"`
	IconURL      string `json:"icon_url,omitempty"`
	Color        string `json:"color"`
	Requests     int64  `json:"requests"`
	Tokens       int64  `json:"tokens"`
	CommittedUSD string `json:"committed_usd"`
}

type dashboardTimeSeriesUsage struct {
	Label        string  `json:"label"`
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	CommittedUSD float64 `json:"committed_usd"`
}

type dashboardCharts struct {
	ModelStats      []dashboardModelUsage      `json:"model_stats"`
	TimeSeriesStats []dashboardTimeSeriesUsage `json:"time_series_stats"`
}

type dashboardResponse struct {
	TodayUsageUSD            string                 `json:"today_usage_usd"`
	TodayRequests            int64                  `json:"today_requests"`
	TodayTokens              int64                  `json:"today_tokens"`
	TodayRPM                 string                 `json:"today_rpm"`
	TodayTPM                 string                 `json:"today_tpm"`
	UnreadAnnouncementsCount int64                  `json:"unread_announcements_count"`
	Subscription             *dashboardSubscription `json:"subscription,omitempty"`
	Charts                   dashboardCharts        `json:"charts"`
}

func setDashboardAPIRoutes(r gin.IRoutes, opts Options) {
	authn := requireUserSession(opts)
	r.GET("/dashboard", authn, dashboardHandler(opts))
}

func dashboardHandler(opts Options) gin.HandlerFunc {
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

		now := time.Now()

		// Calculate today's usage (since midnight Asia/Shanghai) to match legacy SSR behavior.
		loc, _ := time.LoadLocation("Asia/Shanghai")
		if loc == nil {
			loc = time.FixedZone("CST", 8*60*60)
		}
		todayLocal := now.In(loc)
		todayStart := time.Date(todayLocal.Year(), todayLocal.Month(), todayLocal.Day(), 0, 0, 0, 0, loc)

		todayCommitted, todayReserved, err := opts.Store.SumCommittedAndReservedUSD(c.Request.Context(), store.UsageSumWithReservedInput{
			UserID: userID,
			Since:  todayStart,
			Now:    now,
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "用量汇总失败"})
			return
		}
		todayUsageUSD := formatUSD(todayCommitted.Add(todayReserved))

		todayStats, _ := opts.Store.GetUsageTokenStatsByUserRange(c.Request.Context(), userID, todayStart, now)

		// RPM/TPM: 1-minute moving window.
		window := time.Minute
		oneMinAgo := now.Add(-window)
		recentStats, _ := opts.Store.GetUsageTokenStatsByUserRange(c.Request.Context(), userID, oneMinAgo, now)
		rpm := metrics.FormatRatePerMinute(recentStats.Requests, window)
		tpm := metrics.FormatRatePerMinute(recentStats.Tokens, window)

		// Subscription summary (closest end_at among active subscriptions).
		var subView *dashboardSubscription
		subs, err := opts.Store.ListNonExpiredSubscriptionsWithPlans(c.Request.Context(), userID, now)
		if err == nil && len(subs) > 0 {
			var active *store.SubscriptionWithPlan
			for _, row := range subs {
				if row.Subscription.StartAt.After(now) {
					continue
				}
				if active == nil || row.Subscription.EndAt.Before(active.Subscription.EndAt) {
					tmp := row
					active = &tmp
				}
			}
			if active != nil {
				subView = &dashboardSubscription{
					Active:   true,
					PlanName: active.Plan.Name,
					EndAt:    active.Subscription.EndAt.Format("2006-01-02 15:04"),
				}

				// Match legacy dashboard: show 5h window percent only when limit is set.
				since := now.Add(-5 * time.Hour)
				if active.Subscription.StartAt.After(since) {
					since = active.Subscription.StartAt
				}
				committed, reserved, err := opts.Store.SumCommittedAndReservedUSDBySubscription(c.Request.Context(), store.UsageSumWithReservedBySubscriptionInput{
					UserID:         userID,
					SubscriptionID: active.Subscription.ID,
					Since:          since,
					Now:            now,
				})
				if err == nil && !active.Plan.Limit5HUSD.IsZero() {
					used := committed.Add(reserved)
					percent := int(used.Mul(decimal.NewFromInt(100)).Div(active.Plan.Limit5HUSD).IntPart())
					if percent > 100 {
						percent = 100
					}
					subView.UsageWindows = append(subView.UsageWindows, dashboardSubscriptionWindow{
						Window:      "5小时",
						UsedUSD:     formatUSD(used),
						LimitUSD:    formatUSD(active.Plan.Limit5HUSD),
						UsedPercent: percent,
					})
				}
			}
		}

		// Charts data (Today)
		modelStats, _ := opts.Store.GetUsageStatsByModelRange(c.Request.Context(), userID, todayStart, now)
		timeStats, _ := opts.Store.GetUsageTimeSeriesRange(c.Request.Context(), userID, todayStart, now)

		unreadCount, _ := opts.Store.CountUnreadAnnouncements(c.Request.Context(), userID)

		timeMap := make(map[string]store.TimeSeriesUsageStats, len(timeStats))
		for _, ts := range timeStats {
			timeMap[ts.Time.In(loc).Format("15:00")] = ts
		}

		palette := []string{"#6366f1", "#10b981", "#f59e0b", "#ef4444", "#8b5cf6", "#ec4899", "#06b6d4", "#84cc16", "#14b8a6", "#64748b"}
		chartView := dashboardCharts{
			ModelStats:      make([]dashboardModelUsage, 0, len(modelStats)),
			TimeSeriesStats: make([]dashboardTimeSeriesUsage, 0, 24),
		}
		for i, m := range modelStats {
			color := palette[i%len(palette)]
			chartView.ModelStats = append(chartView.ModelStats, dashboardModelUsage{
				Model:        m.Model,
				IconURL:      strings.TrimSpace(icons.ModelIconURL(m.Model, "")),
				Color:        color,
				Requests:     m.Requests,
				Tokens:       m.Tokens,
				CommittedUSD: formatUSDPlain(m.CommittedUSD),
			})
		}
		for i := 0; i < 24; i++ {
			hr := time.Date(todayStart.Year(), todayStart.Month(), todayStart.Day(), i, 0, 0, 0, loc)
			label := hr.Format("15:00")
			if ts, ok := timeMap[label]; ok {
				f, _ := ts.CommittedUSD.Float64()
				chartView.TimeSeriesStats = append(chartView.TimeSeriesStats, dashboardTimeSeriesUsage{
					Label:        label,
					Requests:     ts.Requests,
					Tokens:       ts.Tokens,
					CommittedUSD: f,
				})
			} else {
				chartView.TimeSeriesStats = append(chartView.TimeSeriesStats, dashboardTimeSeriesUsage{
					Label:        label,
					Requests:     0,
					Tokens:       0,
					CommittedUSD: 0,
				})
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": dashboardResponse{
				TodayUsageUSD:            todayUsageUSD,
				TodayRequests:            todayStats.Requests,
				TodayTokens:              todayStats.Tokens,
				TodayRPM:                 rpm,
				TodayTPM:                 tpm,
				UnreadAnnouncementsCount: unreadCount,
				Subscription:             subView,
				Charts:                   chartView,
			},
		})
	}
}

func formatDecimalPlain(d decimal.Decimal, scale int32) string {
	if scale < 0 {
		scale = 0
	}
	d = d.Truncate(scale)
	s := d.StringFixed(scale)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

func formatUSDPlain(usd decimal.Decimal) string {
	return formatDecimalPlain(usd, store.USDScale)
}

func formatUSD(usd decimal.Decimal) string {
	if usd.IsNegative() {
		return "-$" + formatUSDPlain(usd.Abs())
	}
	return "$" + formatUSDPlain(usd)
}
