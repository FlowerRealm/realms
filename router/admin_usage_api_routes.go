package router

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/metrics"
	"realms/internal/store"
)

type adminUsageWindowView struct {
	Window               string `json:"window"`
	Since                string `json:"since"`
	Until                string `json:"until"`
	Requests             int64  `json:"requests"`
	Tokens               int64  `json:"tokens"`
	InputTokens          int64  `json:"input_tokens"`
	OutputTokens         int64  `json:"output_tokens"`
	CachedTokens         int64  `json:"cached_tokens"`
	CacheRatio           string `json:"cache_ratio"`
	RPM                  string `json:"rpm"`
	TPM                  string `json:"tpm"`
	AvgFirstTokenLatency string `json:"avg_first_token_latency"`
	TokensPerSecond      string `json:"tokens_per_second"`
	CommittedUSD         string `json:"committed_usd"`
	ReservedUSD          string `json:"reserved_usd"`
	TotalUSD             string `json:"total_usd"`
}

type adminUsageUserView struct {
	UserID       int64  `json:"user_id"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	Status       int    `json:"status"`
	CommittedUSD string `json:"committed_usd"`
	ReservedUSD  string `json:"reserved_usd"`
}

type adminUsageEventView struct {
	ID                  int64  `json:"id"`
	Time                string `json:"time"`
	UserID              int64  `json:"user_id"`
	UserEmail           string `json:"user_email"`
	Endpoint            string `json:"endpoint"`
	Method              string `json:"method"`
	Model               string `json:"model"`
	Account             string `json:"account"`
	StatusCode          string `json:"status_code"`
	LatencyMS           string `json:"latency_ms"`
	FirstTokenLatencyMS string `json:"first_token_latency_ms"`
	TokensPerSecond     string `json:"tokens_per_second"`
	InputTokens         string `json:"input_tokens"`
	OutputTokens        string `json:"output_tokens"`
	CachedTokens        string `json:"cached_tokens"`
	RequestBytes        string `json:"request_bytes"`
	ResponseBytes       string `json:"response_bytes"`
	CostUSD             string `json:"cost_usd"`
	StateLabel          string `json:"state_label"`
	StateBadgeClass     string `json:"state_badge_class"`
	IsStream            bool   `json:"is_stream"`
	UpstreamChannelID   string `json:"upstream_channel_id"`
	UpstreamChannelName string `json:"upstream_channel_name"`
	RequestID           string `json:"request_id"`
	Error               string `json:"error"`
	ErrorClass          string `json:"error_class"`
	ErrorMessage        string `json:"error_message"`
}

type adminUsagePageResponse struct {
	AdminTimeZone string `json:"admin_time_zone"`
	Now           string `json:"now"`
	Start         string `json:"start"`
	End           string `json:"end"`
	Limit         int    `json:"limit"`

	Window   adminUsageWindowView  `json:"window"`
	TopUsers []adminUsageUserView  `json:"top_users"`
	Events   []adminUsageEventView `json:"events"`

	NextBeforeID *int64 `json:"next_before_id,omitempty"`
	PrevAfterID  *int64 `json:"prev_after_id,omitempty"`
	CursorActive bool   `json:"cursor_active"`
}

type adminUsageTimeSeriesPointView struct {
	Bucket               string  `json:"bucket"`
	Requests             int64   `json:"requests"`
	Tokens               int64   `json:"tokens"`
	CommittedUSD         float64 `json:"committed_usd"`
	CacheRatio           float64 `json:"cache_ratio"`
	AvgFirstTokenLatency float64 `json:"avg_first_token_latency"`
	TokensPerSecond      float64 `json:"tokens_per_second"`
}

type adminUsageTimeSeriesResponse struct {
	AdminTimeZone string                          `json:"admin_time_zone"`
	Start         string                          `json:"start"`
	End           string                          `json:"end"`
	Granularity   string                          `json:"granularity"`
	Points        []adminUsageTimeSeriesPointView `json:"points"`
}

func setAdminUsageAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/usage", adminUsagePageHandler(opts))
	r.GET("/usage/events/:event_id/detail", adminUsageEventDetailHandler(opts))
	r.GET("/usage/timeseries", adminUsageTimeSeriesHandler(opts))
}

func adminUsageFeatureDisabled(c *gin.Context, opts Options) bool {
	if c == nil || opts.Store == nil {
		return false
	}
	if opts.Store.FeatureDisabledEffective(c.Request.Context(), opts.SelfMode, store.SettingFeatureDisableAdminUsage) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
		return true
	}
	return false
}

func adminUsagePageHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsageFeatureDisabled(c, opts) {
			return
		}

		loc, tzName := adminTimeLocation(c.Request.Context(), opts)

		nowUTC := time.Now().UTC()
		now := nowUTC.In(loc)
		todayStartLocal := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		todayStr := todayStartLocal.Format("2006-01-02")

		q := c.Request.URL.Query()
		startStr := strings.TrimSpace(q.Get("start"))
		endStr := strings.TrimSpace(q.Get("end"))

		limit := 50
		if v := strings.TrimSpace(q.Get("limit")); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "limit 不合法"})
				return
			}
			limit = n
		}
		if limit < 10 {
			limit = 10
		}
		if limit > 200 {
			limit = 200
		}

		if startStr == "" {
			startStr = todayStr
		}
		if endStr == "" {
			endStr = startStr
		}

		sinceLocal, err := time.ParseInLocation("2006-01-02", startStr, loc)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "start 不合法（格式：YYYY-MM-DD）"})
			return
		}
		endDateLocal, err := time.ParseInLocation("2006-01-02", endStr, loc)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "end 不合法（格式：YYYY-MM-DD）"})
			return
		}
		if sinceLocal.After(endDateLocal) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "start 不能晚于 end"})
			return
		}
		if endDateLocal.After(todayStartLocal) {
			endDateLocal = todayStartLocal
			endStr = todayStr
		}
		untilLocal := endDateLocal.AddDate(0, 0, 1)
		if endStr == todayStr {
			untilLocal = now
		}

		since := sinceLocal.UTC()
		until := untilLocal.UTC()

		committed, reserved, err := opts.Store.SumCommittedAndReservedUSDAllRange(c.Request.Context(), store.UsageSumAllWithReservedRangeInput{
			Since: since,
			Until: until,
			Now:   nowUTC,
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "用量汇总失败"})
			return
		}
		stats, err := opts.Store.GetGlobalUsageStatsRange(c.Request.Context(), since, until)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "统计失败"})
			return
		}
		// New API 口径：RPM/TPM 固定统计最近 60 秒，不随筛选区间变化。
		recentSince := nowUTC.Add(-time.Minute)
		recentStats, err := opts.Store.GetGlobalUsageStatsRange(c.Request.Context(), recentSince, nowUTC)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "实时速率统计失败"})
			return
		}
		rpm := metrics.FormatRatePerMinute(recentStats.Requests, time.Minute)
		tpm := metrics.FormatRatePerMinute(recentStats.Tokens, time.Minute)
		window := adminUsageWindowView{
			Window:               "统计区间",
			Since:                sinceLocal.Format("2006-01-02 15:04"),
			Until:                untilLocal.Format("2006-01-02 15:04"),
			Requests:             stats.Requests,
			Tokens:               stats.Tokens,
			InputTokens:          stats.InputTokens,
			OutputTokens:         stats.OutputTokens,
			CachedTokens:         stats.CachedInputTokens + stats.CachedOutputTokens,
			CacheRatio:           strconv.FormatFloat(stats.CacheRatio*100, 'f', 1, 64) + "%",
			RPM:                  rpm,
			TPM:                  tpm,
			AvgFirstTokenLatency: formatAvgFirstTokenLatency(stats.AvgFirstTokenMS, stats.FirstTokenSamples),
			TokensPerSecond:      formatTokensPerSecond(stats.OutputTokensPerSec),
			CommittedUSD:         formatUSDPlain(committed),
			ReservedUSD:          formatUSDPlain(reserved),
			TotalUSD:             formatUSDPlain(committed.Add(reserved)),
		}

		topUsers, err := opts.Store.ListUsageTopUsers(c.Request.Context(), store.UsageTopUsersInput{
			Since: since,
			Until: until,
			Now:   now,
			Limit: 50,
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "用户用量汇总失败"})
			return
		}
		topViews := make([]adminUsageUserView, 0, len(topUsers))
		for _, row := range topUsers {
			topViews = append(topViews, adminUsageUserView{
				UserID:       row.UserID,
				Email:        row.Email,
				Role:         row.Role,
				Status:       row.Status,
				CommittedUSD: formatUSDPlain(row.CommittedUSD),
				ReservedUSD:  formatUSDPlain(row.ReservedUSD),
			})
		}

		var beforeID *int64
		if v := strings.TrimSpace(q.Get("before_id")); v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil || id <= 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "before_id 不合法"})
				return
			}
			beforeID = &id
		}
		var afterID *int64
		if v := strings.TrimSpace(q.Get("after_id")); v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil || id <= 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "after_id 不合法"})
				return
			}
			afterID = &id
		}
		if beforeID != nil && afterID != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "before_id 与 after_id 不能同时使用"})
			return
		}

		events, err := opts.Store.ListUsageEventsWithUserRange(c.Request.Context(), since, until, limit, beforeID, afterID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询请求明细失败"})
			return
		}

		channelNameByID := map[int64]string{}
		channelTypeByID := map[int64]string{}
		if channels, err := opts.Store.ListUpstreamChannels(c.Request.Context()); err == nil {
			channelNameByID = make(map[int64]string, len(channels))
			channelTypeByID = make(map[int64]string, len(channels))
			for _, ch := range channels {
				channelNameByID[ch.ID] = ch.Name
				channelTypeByID[ch.ID] = ch.Type
			}
		}

		codexAccountByCredentialID := make(map[int64]string)
		eventViews := make([]adminUsageEventView, 0, len(events))
		for _, row := range events {
			e := row.Event
			endpoint := "-"
			if e.Endpoint != nil && strings.TrimSpace(*e.Endpoint) != "" {
				endpoint = *e.Endpoint
			}
			method := "-"
			if e.Method != nil && strings.TrimSpace(*e.Method) != "" {
				method = *e.Method
			}
			model := "-"
			if e.Model != nil && strings.TrimSpace(*e.Model) != "" {
				model = *e.Model
			}
			statusCode := "-"
			if e.StatusCode > 0 {
				statusCode = strconv.Itoa(e.StatusCode)
			}
			latencyMS := "-"
			if e.LatencyMS > 0 {
				latencyMS = strconv.Itoa(e.LatencyMS)
			}
			firstTokenLatencyMS := "-"
			if e.FirstTokenLatencyMS > 0 {
				firstTokenLatencyMS = strconv.Itoa(e.FirstTokenLatencyMS)
			}
			inTok := "-"
			if e.InputTokens != nil {
				inTok = strconv.FormatInt(*e.InputTokens, 10)
			}
			outTok := "-"
			if e.OutputTokens != nil {
				outTok = strconv.FormatInt(*e.OutputTokens, 10)
			}
			tokensPerSecond := "-"
			if e.OutputTokens != nil && *e.OutputTokens > 0 {
				decodeLatencyMS := int64(e.LatencyMS - e.FirstTokenLatencyMS)
				if decodeLatencyMS > 0 {
					tokensPerSecond = formatTokensPerSecond(float64(*e.OutputTokens) * 1000 / float64(decodeLatencyMS))
				}
			}
			var cached int64
			if e.CachedInputTokens != nil {
				cached += *e.CachedInputTokens
			}
			if e.CachedOutputTokens != nil {
				cached += *e.CachedOutputTokens
			}
			cachedTok := "-"
			if cached > 0 {
				cachedTok = strconv.FormatInt(cached, 10)
			}
			reqBytes := strconv.FormatInt(e.RequestBytes, 10)
			respBytes := strconv.FormatInt(e.ResponseBytes, 10)
			costUSD := decimal.Zero
			switch e.State {
			case store.UsageStateCommitted:
				costUSD = e.CommittedUSD
			case store.UsageStateReserved:
				costUSD = e.ReservedUSD
			}
			cost := formatUSDPlain(costUSD)
			if e.State == store.UsageStateReserved {
				cost += " (预留)"
			}
			stateLabel := e.State
			stateBadge := "bg-secondary-subtle text-secondary border border-secondary-subtle"
			switch e.State {
			case store.UsageStateCommitted:
				stateLabel = "已结算"
				stateBadge = "bg-success-subtle text-success border border-success-subtle"
			case store.UsageStateReserved:
				stateLabel = "预留中"
				stateBadge = "bg-warning-subtle text-warning border border-warning-subtle"
			case store.UsageStateVoid:
				stateLabel = "已作废"
				stateBadge = "bg-secondary-subtle text-secondary border border-secondary-subtle"
			case store.UsageStateExpired:
				stateLabel = "已过期"
				stateBadge = "bg-secondary-subtle text-secondary border border-secondary-subtle"
			}
			upstreamChannelID := "-"
			upstreamChannelName := ""
			upstreamChannelType := ""
			if e.UpstreamChannelID != nil && *e.UpstreamChannelID > 0 {
				upstreamChannelID = strconv.FormatInt(*e.UpstreamChannelID, 10)
				if name := strings.TrimSpace(channelNameByID[*e.UpstreamChannelID]); name != "" {
					upstreamChannelName = name
				}
				upstreamChannelType = strings.TrimSpace(channelTypeByID[*e.UpstreamChannelID])
			}
			account := "-"
			if upstreamChannelType == store.UpstreamTypeCodexOAuth && e.UpstreamCredID != nil && *e.UpstreamCredID > 0 {
				credID := *e.UpstreamCredID
				if v, ok := codexAccountByCredentialID[credID]; ok {
					account = v
				} else {
					resolved := "-"
					acc, err := opts.Store.GetCodexOAuthAccountByID(c.Request.Context(), credID)
					if err == nil {
						if id := strings.TrimSpace(acc.AccountID); id != "" {
							resolved = id
						}
					}
					codexAccountByCredentialID[credID] = resolved
					account = resolved
				}
				if account != "-" {
					model = account
				}
			}
			errClass := ""
			if e.ErrorClass != nil && strings.TrimSpace(*e.ErrorClass) != "" {
				errClass = strings.TrimSpace(*e.ErrorClass)
			}
			errMsg := ""
			if e.ErrorMessage != nil && strings.TrimSpace(*e.ErrorMessage) != "" {
				errMsg = strings.TrimSpace(*e.ErrorMessage)
			}
			if errClass == "client_disconnect" {
				errClass = ""
				errMsg = ""
			}
			errText := ""
			if errClass != "" {
				errText = errClass
			}
			if errMsg != "" {
				if errText == "" {
					errText = errMsg
				} else {
					errText = errText + " (" + errMsg + ")"
				}
			}

			eventViews = append(eventViews, adminUsageEventView{
				ID:                  e.ID,
				Time:                formatTimeIn(e.Time, "2006-01-02 15:04:05", loc),
				UserID:              e.UserID,
				UserEmail:           row.UserEmail,
				Endpoint:            endpoint,
				Method:              method,
				Model:               model,
				Account:             account,
				StatusCode:          statusCode,
				LatencyMS:           latencyMS,
				FirstTokenLatencyMS: firstTokenLatencyMS,
				TokensPerSecond:     tokensPerSecond,
				InputTokens:         inTok,
				OutputTokens:        outTok,
				CachedTokens:        cachedTok,
				RequestBytes:        reqBytes,
				ResponseBytes:       respBytes,
				CostUSD:             cost,
				StateLabel:          stateLabel,
				StateBadgeClass:     stateBadge,
				IsStream:            e.IsStream,
				UpstreamChannelID:   upstreamChannelID,
				UpstreamChannelName: upstreamChannelName,
				RequestID:           e.RequestID,
				Error:               errText,
				ErrorClass:          errClass,
				ErrorMessage:        errMsg,
			})
		}

		var nextBeforeID *int64
		if len(events) == limit && len(events) > 0 {
			next := events[len(events)-1].Event.ID
			nextBeforeID = &next
		}
		var prevAfterID *int64
		if len(events) > 0 {
			canPrev := beforeID != nil || (afterID != nil && len(events) == limit)
			if canPrev {
				prev := events[0].Event.ID
				prevAfterID = &prev
			}
		}

		resp := adminUsagePageResponse{
			AdminTimeZone: tzName,
			Now:           now.Format("2006-01-02 15:04"),
			Start:         startStr,
			End:           endStr,
			Limit:         limit,
			Window:        window,
			TopUsers:      topViews,
			Events:        eventViews,
			NextBeforeID:  nextBeforeID,
			PrevAfterID:   prevAfterID,
			CursorActive:  beforeID != nil || afterID != nil,
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": resp})
	}
}

func adminUsageEventDetailHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsageFeatureDisabled(c, opts) {
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
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "not found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
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

func adminUsageTimeSeriesHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsageFeatureDisabled(c, opts) {
			return
		}

		loc, tzName := adminTimeLocation(c.Request.Context(), opts)

		nowUTC := time.Now().UTC()
		nowLocal := nowUTC.In(loc)
		todayStartLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
		todayStr := todayStartLocal.Format("2006-01-02")

		q := c.Request.URL.Query()
		startStr := strings.TrimSpace(q.Get("start"))
		endStr := strings.TrimSpace(q.Get("end"))
		granularity := strings.TrimSpace(strings.ToLower(q.Get("granularity")))
		if granularity == "" {
			granularity = "hour"
		}
		if granularity != "hour" && granularity != "day" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "granularity 仅支持 hour/day"})
			return
		}
		if startStr == "" {
			startStr = todayStr
		}
		if endStr == "" {
			endStr = startStr
		}
		sinceLocal, err := time.ParseInLocation("2006-01-02", startStr, loc)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "start 不合法（格式：YYYY-MM-DD）"})
			return
		}
		endDateLocal, err := time.ParseInLocation("2006-01-02", endStr, loc)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "end 不合法（格式：YYYY-MM-DD）"})
			return
		}
		if sinceLocal.After(endDateLocal) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "start 不能晚于 end"})
			return
		}
		if endDateLocal.After(todayStartLocal) {
			endDateLocal = todayStartLocal
			endStr = todayStr
		}
		untilLocal := endDateLocal.AddDate(0, 0, 1)
		if endStr == todayStr {
			untilLocal = nowLocal
		}

		rows, err := opts.Store.GetGlobalUsageTimeSeriesRange(c.Request.Context(), sinceLocal.UTC(), untilLocal.UTC(), granularity)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询全站时间序列失败"})
			return
		}

		points := make([]adminUsageTimeSeriesPointView, 0, len(rows))
		for _, row := range rows {
			points = append(points, adminUsageTimeSeriesPointView{
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
			"data": adminUsageTimeSeriesResponse{
				AdminTimeZone: tzName,
				Start:         startStr,
				End:           endStr,
				Granularity:   granularity,
				Points:        points,
			},
		})
	}
}
