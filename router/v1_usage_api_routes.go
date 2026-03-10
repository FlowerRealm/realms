package router

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/store"
)

type httpAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func writeHTTPAPIJSON(w http.ResponseWriter, success bool, message string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(httpAPIResponse{
		Success: success,
		Message: message,
		Data:    data,
	})
}

func usageRequestLocationFromRequest(r *http.Request) (*time.Location, string, bool) {
	var tz string
	if r != nil && r.URL != nil {
		tz = normalizeAdminTimeZoneName(strings.TrimSpace(r.URL.Query().Get("tz")))
	}
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

func rejectTokenQueryParamsForDataPlane(w http.ResponseWriter, r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	q := r.URL.Query()
	if strings.TrimSpace(q.Get("token_id")) != "" || strings.TrimSpace(q.Get("token_ids")) != "" {
		writeHTTPAPIJSON(w, false, "token_id/token_ids 不支持（数据面仅允许查询当前 key）", nil)
		return true
	}
	return false
}

func v1UsageWindowsHTTPHandler(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if opts.Store == nil {
			writeHTTPAPIJSON(w, false, "store 未初始化", nil)
			return
		}
		if rejectTokenQueryParamsForDataPlane(w, r) {
			return
		}

		p, ok := auth.PrincipalFromContext(r.Context())
		if !ok || p.TokenID == nil || *p.TokenID <= 0 || p.UserID <= 0 {
			http.Error(w, "未提供 Token", http.StatusUnauthorized)
			return
		}
		tokenID := *p.TokenID
		userID := p.UserID

		loc, tzName, ok := usageRequestLocationFromRequest(r)
		if !ok {
			writeHTTPAPIJSON(w, false, "tz 不合法（需为 IANA 时区名，如 Asia/Shanghai）", nil)
			return
		}

		now := time.Now().UTC()
		startStr := ""
		endStr := ""
		if r != nil && r.URL != nil {
			startStr = strings.TrimSpace(r.URL.Query().Get("start"))
			endStr = strings.TrimSpace(r.URL.Query().Get("end"))
		}
		since, until, sinceLocal, untilLocal, ok := parseDateRangeInLocation(now, startStr, endStr, loc)
		if !ok {
			writeHTTPAPIJSON(w, false, "start/end 不合法（格式：YYYY-MM-DD）", nil)
			return
		}

		subs, err := opts.Store.ListActiveSubscriptionsWithPlans(r.Context(), userID, now)
		if err != nil {
			writeHTTPAPIJSON(w, false, "订阅查询失败", nil)
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

		committed, reserved, err := opts.Store.SumCommittedAndReservedUSDRangeByToken(r.Context(), store.UsageSumWithReservedRangeByTokenInput{
			TokenID: tokenID,
			Since:   since,
			Until:   until,
			Now:     now,
		})
		if err != nil {
			writeHTTPAPIJSON(w, false, "用量汇总失败", nil)
			return
		}

		tokenStats, err := opts.Store.GetUsageTokenStatsByTokenRange(r.Context(), tokenID, since, until)
		if err != nil {
			writeHTTPAPIJSON(w, false, "Token 统计失败", nil)
			return
		}
		recentSince := now.Add(-time.Minute)
		recentStats, err := opts.Store.GetUsageTokenStatsByTokenRange(r.Context(), tokenID, recentSince, now)
		if err != nil {
			writeHTTPAPIJSON(w, false, "实时速率统计失败", nil)
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

		writeHTTPAPIJSON(w, true, "", resp)
	}
}

func v1UsageEventsHTTPHandler(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if opts.Store == nil {
			writeHTTPAPIJSON(w, false, "store 未初始化", nil)
			return
		}
		if rejectTokenQueryParamsForDataPlane(w, r) {
			return
		}

		p, ok := auth.PrincipalFromContext(r.Context())
		if !ok || p.TokenID == nil || *p.TokenID <= 0 {
			http.Error(w, "未提供 Token", http.StatusUnauthorized)
			return
		}
		tokenID := *p.TokenID

		limit := 100
		var beforeID *int64
		startStr := ""
		endStr := ""
		if r != nil && r.URL != nil {
			q := r.URL.Query()
			if v := strings.TrimSpace(q.Get("limit")); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					limit = n
				} else {
					writeHTTPAPIJSON(w, false, "limit 不合法", nil)
					return
				}
			}
			if v := strings.TrimSpace(q.Get("before_id")); v != "" {
				id, err := strconv.ParseInt(v, 10, 64)
				if err != nil || id <= 0 {
					writeHTTPAPIJSON(w, false, "before_id 不合法", nil)
					return
				}
				beforeID = &id
			}
			startStr = strings.TrimSpace(q.Get("start"))
			endStr = strings.TrimSpace(q.Get("end"))
		}
		if limit <= 0 {
			limit = 100
		}
		if limit > 500 {
			limit = 500
		}

		useRange := startStr != "" || endStr != ""
		var events []store.UsageEvent
		var err error
		if useRange {
			loc, _, ok := usageRequestLocationFromRequest(r)
			if !ok {
				writeHTTPAPIJSON(w, false, "tz 不合法（需为 IANA 时区名，如 Asia/Shanghai）", nil)
				return
			}
			now := time.Now().UTC()
			since, until, _, _, ok := parseDateRangeInLocation(now, startStr, endStr, loc)
			if !ok {
				writeHTTPAPIJSON(w, false, "start/end 不合法（格式：YYYY-MM-DD）", nil)
				return
			}
			events, err = opts.Store.ListUsageEventsByTokenRange(r.Context(), tokenID, since, until, limit, beforeID, nil)
		} else {
			events, err = opts.Store.ListUsageEventsByToken(r.Context(), tokenID, limit, beforeID)
		}
		if err != nil {
			writeHTTPAPIJSON(w, false, "查询失败", nil)
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
			if errClass != nil && strings.TrimSpace(*errClass) == "upstream_unavailable" {
				m := "上游不可用"
				errMsg = &m
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
				ServiceTier:        normalizeUsageServiceTierPtr(e.ServiceTier),
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
				ModelMismatch:      usageEventModelMismatch(e.ForwardedModel, e.UpstreamResponseModel),
				CreatedAt:          e.CreatedAt,
				UpdatedAt:          e.UpdatedAt,
			})
		}
		if len(events) > 0 {
			next := events[len(events)-1].ID
			resp.NextBeforeID = &next
		}

		writeHTTPAPIJSON(w, true, "", resp)
	}
}

