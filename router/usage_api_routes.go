package router

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/store"
)

type usageWindowAPI struct {
	Window             string          `json:"window"`
	Since              time.Time       `json:"since"`
	Until              time.Time       `json:"until"`
	Requests           int64           `json:"requests"`
	Tokens             int64           `json:"tokens"`
	RPM                int64           `json:"rpm"`
	TPM                int64           `json:"tpm"`
	InputTokens        int64           `json:"input_tokens"`
	OutputTokens       int64           `json:"output_tokens"`
	CachedInputTokens  int64           `json:"cached_input_tokens"`
	CachedOutputTokens int64           `json:"cached_output_tokens"`
	CacheRatio         float64         `json:"cache_ratio"`
	UsedUSD            decimal.Decimal `json:"used_usd"`
	CommittedUSD       decimal.Decimal `json:"committed_usd"`
	ReservedUSD        decimal.Decimal `json:"reserved_usd"`
	LimitUSD           decimal.Decimal `json:"limit_usd"`
	RemainingUSD       decimal.Decimal `json:"remaining_usd"`
}

type subscriptionAPI struct {
	Active   bool      `json:"active"`
	PlanName string    `json:"plan_name,omitempty"`
	StartAt  time.Time `json:"start_at,omitempty"`
	EndAt    time.Time `json:"end_at,omitempty"`
}

type usageWindowsAPIResponse struct {
	TimeZone     string           `json:"time_zone"`
	Now          time.Time        `json:"now"`
	Subscription subscriptionAPI  `json:"subscription"`
	Windows      []usageWindowAPI `json:"windows"`
}

type usageEventsAPIResponse struct {
	Events       []usageEventAPI `json:"events"`
	NextBeforeID *int64          `json:"next_before_id,omitempty"`
}

type usageTimeSeriesPointAPI struct {
	Bucket               string  `json:"bucket"`
	Requests             int64   `json:"requests"`
	Tokens               int64   `json:"tokens"`
	CommittedUSD         float64 `json:"committed_usd"`
	CacheRatio           float64 `json:"cache_ratio"`
	AvgFirstTokenLatency float64 `json:"avg_first_token_latency"`
	TokensPerSecond      float64 `json:"tokens_per_second"`
}

type usageTimeSeriesAPIResponse struct {
	TimeZone    string                    `json:"time_zone"`
	Start       string                    `json:"start"`
	End         string                    `json:"end"`
	Granularity string                    `json:"granularity"`
	Points      []usageTimeSeriesPointAPI `json:"points"`
}

func setUsageAPIRoutes(r gin.IRoutes, opts Options) {
	authn := requireUserSession(opts)

	r.GET("/usage/windows", authn, usageWindowsHandler(opts))
	r.GET("/usage/events", authn, usageEventsHandler(opts))
	r.GET("/usage/events/:event_id/detail", authn, usageEventDetailHandler(opts))
	r.GET("/usage/timeseries", authn, usageTimeSeriesHandler(opts))
}

type usageEventAPI struct {
	ID                 int64           `json:"id"`
	Time               time.Time       `json:"time"`
	RequestID          string          `json:"request_id"`
	Endpoint           *string         `json:"endpoint,omitempty"`
	Method             *string         `json:"method,omitempty"`
	TokenID            int64           `json:"token_id"`
	State              string          `json:"state"`
	Model              *string         `json:"model,omitempty"`
	InputTokens        *int64          `json:"input_tokens,omitempty"`
	CachedInputTokens  *int64          `json:"cached_input_tokens,omitempty"`
	OutputTokens       *int64          `json:"output_tokens,omitempty"`
	CachedOutputTokens *int64          `json:"cached_output_tokens,omitempty"`
	ReservedUSD        decimal.Decimal `json:"reserved_usd"`
	CommittedUSD       decimal.Decimal `json:"committed_usd"`
	ReserveExpiresAt   time.Time       `json:"reserve_expires_at"`
	StatusCode         int             `json:"status_code"`
	LatencyMS          int             `json:"latency_ms"`
	ErrorClass         *string         `json:"error_class,omitempty"`
	ErrorMessage       *string         `json:"error_message,omitempty"`
	IsStream           bool            `json:"is_stream"`
	RequestBytes       int64           `json:"request_bytes"`
	ResponseBytes      int64           `json:"response_bytes"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

func usageWindowsHandler(opts Options) gin.HandlerFunc {
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

		loc, tzName, ok := usageRequestLocation(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "tz 不合法（需为 IANA 时区名，如 Asia/Shanghai）"})
			return
		}
		now := time.Now().UTC()
		since, until, sinceLocal, untilLocal, ok := parseDateRangeInLocation(now, strings.TrimSpace(c.Query("start")), strings.TrimSpace(c.Query("end")), loc)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "start/end 不合法（格式：YYYY-MM-DD）"})
			return
		}

		subs, err := opts.Store.ListActiveSubscriptionsWithPlans(c.Request.Context(), userID, now)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "订阅查询失败"})
			return
		}

		var resp usageWindowsAPIResponse
		resp.TimeZone = tzName
		resp.Now = now.In(loc)
		if len(subs) > 0 {
			resp.Subscription = subscriptionAPI{
				Active:   true,
				PlanName: subs[0].Plan.Name,
				StartAt:  subs[0].Subscription.StartAt,
				EndAt:    subs[0].Subscription.EndAt,
			}
		}

		committed, reserved, err := opts.Store.SumCommittedAndReservedUSDRange(c.Request.Context(), store.UsageSumWithReservedRangeInput{
			UserID: userID,
			Since:  since,
			Until:  until,
			Now:    now,
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "用量汇总失败"})
			return
		}

		tokenStats, err := opts.Store.GetUsageTokenStatsByUserRange(c.Request.Context(), userID, since, until)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "Token 统计失败"})
			return
		}
		recentSince := now.Add(-time.Minute)
		recentStats, err := opts.Store.GetUsageTokenStatsByUserRange(c.Request.Context(), userID, recentSince, now)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "实时速率统计失败"})
			return
		}

		resp.Windows = append(resp.Windows, usageWindowAPI{
			Window:             "range",
			Since:              sinceLocal,
			Until:              untilLocal,
			Requests:           tokenStats.Requests,
			Tokens:             tokenStats.Tokens,
			RPM:                recentStats.Requests,
			TPM:                recentStats.Tokens,
			InputTokens:        tokenStats.InputTokens,
			OutputTokens:       tokenStats.OutputTokens,
			CachedInputTokens:  tokenStats.CachedInputTokens,
			CachedOutputTokens: tokenStats.CachedOutputTokens,
			CacheRatio:         tokenStats.CacheRatio,
			UsedUSD:            committed.Add(reserved),
			CommittedUSD:       committed,
			ReservedUSD:        reserved,
			LimitUSD:           decimal.Zero,
			RemainingUSD:       decimal.Zero,
		})

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": resp})
	}
}

func usageEventsHandler(opts Options) gin.HandlerFunc {
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

		limit := 100
		if v := strings.TrimSpace(c.Query("limit")); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "limit 不合法"})
				return
			}
			limit = n
		}
		if limit <= 0 {
			limit = 100
		}
		if limit > 500 {
			limit = 500
		}

		var beforeID *int64
		if v := strings.TrimSpace(c.Query("before_id")); v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil || id <= 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "before_id 不合法"})
				return
			}
			beforeID = &id
		}

		startStr := strings.TrimSpace(c.Query("start"))
		endStr := strings.TrimSpace(c.Query("end"))
		useRange := startStr != "" || endStr != ""

		var events []store.UsageEvent
		var err error
		if useRange {
			loc, _, ok := usageRequestLocation(c)
			if !ok {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "tz 不合法（需为 IANA 时区名，如 Asia/Shanghai）"})
				return
			}
			now := time.Now().UTC()
			since, until, _, _, ok := parseDateRangeInLocation(now, startStr, endStr, loc)
			if !ok {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "start/end 不合法（格式：YYYY-MM-DD）"})
				return
			}
			events, err = opts.Store.ListUsageEventsByUserRange(c.Request.Context(), userID, since, until, limit, beforeID, nil)
		} else {
			events, err = opts.Store.ListUsageEventsByUser(c.Request.Context(), userID, limit, beforeID)
		}
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		resp := usageEventsAPIResponse{
			Events: make([]usageEventAPI, 0, len(events)),
		}
		for _, e := range events {
			errClass := e.ErrorClass
			errMsg := e.ErrorMessage
			if errClass != nil && strings.TrimSpace(*errClass) == "client_disconnect" {
				errClass = nil
				errMsg = nil
			}

			resp.Events = append(resp.Events, usageEventAPI{
				ID:                 e.ID,
				Time:               e.Time,
				RequestID:          e.RequestID,
				Endpoint:           e.Endpoint,
				Method:             e.Method,
				TokenID:            e.TokenID,
				State:              e.State,
				Model:              e.Model,
				InputTokens:        e.InputTokens,
				CachedInputTokens:  e.CachedInputTokens,
				OutputTokens:       e.OutputTokens,
				CachedOutputTokens: e.CachedOutputTokens,
				ReservedUSD:        e.ReservedUSD,
				CommittedUSD:       e.CommittedUSD,
				ReserveExpiresAt:   e.ReserveExpiresAt,
				StatusCode:         e.StatusCode,
				LatencyMS:          e.LatencyMS,
				ErrorClass:         errClass,
				ErrorMessage:       errMsg,
				IsStream:           e.IsStream,
				RequestBytes:       e.RequestBytes,
				ResponseBytes:      e.ResponseBytes,
				CreatedAt:          e.CreatedAt,
				UpdatedAt:          e.UpdatedAt,
			})
		}
		if len(events) > 0 {
			next := events[len(events)-1].ID
			resp.NextBeforeID = &next
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": resp})
	}
}

type usageEventDetailAPIResponse struct {
	EventID          int64                          `json:"event_id"`
	PricingBreakdown *usageEventPricingBreakdownAPI `json:"pricing_breakdown,omitempty"`
}

func usageEventDetailHandler(opts Options) gin.HandlerFunc {
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

		idStr := strings.TrimSpace(c.Param("event_id"))
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "event_id 不合法"})
			return
		}

		ev, err := opts.Store.GetUsageEvent(c.Request.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "not found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}
		if ev.UserID != userID {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "not found"})
			return
		}
		pricingBreakdown, err := buildUsageEventPricingBreakdown(c.Request.Context(), opts.Store, ev)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": usageEventDetailAPIResponse{
			EventID:          id,
			PricingBreakdown: &pricingBreakdown,
		}})
	}
}

func usageTimeSeriesHandler(opts Options) gin.HandlerFunc {
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

		loc, tzName, ok := usageRequestLocation(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "tz 不合法（需为 IANA 时区名，如 Asia/Shanghai）"})
			return
		}
		now := time.Now().UTC()
		startStr := strings.TrimSpace(c.Query("start"))
		endStr := strings.TrimSpace(c.Query("end"))
		since, until, sinceLocal, untilLocal, ok := parseDateRangeInLocation(now, startStr, endStr, loc)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "start/end 不合法（格式：YYYY-MM-DD）"})
			return
		}
		startResp := sinceLocal.Format("2006-01-02")
		endResp := untilLocal.Add(-time.Second).Format("2006-01-02")

		granularity := strings.TrimSpace(strings.ToLower(c.Query("granularity")))
		if granularity == "" {
			granularity = "hour"
		}
		if granularity != "hour" && granularity != "day" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "granularity 仅支持 hour/day"})
			return
		}

		rows, err := opts.Store.GetUserUsageTimeSeriesRange(c.Request.Context(), userID, since, until, granularity)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询用户时间序列失败"})
			return
		}
		points := make([]usageTimeSeriesPointAPI, 0, len(rows))
		for _, row := range rows {
			points = append(points, usageTimeSeriesPointAPI{
				Bucket:               row.Time.In(loc).Format("2006-01-02 15:04"),
				Requests:             row.Requests,
				Tokens:               row.Tokens,
				CommittedUSD:         row.CommittedUSD.InexactFloat64(),
				CacheRatio:           row.CacheRatio * 100,
				AvgFirstTokenLatency: row.AvgFirstTokenMS,
				TokensPerSecond:      row.OutputTokensPerSec,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": usageTimeSeriesAPIResponse{
				TimeZone:    tzName,
				Start:       startResp,
				End:         endResp,
				Granularity: granularity,
				Points:      points,
			},
		})
	}
}

func usageRequestLocation(c *gin.Context) (*time.Location, string, bool) {
	tz := normalizeAdminTimeZoneName(strings.TrimSpace(c.Query("tz")))
	if tz == "" {
		return time.UTC, "UTC", true
	}
	if tz == "UTC" {
		return time.UTC, "UTC", true
	}
	loc, err := time.LoadLocation(tz)
	if err != nil || loc == nil {
		return nil, "", false
	}
	return loc, tz, true
}

func parseDateRangeInLocation(nowUTC time.Time, startStr, endStr string, loc *time.Location) (sinceUTC time.Time, untilUTC time.Time, sinceLocal time.Time, untilLocal time.Time, ok bool) {
	if loc == nil {
		loc = time.UTC
	}
	nowLocal := nowUTC.In(loc)
	todayStartLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	todayStr := todayStartLocal.Format("2006-01-02")

	if startStr == "" {
		startStr = todayStr
	}
	if endStr == "" {
		endStr = startStr
	}

	sinceLocal, err := time.ParseInLocation("2006-01-02", startStr, loc)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, time.Time{}, false
	}
	endDateLocal, err := time.ParseInLocation("2006-01-02", endStr, loc)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, time.Time{}, false
	}
	if sinceLocal.After(endDateLocal) {
		return time.Time{}, time.Time{}, time.Time{}, time.Time{}, false
	}
	if endDateLocal.After(todayStartLocal) {
		endDateLocal = todayStartLocal
		endStr = todayStr
	}
	if sinceLocal.After(endDateLocal) {
		return time.Time{}, time.Time{}, time.Time{}, time.Time{}, false
	}

	untilLocal = endDateLocal.Add(24 * time.Hour)
	if endStr == todayStr {
		untilLocal = nowLocal
	}
	return sinceLocal.UTC(), untilLocal.UTC(), sinceLocal, untilLocal, true
}