func v1UsageEventIDFromPath(path string) (int64, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 5 {
		return 0, false
	}
	if parts[0] != "v1" || parts[1] != "usage" || parts[2] != "events" || parts[4] != "detail" {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func v1UsageEventDetailHTTPHandler(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if opts.Store == nil {
			writeHTTPAPIJSON(w, false, "store 未初始化", nil)
			return
		}
		if rejectTokenQueryParamsForDataPlane(w, r) {
			return
		}

		p, ok := auth.PrincipalFromContext(r.Context())
		if !ok || p.TokenID == nil || *p.TokenID <= 0 {
			http.Error(w, "未提供 Token", http.StatusUnauthorized)
			return
		}
		tokenID := *p.TokenID

		id, ok := v1UsageEventIDFromPath(r.URL.Path)
		if !ok {
			writeHTTPAPIJSON(w, false, "event_id 不合法", nil)
			return
		}

		ev, err := opts.Store.GetUsageEvent(r.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				writeHTTPAPIJSON(w, false, "not found", nil)
				return
			}
			writeHTTPAPIJSON(w, false, "查询失败", nil)
			return
		}
		if ev.TokenID != tokenID {
			writeHTTPAPIJSON(w, false, "not found", nil)
			return
		}

		pricingBreakdown, err := buildUsageEventPricingBreakdown(r.Context(), opts.Store, ev)
		if err != nil {
			writeHTTPAPIJSON(w, false, "查询失败", nil)
			return
		}

		writeHTTPAPIJSON(w, true, "", usageEventDetailAPIResponse{
			EventID:          id,
			PricingBreakdown: &pricingBreakdown,
			ModelCheck:       buildUsageEventModelCheck(ev),
		})
	}
}

func v1UsageTimeSeriesHTTPHandler(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if opts.Store == nil {
			writeHTTPAPIJSON(w, false, "store 未初始化", nil)
			return
		}
		if rejectTokenQueryParamsForDataPlane(w, r) {
			return
		}

		p, ok := auth.PrincipalFromContext(r.Context())
		if !ok || p.TokenID == nil || *p.TokenID <= 0 {
			http.Error(w, "未提供 Token", http.StatusUnauthorized)
			return
		}
		tokenID := *p.TokenID

		loc, tzName, ok := usageRequestLocationFromRequest(r)
		if !ok {
			writeHTTPAPIJSON(w, false, "tz 不合法（需为 IANA 时区名，如 Asia/Shanghai）", nil)
			return
		}

		now := time.Now().UTC()
		startStr := ""
		endStr := ""
		granularity := "hour"
		if r != nil && r.URL != nil {
			q := r.URL.Query()
			startStr = strings.TrimSpace(q.Get("start"))
			endStr = strings.TrimSpace(q.Get("end"))
			if v := strings.TrimSpace(strings.ToLower(q.Get("granularity"))); v != "" {
				granularity = v
			}
		}
		since, until, sinceLocal, untilLocal, ok := parseDateRangeInLocation(now, startStr, endStr, loc)
		if !ok {
			writeHTTPAPIJSON(w, false, "start/end 不合法（格式：YYYY-MM-DD）", nil)
			return
		}
		startResp := sinceLocal.Format("2006-01-02")
		endResp := untilLocal.Add(-time.Second).Format("2006-01-02")

		if granularity == "" {
			granularity = "hour"
		}
		if granularity != "hour" && granularity != "day" {
			writeHTTPAPIJSON(w, false, "granularity 仅支持 hour/day", nil)
			return
		}

		rows, err := opts.Store.GetTokenUsageTimeSeriesRange(r.Context(), tokenID, since, until, granularity)
		if err != nil {
			writeHTTPAPIJSON(w, false, "查询时间序列失败", nil)
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

		writeHTTPAPIJSON(w, true, "", usageTimeSeriesAPIResponse{
			TimeZone:    tzName,
			Start:       startResp,
			End:         endResp,
			Granularity: granularity,
			Points:      points,
		})
	}
}
