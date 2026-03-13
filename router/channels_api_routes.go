package router

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/codexoauth"
	"realms/internal/modelcheck"
	"realms/internal/security"
	"realms/internal/store"
)

type channelView struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Groups    string `json:"groups"`
	Status    int    `json:"status"`
	Priority  int    `json:"priority"`
	Promotion bool   `json:"promotion"`
	BaseURL   string `json:"base_url,omitempty"`

	AllowServiceTier      bool `json:"allow_service_tier"`
	FastMode              bool `json:"fast_mode"`
	DisableStore          bool `json:"disable_store"`
	AllowSafetyIdentifier bool `json:"allow_safety_identifier"`

	Tag    *string `json:"tag,omitempty"`
	Weight int     `json:"weight"`

	KeyHint *string `json:"key_hint,omitempty"`

	LastTestAt        *time.Time `json:"last_test_at,omitempty"`
	LastTestLatencyMS int        `json:"last_test_latency_ms,omitempty"`
	LastTestOK        bool       `json:"last_test_ok"`
}

type channelDetailView struct {
	channelView

	OpenAIOrganization   *string                      `json:"openai_organization,omitempty"`
	TestModel            *string                      `json:"test_model,omitempty"`
	Remark               *string                      `json:"remark,omitempty"`
	AutoBan              bool                         `json:"auto_ban"`
	Setting              store.UpstreamChannelSetting `json:"setting,omitempty"`
	ParamOverride        string                       `json:"param_override,omitempty"`
	HeaderOverride       string                       `json:"header_override,omitempty"`
	StatusCodeMapping    string                       `json:"status_code_mapping,omitempty"`
	ModelSuffixPreserve  string                       `json:"model_suffix_preserve,omitempty"`
	RequestBodyBlacklist string                       `json:"request_body_blacklist,omitempty"`
	RequestBodyWhitelist string                       `json:"request_body_whitelist,omitempty"`
}

func setChannelAPIRoutes(r gin.IRoutes, opts Options) {
	admin := requireRoot(opts)

	r.GET("/channel", admin, listChannelsHandler(opts))
	r.GET("/channel/", admin, listChannelsHandler(opts))
	r.GET("/channel/page", admin, channelsPageHandler(opts))
	r.GET("/channel/page/", admin, channelsPageHandler(opts))
	r.GET("/channel/:channel_id/timeseries", admin, channelTimeSeriesHandler(opts))
	r.GET("/channel/:channel_id/timeseries/", admin, channelTimeSeriesHandler(opts))

	r.POST("/channel", admin, createChannelHandler(opts))
	r.POST("/channel/", admin, createChannelHandler(opts))

	r.PUT("/channel", admin, updateChannelHandler(opts))
	r.PUT("/channel/", admin, updateChannelHandler(opts))
	r.POST("/channel/reorder", admin, reorderChannelsHandler(opts))
	r.POST("/channel/reorder/", admin, reorderChannelsHandler(opts))

	r.GET("/channel/:channel_id", admin, getChannelHandler(opts))
	r.DELETE("/channel/:channel_id", admin, deleteChannelHandler(opts))
	r.DELETE("/channel/:channel_id/", admin, deleteChannelHandler(opts))

	r.POST("/channel/:channel_id/key", admin, getChannelKeyHandler(opts))
	r.POST("/channel/:channel_id/key/", admin, getChannelKeyHandler(opts))

	r.GET("/channel/:channel_id/credentials", admin, listChannelCredentialsHandler(opts))
	r.POST("/channel/:channel_id/credentials", admin, createChannelCredentialHandler(opts))
	r.DELETE("/channel/:channel_id/credentials/:credential_id", admin, deleteChannelCredentialHandler(opts))
	r.GET("/channel/:channel_id/codex-accounts", admin, listChannelCodexAccountsHandler(opts))
	r.POST("/channel/:channel_id/codex-oauth/start", admin, startChannelCodexOAuthHandler(opts))
	r.POST("/channel/:channel_id/codex-oauth/complete", admin, completeChannelCodexOAuthHandler(opts))
	r.POST("/channel/:channel_id/codex-accounts", admin, createChannelCodexAccountHandler(opts))
	r.POST("/channel/:channel_id/codex-accounts/refresh", admin, refreshChannelCodexAccountsHandler(opts))
	r.POST("/channel/:channel_id/codex-accounts/:account_id/refresh", admin, refreshChannelCodexAccountHandler(opts))
	r.DELETE("/channel/:channel_id/codex-accounts/:account_id", admin, deleteChannelCodexAccountHandler(opts))

	r.PUT("/channel/:channel_id/meta", admin, updateChannelMetaHandler(opts))
	r.PUT("/channel/:channel_id/setting", admin, updateChannelSettingHandler(opts))
	r.PUT("/channel/:channel_id/param_override", admin, updateChannelParamOverrideHandler(opts))
	r.PUT("/channel/:channel_id/header_override", admin, updateChannelHeaderOverrideHandler(opts))
	r.PUT("/channel/:channel_id/model_suffix_preserve", admin, updateChannelModelSuffixPreserveHandler(opts))
	r.PUT("/channel/:channel_id/request_body_whitelist", admin, updateChannelRequestBodyWhitelistHandler(opts))
	r.PUT("/channel/:channel_id/request_body_blacklist", admin, updateChannelRequestBodyBlacklistHandler(opts))
	r.PUT("/channel/:channel_id/status_code_mapping", admin, updateChannelStatusCodeMappingHandler(opts))

	r.GET("/channel/test/:channel_id", admin, testChannelHandler(opts))
}

func listChannelsHandler(opts Options) gin.HandlerFunc {
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
		out := make([]channelView, 0, len(channels))
		for _, ch := range channels {
			view := channelView{
				ID:        ch.ID,
				Type:      ch.Type,
				Name:      ch.Name,
				Groups:    ch.Groups,
				Status:    ch.Status,
				Priority:  ch.Priority,
				Promotion: ch.Promotion,

				AllowServiceTier:      ch.AllowServiceTier,
				FastMode:              ch.FastMode,
				DisableStore:          ch.DisableStore,
				AllowSafetyIdentifier: ch.AllowSafetyIdentifier,

				Tag:    ch.Tag,
				Weight: ch.Weight,

				LastTestAt:        ch.LastTestAt,
				LastTestLatencyMS: ch.LastTestLatencyMS,
				LastTestOK:        ch.LastTestOK,
			}
			ep, err := opts.Store.GetUpstreamEndpointByChannelID(c.Request.Context(), ch.ID)
			if err == nil && ep.ID > 0 {
				view.BaseURL = ep.BaseURL
				switch ch.Type {
				case store.UpstreamTypeOpenAICompatible:
					if creds, err := opts.Store.ListOpenAICompatibleCredentialsByEndpoint(c.Request.Context(), ep.ID); err == nil && len(creds) > 0 {
						view.KeyHint = creds[0].APIKeyHint
					}
				case store.UpstreamTypeAnthropic:
					if creds, err := opts.Store.ListAnthropicCredentialsByEndpoint(c.Request.Context(), ep.ID); err == nil && len(creds) > 0 {
						view.KeyHint = creds[0].APIKeyHint
					}
				}
			}
			out = append(out, view)
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

type channelUsageView struct {
	CommittedUSD          string `json:"committed_usd"`
	Tokens                int64  `json:"tokens"`
	CacheRatio            string `json:"cache_ratio"`
	AvgFirstTokenLatency  string `json:"avg_first_token_latency"`
	OutputTokensPerSecond string `json:"tokens_per_second"`
}

type channelUsageOverviewView struct {
	Requests             int64  `json:"requests"`
	Tokens               int64  `json:"tokens"`
	CommittedUSD         string `json:"committed_usd"`
	CacheRatio           string `json:"cache_ratio"`
	AvgFirstTokenLatency string `json:"avg_first_token_latency"`
	TokensPerSecond      string `json:"tokens_per_second"`
}

type channelAdminListItem struct {
	channelView
	InUse   bool               `json:"in_use"`
	Usage   channelUsageView   `json:"usage"`
	Runtime channelRuntimeInfo `json:"runtime"`
}

type channelsPageResponse struct {
	AdminTimeZone string                   `json:"admin_time_zone"`
	Start         string                   `json:"start"`
	End           string                   `json:"end"`
	Overview      channelUsageOverviewView `json:"overview"`
	Channels      []channelAdminListItem   `json:"channels"`
}

type channelTimeSeriesPointView struct {
	Bucket               string  `json:"bucket"`
	CommittedUSD         float64 `json:"committed_usd"`
	Tokens               int64   `json:"tokens"`
	CacheRatio           float64 `json:"cache_ratio"`
	AvgFirstTokenLatency float64 `json:"avg_first_token_latency"`
	TokensPerSecond      float64 `json:"tokens_per_second"`
}

type channelTimeSeriesResponse struct {
	AdminTimeZone string                       `json:"admin_time_zone"`
	ChannelID     int64                        `json:"channel_id"`
	Start         string                       `json:"start"`
	End           string                       `json:"end"`
	Granularity   string                       `json:"granularity"`
	Points        []channelTimeSeriesPointView `json:"points"`
}

func formatAvgFirstTokenLatency(ms float64, samples int64) string {
	if samples <= 0 || ms <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f ms", ms)
}

func formatTokensPerSecond(v float64) string {
	if v <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", v)
}

func channelsPageHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
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
		allTime := queryBool(q.Get("all_time"))
		if allTime {
			s, e, has, err := resolveAllTimeGlobalStartEnd(c.Request.Context(), opts, loc, todayStr)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
				return
			}
			if has {
				startStr = s
				endStr = e
			}
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

		since := sinceLocal.UTC()
		until := untilLocal.UTC()

		rawUsage, err := opts.Store.GetUsageStatsByChannelRange(c.Request.Context(), since, until)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "渠道用量统计失败"})
			return
		}
		overviewStats, err := opts.Store.GetGlobalUsageStatsRange(c.Request.Context(), since, until)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "渠道总览统计失败"})
			return
		}
		usageByChannelID := make(map[int64]store.ChannelUsageStats, len(rawUsage))
		for _, row := range rawUsage {
			usageByChannelID[row.ChannelID] = row
		}

		channels, err := opts.Store.ListUpstreamChannels(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询渠道失败"})
			return
		}

		usedIDs, err := opts.Store.ListUsedUpstreamChannelIDs(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询使用中渠道失败"})
			return
		}
		usedSet := make(map[int64]struct{}, len(usedIDs))
		for _, id := range usedIDs {
			usedSet[id] = struct{}{}
		}

		sinceRecent := time.Now().UTC().Add(-1 * time.Minute)
		recentIDs, err := opts.Store.ListUpstreamChannelIDsWithRequestsSince(c.Request.Context(), sinceRecent)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询最近请求渠道失败"})
			return
		}
		recentSet := make(map[int64]struct{}, len(recentIDs))
		for _, id := range recentIDs {
			recentSet[id] = struct{}{}
		}

		out := make([]channelAdminListItem, 0, len(channels))
		for _, ch := range channels {
			view := channelView{
				ID:        ch.ID,
				Type:      ch.Type,
				Name:      ch.Name,
				Groups:    ch.Groups,
				Status:    ch.Status,
				Priority:  ch.Priority,
				Promotion: ch.Promotion,

				AllowServiceTier:      ch.AllowServiceTier,
				FastMode:              ch.FastMode,
				DisableStore:          ch.DisableStore,
				AllowSafetyIdentifier: ch.AllowSafetyIdentifier,

				Tag:    ch.Tag,
				Weight: ch.Weight,

				LastTestAt:        ch.LastTestAt,
				LastTestLatencyMS: ch.LastTestLatencyMS,
				LastTestOK:        ch.LastTestOK,
			}
			ep, err := opts.Store.GetUpstreamEndpointByChannelID(c.Request.Context(), ch.ID)
			if err == nil && ep.ID > 0 {
				view.BaseURL = ep.BaseURL
				switch ch.Type {
				case store.UpstreamTypeOpenAICompatible:
					if creds, err := opts.Store.ListOpenAICompatibleCredentialsByEndpoint(c.Request.Context(), ep.ID); err == nil && len(creds) > 0 {
						view.KeyHint = creds[0].APIKeyHint
					}
				case store.UpstreamTypeAnthropic:
					if creds, err := opts.Store.ListAnthropicCredentialsByEndpoint(c.Request.Context(), ep.ID); err == nil && len(creds) > 0 {
						view.KeyHint = creds[0].APIKeyHint
					}
				}
			}

			us := usageByChannelID[ch.ID]
			usageView := channelUsageView{
				CommittedUSD:          formatUSDPlain(us.CommittedUSD),
				Tokens:                us.Tokens,
				CacheRatio:            fmt.Sprintf("%.1f%%", us.CacheRatio*100),
				AvgFirstTokenLatency:  formatAvgFirstTokenLatency(us.AvgFirstTokenMS, us.FirstTokenSamples),
				OutputTokensPerSecond: formatTokensPerSecond(us.OutputTokensPerSec),
			}

			runtime := channelRuntimeForAPI(c.Request.Context(), opts, ch.ID, loc)

			inUse := false
			if _, ok := usedSet[ch.ID]; ok {
				if _, ok2 := recentSet[ch.ID]; ok2 {
					inUse = true
				}
			}

			out = append(out, channelAdminListItem{
				channelView: view,
				InUse:       inUse,
				Usage:       usageView,
				Runtime:     runtime,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": channelsPageResponse{
				AdminTimeZone: tzName,
				Start:         startStr,
				End:           endStr,
				Overview: channelUsageOverviewView{
					Requests:             overviewStats.Requests,
					Tokens:               overviewStats.Tokens,
					CommittedUSD:         formatUSDPlain(overviewStats.CostUSD),
					CacheRatio:           fmt.Sprintf("%.1f%%", overviewStats.CacheRatio*100),
					AvgFirstTokenLatency: formatAvgFirstTokenLatency(overviewStats.AvgFirstTokenMS, overviewStats.FirstTokenSamples),
					TokensPerSecond:      formatTokensPerSecond(overviewStats.OutputTokensPerSec),
				},
				Channels: out,
			},
		})
	}
}

func channelTimeSeriesHandler(opts Options) gin.HandlerFunc {
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

		loc, tzName := adminTimeLocation(c.Request.Context(), opts)

		nowUTC := time.Now().UTC()
		nowLocal := nowUTC.In(loc)
		todayStartLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
		todayStr := todayStartLocal.Format("2006-01-02")

		q := c.Request.URL.Query()
		startStr := strings.TrimSpace(q.Get("start"))
		endStr := strings.TrimSpace(q.Get("end"))
		allTime := queryBool(q.Get("all_time"))
		granularity := strings.TrimSpace(strings.ToLower(q.Get("granularity")))
		if granularity == "" {
			granularity = "hour"
		}
		if granularity != "hour" && granularity != "day" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "granularity 仅支持 hour/day"})
			return
		}
		if allTime {
			s, e, has, err := resolveAllTimeChannelStartEnd(c.Request.Context(), opts, loc, todayStr, channelID)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
				return
			}
			if has {
				startStr = s
				endStr = e
			}
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

		rows, err := opts.Store.GetChannelUsageTimeSeriesRange(c.Request.Context(), channelID, sinceLocal.UTC(), untilLocal.UTC(), granularity)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询渠道时间序列失败"})
			return
		}

		points := make([]channelTimeSeriesPointView, 0, len(rows))
		for _, row := range rows {
			points = append(points, channelTimeSeriesPointView{
				Bucket:               row.Time.In(loc).Format("2006-01-02 15:04"),
				CommittedUSD:         row.CommittedUSD.InexactFloat64(),
				Tokens:               row.Tokens,
				CacheRatio:           row.CacheRatio * 100,
				AvgFirstTokenLatency: row.AvgFirstTokenMS,
				TokensPerSecond:      row.OutputTokensPerSec,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": channelTimeSeriesResponse{
				AdminTimeZone: tzName,
				ChannelID:     channelID,
				Start:         startStr,
				End:           endStr,
				Granularity:   granularity,
				Points:        points,
			},
		})
	}
}

func getChannelHandler(opts Options) gin.HandlerFunc {
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
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}
		view := channelView{
			ID:        ch.ID,
			Type:      ch.Type,
			Name:      ch.Name,
			Groups:    ch.Groups,
			Status:    ch.Status,
			Priority:  ch.Priority,
			Promotion: ch.Promotion,

			AllowServiceTier:      ch.AllowServiceTier,
			FastMode:              ch.FastMode,
			DisableStore:          ch.DisableStore,
			AllowSafetyIdentifier: ch.AllowSafetyIdentifier,

			Tag:    ch.Tag,
			Weight: ch.Weight,

			LastTestAt:        ch.LastTestAt,
			LastTestLatencyMS: ch.LastTestLatencyMS,
			LastTestOK:        ch.LastTestOK,
		}
		ep, err := opts.Store.GetUpstreamEndpointByChannelID(c.Request.Context(), ch.ID)
		if err == nil && ep.ID > 0 {
			view.BaseURL = ep.BaseURL
			switch ch.Type {
			case store.UpstreamTypeOpenAICompatible:
				if creds, err := opts.Store.ListOpenAICompatibleCredentialsByEndpoint(c.Request.Context(), ep.ID); err == nil && len(creds) > 0 {
					view.KeyHint = creds[0].APIKeyHint
				}
			case store.UpstreamTypeAnthropic:
				if creds, err := opts.Store.ListAnthropicCredentialsByEndpoint(c.Request.Context(), ep.ID); err == nil && len(creds) > 0 {
					view.KeyHint = creds[0].APIKeyHint
				}
			}
		}

		detail := channelDetailView{
			channelView:          view,
			OpenAIOrganization:   ch.OpenAIOrganization,
			TestModel:            ch.TestModel,
			Remark:               ch.Remark,
			AutoBan:              ch.AutoBan,
			Setting:              ch.Setting,
			ParamOverride:        ch.ParamOverride,
			HeaderOverride:       ch.HeaderOverride,
			StatusCodeMapping:    ch.StatusCodeMapping,
			ModelSuffixPreserve:  ch.ModelSuffixPreserve,
			RequestBodyBlacklist: ch.RequestBodyBlacklist,
			RequestBodyWhitelist: ch.RequestBodyWhitelist,
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": detail})
	}
}

type createChannelRequest struct {
	Type                  string  `json:"type"`
	Name                  string  `json:"name"`
	Groups                string  `json:"groups"`
	BaseURL               string  `json:"base_url"`
	Key                   *string `json:"key,omitempty"`
	Priority              int     `json:"priority"`
	Promotion             bool    `json:"promotion"`
	AllowServiceTier      *bool   `json:"allow_service_tier,omitempty"`
	FastMode              *bool   `json:"fast_mode,omitempty"`
	DisableStore          bool    `json:"disable_store"`
	AllowSafetyIdentifier bool    `json:"allow_safety_identifier"`
}

func createChannelHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		var req createChannelRequest
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		req.Type = strings.TrimSpace(req.Type)
		req.Name = strings.TrimSpace(req.Name)
		req.Groups = strings.TrimSpace(req.Groups)
		req.BaseURL = strings.TrimSpace(req.BaseURL)
		if req.Key != nil {
			k := strings.TrimSpace(*req.Key)
			if k == "" {
				req.Key = nil
			} else {
				req.Key = &k
			}
		}

		if req.Type == "" || req.Name == "" || req.BaseURL == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		switch req.Type {
		case store.UpstreamTypeOpenAICompatible, store.UpstreamTypeAnthropic, store.UpstreamTypeCodexOAuth:
		default:
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "不支持的渠道类型"})
			return
		}
		if req.Type == store.UpstreamTypeCodexOAuth && req.Key != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "codex_oauth Channel 不支持 key"})
			return
		}
		if _, err := security.ValidateBaseURL(req.BaseURL); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "base_url 不合法"})
			return
		}

		fastMode := true
		if req.FastMode != nil {
			fastMode = *req.FastMode
		}
		allowServiceTier := true
		if req.AllowServiceTier != nil {
			allowServiceTier = *req.AllowServiceTier
		}

		id, err := opts.Store.CreateUpstreamChannelWithRequestPolicy(c.Request.Context(), req.Type, req.Name, req.Groups, req.Priority, req.Promotion, allowServiceTier, req.DisableStore, req.AllowSafetyIdentifier, fastMode)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		ep, err := opts.Store.SetUpstreamEndpointBaseURL(c.Request.Context(), id, req.BaseURL)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建 Endpoint 失败"})
			return
		}
		if req.Key != nil {
			switch req.Type {
			case store.UpstreamTypeOpenAICompatible:
				if _, _, err := opts.Store.CreateOpenAICompatibleCredential(c.Request.Context(), ep.ID, nil, *req.Key); err != nil {
					_ = opts.Store.DeleteUpstreamChannel(c.Request.Context(), id)
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建 Credential 失败"})
					return
				}
			case store.UpstreamTypeAnthropic:
				if _, _, err := opts.Store.CreateAnthropicCredential(c.Request.Context(), ep.ID, nil, *req.Key); err != nil {
					_ = opts.Store.DeleteUpstreamChannel(c.Request.Context(), id)
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建 Credential 失败"})
					return
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"id": id}})
	}
}

type updateChannelRequest struct {
	ID                    int64   `json:"id"`
	Name                  *string `json:"name,omitempty"`
	Groups                *string `json:"groups,omitempty"`
	BaseURL               *string `json:"base_url,omitempty"`
	Key                   *string `json:"key,omitempty"`
	Status                *int    `json:"status,omitempty"`
	Priority              *int    `json:"priority,omitempty"`
	Promotion             *bool   `json:"promotion,omitempty"`
	AllowServiceTier      *bool   `json:"allow_service_tier,omitempty"`
	FastMode              *bool   `json:"fast_mode,omitempty"`
	DisableStore          *bool   `json:"disable_store,omitempty"`
	AllowSafetyIdentifier *bool   `json:"allow_safety_identifier,omitempty"`
}

func reorderChannelsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		var ids []int64
		if err := c.ShouldBindJSON(&ids); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if len(ids) == 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "ids 不能为空"})
			return
		}
		base := len(ids) * 10
		patches := make([]store.UpstreamChannelPriorityPatch, 0, len(ids))
		for idx, id := range ids {
			if id <= 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 不合法"})
				return
			}
			patches = append(patches, store.UpstreamChannelPriorityPatch{ID: id, Priority: base - idx*10})
		}
		if err := opts.Store.UpdateUpstreamChannelPriorities(c.Request.Context(), patches); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func updateChannelHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		var req updateChannelRequest
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if req.ID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 不合法"})
			return
		}

		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), req.ID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}
		if ch.Type == store.UpstreamTypeCodexOAuth && req.Key != nil && strings.TrimSpace(*req.Key) != "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "codex_oauth Channel 不支持 key"})
			return
		}

		name := ch.Name
		if req.Name != nil {
			name = strings.TrimSpace(*req.Name)
			if name == "" {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "name 不能为空"})
				return
			}
		}
		status := ch.Status
		if req.Status != nil {
			status = *req.Status
		}
		priority := ch.Priority
		if req.Priority != nil {
			priority = *req.Priority
		}
		promotion := ch.Promotion
		if req.Promotion != nil {
			promotion = *req.Promotion
		}
		allowServiceTier := ch.AllowServiceTier
		if req.AllowServiceTier != nil {
			allowServiceTier = *req.AllowServiceTier
		}
		fastMode := ch.FastMode
		if req.FastMode != nil {
			fastMode = *req.FastMode
		}
		disableStore := ch.DisableStore
		if req.DisableStore != nil {
			disableStore = *req.DisableStore
		}
		allowSafetyIdentifier := ch.AllowSafetyIdentifier
		if req.AllowSafetyIdentifier != nil {
			allowSafetyIdentifier = *req.AllowSafetyIdentifier
		}

		if err := opts.Store.UpdateUpstreamChannelBasics(c.Request.Context(), ch.ID, name, status, priority, promotion, allowServiceTier, disableStore, allowSafetyIdentifier, fastMode); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}

		if req.Groups != nil {
			if err := opts.Store.SetUpstreamChannelGroups(c.Request.Context(), ch.ID, strings.TrimSpace(*req.Groups)); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新渠道组失败"})
				return
			}
		}

		var ep store.UpstreamEndpoint
		if req.BaseURL != nil {
			baseURL := strings.TrimSpace(*req.BaseURL)
			if baseURL == "" {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "base_url 不能为空"})
				return
			}
			if _, err := security.ValidateBaseURL(baseURL); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "base_url 不合法"})
				return
			}
			ep, err = opts.Store.SetUpstreamEndpointBaseURL(c.Request.Context(), ch.ID, baseURL)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新 Endpoint 失败"})
				return
			}
		} else {
			ep, _ = opts.Store.GetUpstreamEndpointByChannelID(c.Request.Context(), ch.ID)
		}

		if req.Key != nil && ep.ID > 0 {
			key := strings.TrimSpace(*req.Key)
			if key != "" {
				switch ch.Type {
				case store.UpstreamTypeOpenAICompatible:
					if _, _, err := opts.Store.CreateOpenAICompatibleCredential(c.Request.Context(), ep.ID, nil, key); err != nil {
						c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新 Credential 失败"})
						return
					}
				case store.UpstreamTypeAnthropic:
					if _, _, err := opts.Store.CreateAnthropicCredential(c.Request.Context(), ep.ID, nil, key); err != nil {
						c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新 Credential 失败"})
						return
					}
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}

type channelCredentialView struct {
	ID         int64   `json:"id"`
	Name       *string `json:"name,omitempty"`
	APIKeyHint *string `json:"api_key_hint,omitempty"`
	MaskedKey  string  `json:"masked_key"`
	Status     int     `json:"status"`
}

func maskAPIKeyHint(hint *string) string {
	if hint == nil || strings.TrimSpace(*hint) == "" {
		return "-"
	}
	s := strings.TrimSpace(*hint)
	if len(s) > 4 {
		return "..." + s[len(s)-4:]
	}
	return s
}

func trimOptionalString(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}

type channelCodexAccountView struct {
	ID        int64   `json:"id"`
	AccountID string  `json:"account_id"`
	Email     *string `json:"email,omitempty"`
	Status    int     `json:"status"`

	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	LastRefreshAt *time.Time `json:"last_refresh_at,omitempty"`
	CooldownUntil *time.Time `json:"cooldown_until,omitempty"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`

	BalanceTotalGrantedUSD   *string    `json:"balance_total_granted_usd,omitempty"`
	BalanceTotalUsedUSD      *string    `json:"balance_total_used_usd,omitempty"`
	BalanceTotalAvailableUSD *string    `json:"balance_total_available_usd,omitempty"`
	BalanceUpdatedAt         *time.Time `json:"balance_updated_at,omitempty"`
	BalanceError             *string    `json:"balance_error,omitempty"`

	QuotaCreditsHasCredits    *bool      `json:"quota_credits_has_credits,omitempty"`
	QuotaCreditsUnlimited     *bool      `json:"quota_credits_unlimited,omitempty"`
	QuotaCreditsBalance       *string    `json:"quota_credits_balance,omitempty"`
	QuotaPrimaryUsedPercent   *int       `json:"quota_primary_used_percent,omitempty"`
	QuotaPrimaryResetAt       *time.Time `json:"quota_primary_reset_at,omitempty"`
	QuotaSecondaryUsedPercent *int       `json:"quota_secondary_used_percent,omitempty"`
	QuotaSecondaryResetAt     *time.Time `json:"quota_secondary_reset_at,omitempty"`
	QuotaUpdatedAt            *time.Time `json:"quota_updated_at,omitempty"`
	QuotaError                *string    `json:"quota_error,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func decimalStringPtr(v *decimal.Decimal) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(v.String())
	if s == "" {
		return nil
	}
	return &s
}

func codexAccountView(a store.CodexOAuthAccount) channelCodexAccountView {
	return channelCodexAccountView{
		ID:        a.ID,
		AccountID: a.AccountID,
		Email:     a.Email,
		Status:    a.Status,

		ExpiresAt:     a.ExpiresAt,
		LastRefreshAt: a.LastRefreshAt,
		CooldownUntil: a.CooldownUntil,
		LastUsedAt:    a.LastUsedAt,

		BalanceTotalGrantedUSD:   decimalStringPtr(a.BalanceTotalGrantedUSD),
		BalanceTotalUsedUSD:      decimalStringPtr(a.BalanceTotalUsedUSD),
		BalanceTotalAvailableUSD: decimalStringPtr(a.BalanceTotalAvailableUSD),
		BalanceUpdatedAt:         a.BalanceUpdatedAt,
		BalanceError:             a.BalanceError,

		QuotaCreditsHasCredits:    a.QuotaCreditsHasCredits,
		QuotaCreditsUnlimited:     a.QuotaCreditsUnlimited,
		QuotaCreditsBalance:       a.QuotaCreditsBalance,
		QuotaPrimaryUsedPercent:   a.QuotaPrimaryUsedPercent,
		QuotaPrimaryResetAt:       a.QuotaPrimaryResetAt,
		QuotaSecondaryUsedPercent: a.QuotaSecondaryUsedPercent,
		QuotaSecondaryResetAt:     a.QuotaSecondaryResetAt,
		QuotaUpdatedAt:            a.QuotaUpdatedAt,
		QuotaError:                a.QuotaError,

		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}

func loadCodexChannelEndpoint(c *gin.Context, opts Options, channelID int64) (store.UpstreamChannel, store.UpstreamEndpoint, bool) {
	ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
			return store.UpstreamChannel{}, store.UpstreamEndpoint{}, false
		}
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
		return store.UpstreamChannel{}, store.UpstreamEndpoint{}, false
	}
	if ch.Type != store.UpstreamTypeCodexOAuth {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "仅 codex_oauth 渠道支持账号管理"})
		return store.UpstreamChannel{}, store.UpstreamEndpoint{}, false
	}
	ep, err := opts.Store.GetUpstreamEndpointByChannelID(c.Request.Context(), ch.ID)
	if err != nil || ep.ID <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "endpoint 不存在"})
		return store.UpstreamChannel{}, store.UpstreamEndpoint{}, false
	}
	return ch, ep, true
}

func listChannelCodexAccountsHandler(opts Options) gin.HandlerFunc {
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
		_, ep, ok := loadCodexChannelEndpoint(c, opts, channelID)
		if !ok {
			return
		}
		accounts, err := opts.Store.ListCodexOAuthAccountsByEndpoint(c.Request.Context(), ep.ID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询账号失败"})
			return
		}
		out := make([]channelCodexAccountView, 0, len(accounts))
		for _, acc := range accounts {
			out = append(out, codexAccountView(acc))
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func startChannelCodexOAuthHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if opts.StartCodexOAuth == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "Codex OAuth 未启用"})
			return
		}
		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}
		_, ep, ok := loadCodexChannelEndpoint(c, opts, channelID)
		if !ok {
			return
		}
		actorUserID, ok := adminActorIDFromContext(c)
		if !ok || actorUserID < 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		authURL, err := opts.StartCodexOAuth(c.Request.Context(), ep.ID, actorUserID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": codexoauth.UserMessage(err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    gin.H{"auth_url": authURL},
		})
	}
}

func completeChannelCodexOAuthHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		CallbackURL string `json:"callback_url"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if opts.CompleteCodexOAuth == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "Codex OAuth 未启用"})
			return
		}
		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}
		_, ep, ok := loadCodexChannelEndpoint(c, opts, channelID)
		if !ok {
			return
		}
		actorUserID, ok := adminActorIDFromContext(c)
		if !ok || actorUserID < 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		parsed, err := codexoauth.ParseOAuthCallback(req.CallbackURL)
		if err != nil || parsed == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "回调 URL 解析失败，请粘贴包含 code/state 的完整 URL"})
			return
		}
		if strings.TrimSpace(parsed.Error) != "" {
			msg := "OAuth 回调失败：" + strings.TrimSpace(parsed.Error)
			if desc := strings.TrimSpace(parsed.ErrorDescription); desc != "" {
				msg += " - " + desc
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
		if strings.TrimSpace(parsed.Code) == "" || strings.TrimSpace(parsed.State) == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "回调 URL 缺少 code/state"})
			return
		}
		if err := opts.CompleteCodexOAuth(c.Request.Context(), ep.ID, actorUserID, parsed.State, parsed.Code); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": codexoauth.UserMessage(err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已完成授权"})
	}
}

func createChannelCodexAccountHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		AccountID    *string `json:"account_id,omitempty"`
		Email        *string `json:"email,omitempty"`
		AccessToken  string  `json:"access_token"`
		RefreshToken string  `json:"refresh_token"`
		IDToken      *string `json:"id_token,omitempty"`
		ExpiresAt    *string `json:"expires_at,omitempty"`
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
		_, ep, ok := loadCodexChannelEndpoint(c, opts, channelID)
		if !ok {
			return
		}
		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		accountID := ""
		if req.AccountID != nil {
			accountID = strings.TrimSpace(*req.AccountID)
		}
		email := trimOptionalString(req.Email)
		accessToken := strings.TrimSpace(req.AccessToken)
		refreshToken := strings.TrimSpace(req.RefreshToken)
		idToken := trimOptionalString(req.IDToken)

		if accessToken == "" || refreshToken == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "access_token 和 refresh_token 不能为空"})
			return
		}

		if idToken != nil {
			if claims, err := codexoauth.ParseIDTokenClaims(*idToken); err == nil && claims != nil {
				if accountID == "" {
					accountID = strings.TrimSpace(claims.AccountID)
				}
				if email == nil {
					e := strings.TrimSpace(claims.Email)
					if e != "" {
						email = &e
					}
				}
			}
		}

		if accountID == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "account_id 不能为空（可传 account_id，或提供包含 account_id 的 id_token）"})
			return
		}

		var expiresAt *time.Time
		if req.ExpiresAt != nil {
			s := strings.TrimSpace(*req.ExpiresAt)
			if s != "" {
				t, err := time.Parse(time.RFC3339, s)
				if err != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "expires_at 不合法（需 RFC3339）"})
					return
				}
				utc := t.UTC()
				expiresAt = &utc
			}
		}

		id, err := opts.Store.CreateCodexOAuthAccount(c.Request.Context(), ep.ID, accountID, email, accessToken, refreshToken, idToken, expiresAt)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建账号失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已添加账号", "data": gin.H{"id": id}})
	}
}

func refreshChannelCodexAccountsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if opts.RefreshCodexQuotasByEndpointID == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "Codex 刷新能力未启用"})
			return
		}
		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}
		_, ep, ok := loadCodexChannelEndpoint(c, opts, channelID)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
		defer cancel()
		if err := opts.RefreshCodexQuotasByEndpointID(ctx, ep.ID); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "刷新失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已刷新"})
	}
}

func refreshChannelCodexAccountHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if opts.RefreshCodexQuotaByAccountID == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "Codex 刷新能力未启用"})
			return
		}
		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}
		accountID, err := strconv.ParseInt(strings.TrimSpace(c.Param("account_id")), 10, 64)
		if err != nil || accountID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "account_id 不合法"})
			return
		}
		_, ep, ok := loadCodexChannelEndpoint(c, opts, channelID)
		if !ok {
			return
		}
		acc, err := opts.Store.GetCodexOAuthAccountByID(c.Request.Context(), accountID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "account 不存在"})
			return
		}
		if acc.EndpointID != ep.ID {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "account 不属于该渠道"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		if err := opts.RefreshCodexQuotaByAccountID(ctx, acc.ID); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "刷新失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已刷新"})
	}
}

func deleteChannelCodexAccountHandler(opts Options) gin.HandlerFunc {
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
		accountID, err := strconv.ParseInt(strings.TrimSpace(c.Param("account_id")), 10, 64)
		if err != nil || accountID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "account_id 不合法"})
			return
		}
		_, ep, ok := loadCodexChannelEndpoint(c, opts, channelID)
		if !ok {
			return
		}
		acc, err := opts.Store.GetCodexOAuthAccountByID(c.Request.Context(), accountID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "account 不存在"})
			return
		}
		if acc.EndpointID != ep.ID {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "account 不属于该渠道"})
			return
		}
		if err := opts.Store.DeleteCodexOAuthAccount(c.Request.Context(), acc.ID); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
	}
}

func listChannelCredentialsHandler(opts Options) gin.HandlerFunc {
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
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}
		if ch.Type == store.UpstreamTypeCodexOAuth {
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": []channelCredentialView{}})
			return
		}
		ep, err := opts.Store.GetUpstreamEndpointByChannelID(c.Request.Context(), ch.ID)
		if err != nil || ep.ID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "endpoint 不存在"})
			return
		}

		var out []channelCredentialView
		switch ch.Type {
		case store.UpstreamTypeOpenAICompatible:
			creds, err := opts.Store.ListOpenAICompatibleCredentialsByEndpoint(c.Request.Context(), ep.ID)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
				return
			}
			out = make([]channelCredentialView, 0, len(creds))
			for _, cred := range creds {
				out = append(out, channelCredentialView{
					ID:         cred.ID,
					Name:       cred.Name,
					APIKeyHint: cred.APIKeyHint,
					MaskedKey:  maskAPIKeyHint(cred.APIKeyHint),
					Status:     cred.Status,
				})
			}
		case store.UpstreamTypeAnthropic:
			creds, err := opts.Store.ListAnthropicCredentialsByEndpoint(c.Request.Context(), ep.ID)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
				return
			}
			out = make([]channelCredentialView, 0, len(creds))
			for _, cred := range creds {
				out = append(out, channelCredentialView{
					ID:         cred.ID,
					Name:       cred.Name,
					APIKeyHint: cred.APIKeyHint,
					MaskedKey:  maskAPIKeyHint(cred.APIKeyHint),
					Status:     cred.Status,
				})
			}
		default:
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "不支持的渠道类型"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func createChannelCredentialHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Name   *string `json:"name,omitempty"`
		APIKey string  `json:"api_key"`
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
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}
		if ch.Type == store.UpstreamTypeCodexOAuth {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "codex_oauth Channel 不支持 key"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		apiKey := strings.TrimSpace(req.APIKey)
		if apiKey == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "api_key 不能为空"})
			return
		}
		name := trimOptionalString(req.Name)

		ep, err := opts.Store.GetUpstreamEndpointByChannelID(c.Request.Context(), ch.ID)
		if err != nil || ep.ID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "endpoint 不存在"})
			return
		}

		switch ch.Type {
		case store.UpstreamTypeOpenAICompatible:
			id, hint, err := opts.Store.CreateOpenAICompatibleCredential(c.Request.Context(), ep.ID, name, apiKey)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "已添加", "data": gin.H{"id": id, "api_key_hint": hint}})
		case store.UpstreamTypeAnthropic:
			id, hint, err := opts.Store.CreateAnthropicCredential(c.Request.Context(), ep.ID, name, apiKey)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "已添加", "data": gin.H{"id": id, "api_key_hint": hint}})
		default:
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "不支持的渠道类型"})
			return
		}
	}
}

func deleteChannelCredentialHandler(opts Options) gin.HandlerFunc {
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
		credentialID, err := strconv.ParseInt(strings.TrimSpace(c.Param("credential_id")), 10, 64)
		if err != nil || credentialID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "credential_id 不合法"})
			return
		}
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}
		if ch.Type == store.UpstreamTypeCodexOAuth {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "codex_oauth Channel 不支持 key"})
			return
		}
		ep, err := opts.Store.GetUpstreamEndpointByChannelID(c.Request.Context(), ch.ID)
		if err != nil || ep.ID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "endpoint 不存在"})
			return
		}

		switch ch.Type {
		case store.UpstreamTypeOpenAICompatible:
			cred, err := opts.Store.GetOpenAICompatibleCredentialByID(c.Request.Context(), credentialID)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "credential 不存在"})
				return
			}
			if cred.EndpointID != ep.ID {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "credential 不属于该渠道"})
				return
			}
			if err := opts.Store.DeleteOpenAICompatibleCredential(c.Request.Context(), credentialID); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
				return
			}
		case store.UpstreamTypeAnthropic:
			cred, err := opts.Store.GetAnthropicCredentialByID(c.Request.Context(), credentialID)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "credential 不存在"})
				return
			}
			if cred.EndpointID != ep.ID {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "credential 不属于该渠道"})
				return
			}
			if err := opts.Store.DeleteAnthropicCredential(c.Request.Context(), credentialID); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
				return
			}
		default:
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "不支持的渠道类型"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
	}
}

func updateChannelMetaHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		OpenAIOrganization *string `json:"openai_organization,omitempty"`
		TestModel          *string `json:"test_model,omitempty"`
		Tag                *string `json:"tag,omitempty"`
		Remark             *string `json:"remark,omitempty"`
		Weight             *int    `json:"weight,omitempty"`
		AutoBan            *bool   `json:"auto_ban,omitempty"`
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
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}

		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		openAIOrg := ch.OpenAIOrganization
		if req.OpenAIOrganization != nil {
			openAIOrg = req.OpenAIOrganization
		}
		testModel := ch.TestModel
		if req.TestModel != nil {
			testModel = req.TestModel
		}
		tag := ch.Tag
		if req.Tag != nil {
			tag = req.Tag
		}
		remark := ch.Remark
		if req.Remark != nil {
			remark = req.Remark
		}
		weight := ch.Weight
		if req.Weight != nil {
			weight = *req.Weight
		}
		autoBan := ch.AutoBan
		if req.AutoBan != nil {
			autoBan = *req.AutoBan
		}

		if err := opts.Store.UpdateUpstreamChannelNewAPIMeta(c.Request.Context(), ch.ID, openAIOrg, testModel, tag, remark, weight, autoBan); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func updateChannelSettingHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		ForceFormat            *bool   `json:"force_format,omitempty"`
		ThinkingToContent      *bool   `json:"thinking_to_content,omitempty"`
		Proxy                  *string `json:"proxy,omitempty"`
		PassThroughBodyEnabled *bool   `json:"pass_through_body_enabled,omitempty"`
		SystemPrompt           *string `json:"system_prompt,omitempty"`
		SystemPromptOverride   *bool   `json:"system_prompt_override,omitempty"`
		CacheTTLPreference     *string `json:"cache_ttl_preference,omitempty"`
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
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}

		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		next := ch.Setting
		if req.ForceFormat != nil {
			next.ForceFormat = *req.ForceFormat
		}
		if req.ThinkingToContent != nil {
			next.ThinkingToContent = *req.ThinkingToContent
		}
		if req.Proxy != nil {
			next.Proxy = *req.Proxy
		}
		if req.PassThroughBodyEnabled != nil {
			next.PassThroughBodyEnabled = *req.PassThroughBodyEnabled
		}
		if req.SystemPrompt != nil {
			next.SystemPrompt = *req.SystemPrompt
		}
		if req.SystemPromptOverride != nil {
			next.SystemPromptOverride = *req.SystemPromptOverride
		}
		if req.CacheTTLPreference != nil {
			pref := strings.ToLower(strings.TrimSpace(*req.CacheTTLPreference))
			switch pref {
			case "", "inherit", "5m", "1h":
				next.CacheTTLPreference = pref
			default:
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "cache_ttl_preference 仅支持 inherit/5m/1h"})
				return
			}
		}

		if err := opts.Store.UpdateUpstreamChannelNewAPISetting(c.Request.Context(), ch.ID, next); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func updateChannelParamOverrideHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		ParamOverride string `json:"param_override"`
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
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if err := opts.Store.UpdateUpstreamChannelParamOverride(c.Request.Context(), ch.ID, req.ParamOverride); err != nil {
			msg := "保存失败"
			if strings.Contains(err.Error(), "param_override 不是有效 JSON") {
				msg = "param_override 不是有效 JSON"
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func updateChannelHeaderOverrideHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		HeaderOverride string `json:"header_override"`
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
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if err := opts.Store.UpdateUpstreamChannelHeaderOverride(c.Request.Context(), ch.ID, req.HeaderOverride); err != nil {
			msg := "保存失败"
			if strings.Contains(err.Error(), "header_override 不是有效 JSON") {
				msg = "header_override 不是有效 JSON"
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func updateChannelModelSuffixPreserveHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		ModelSuffixPreserve string `json:"model_suffix_preserve"`
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
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if err := opts.Store.UpdateUpstreamChannelModelSuffixPreserve(c.Request.Context(), ch.ID, req.ModelSuffixPreserve); err != nil {
			msg := "保存失败"
			if strings.Contains(err.Error(), "model_suffix_preserve") {
				msg = "model_suffix_preserve 不是有效 JSON 数组"
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func updateChannelRequestBodyWhitelistHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		RequestBodyWhitelist string `json:"request_body_whitelist"`
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
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if err := opts.Store.UpdateUpstreamChannelRequestBodyWhitelist(c.Request.Context(), ch.ID, req.RequestBodyWhitelist); err != nil {
			msg := "保存失败"
			if strings.Contains(err.Error(), "request_body_whitelist") {
				msg = "request_body_whitelist 不是有效 JSON 数组"
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func updateChannelRequestBodyBlacklistHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		RequestBodyBlacklist string `json:"request_body_blacklist"`
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
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if err := opts.Store.UpdateUpstreamChannelRequestBodyBlacklist(c.Request.Context(), ch.ID, req.RequestBodyBlacklist); err != nil {
			msg := "保存失败"
			if strings.Contains(err.Error(), "request_body_blacklist") {
				msg = "request_body_blacklist 不是有效 JSON 数组"
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func updateChannelStatusCodeMappingHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		StatusCodeMapping string `json:"status_code_mapping"`
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
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}
		var req reqBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if err := opts.Store.UpdateUpstreamChannelStatusCodeMapping(c.Request.Context(), ch.ID, req.StatusCodeMapping); err != nil {
			msg := "保存失败"
			if strings.Contains(err.Error(), "status_code_mapping") {
				msg = "status_code_mapping 不合法"
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func deleteChannelHandler(opts Options) gin.HandlerFunc {
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
		_, err = opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}
		if err := opts.Store.DeleteUpstreamChannel(c.Request.Context(), channelID); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}

func getChannelKeyHandler(opts Options) gin.HandlerFunc {
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
		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 channel 失败"})
			return
		}
		if ch.Type == store.UpstreamTypeCodexOAuth {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "codex_oauth Channel 不支持 key"})
			return
		}
		ep, err := opts.Store.GetUpstreamEndpointByChannelID(c.Request.Context(), ch.ID)
		if err != nil || ep.ID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "endpoint 不存在"})
			return
		}
		switch ch.Type {
		case store.UpstreamTypeOpenAICompatible:
			creds, err := opts.Store.ListOpenAICompatibleCredentialsByEndpoint(c.Request.Context(), ep.ID)
			if err != nil || len(creds) == 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "暂无可用 key"})
				return
			}
			sec, err := opts.Store.GetOpenAICompatibleCredentialSecret(c.Request.Context(), creds[0].ID)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取 key 失败"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "获取成功", "data": gin.H{"key": sec.APIKey}})
		case store.UpstreamTypeAnthropic:
			creds, err := opts.Store.ListAnthropicCredentialsByEndpoint(c.Request.Context(), ep.ID)
			if err != nil || len(creds) == 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "暂无可用 key"})
				return
			}
			sec, err := opts.Store.GetAnthropicCredentialSecret(c.Request.Context(), creds[0].ID)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取 key 失败"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "获取成功", "data": gin.H{"key": sec.APIKey}})
		default:
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "不支持的渠道类型"})
		}
	}
}

func testChannelHandler(opts Options) gin.HandlerFunc {
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
		ok, latencyMS, message, probe := runChannelCLITest(c.Request.Context(), opts, channelID)
		c.JSON(http.StatusOK, buildChannelTestResponse(ok, latencyMS, message, probe))
	}
}

func buildChannelTestResponse(ok bool, latencyMS int, message string, probe channelProbeSummary) gin.H {
	return gin.H{
		"success": ok,
		"message": message,
		"data": gin.H{
			"latency_ms": latencyMS,
			"probe":      probe,
		},
	}
}

// ---------------------------------------------------------------------------
// CLI Runner 委派（仅返回最终 JSON，不写数据库、不影响调度）
// ---------------------------------------------------------------------------

func channelTypeToCLIType(chType string) string {
	switch chType {
	case store.UpstreamTypeOpenAICompatible:
		return "codex"
	case store.UpstreamTypeAnthropic:
		return "claude"
	case store.UpstreamTypeCodexOAuth:
		return "codex_oauth"
	default:
		return ""
	}
}

type channelTestRunnerResponse struct {
	OK                    bool   `json:"ok"`
	LatencyMS             int    `json:"latency_ms"`
	TTFTMS                int    `json:"ttft_ms,omitempty"`
	Output                string `json:"output"`
	Error                 string `json:"error"`
	SuccessPath           string `json:"success_path,omitempty"`
	UsedFallback          bool   `json:"used_fallback,omitempty"`
	ForwardedModel        string `json:"forwarded_model,omitempty"`
	UpstreamResponseModel string `json:"upstream_response_model,omitempty"`
}

func readChannelTestRunnerResponse(resp *http.Response) (channelTestRunnerResponse, error) {
	if resp == nil || resp.Body == nil {
		return channelTestRunnerResponse{}, fmt.Errorf("CLI runner 无响应体")
	}
	var runnerResp channelTestRunnerResponse
	if err := json.NewDecoder(resp.Body).Decode(&runnerResp); err != nil {
		return channelTestRunnerResponse{}, err
	}
	if resp.StatusCode != http.StatusOK && strings.TrimSpace(runnerResp.Error) == "" {
		runnerResp.Error = fmt.Sprintf("CLI runner 返回 HTTP %d", resp.StatusCode)
	}
	return runnerResp, nil
}

func resolveRunnerModelCheck(runnerResp channelTestRunnerResponse, fallbackModel string) (*string, *string, modelcheck.Status) {
	forwardedModel := modelcheck.Optional(runnerResp.ForwardedModel)
	if forwardedModel == nil {
		forwardedModel = modelcheck.Optional(fallbackModel)
	}
	upstreamResponseModel := modelcheck.Optional(runnerResp.UpstreamResponseModel)
	return forwardedModel, upstreamResponseModel, modelcheck.StatusFrom(forwardedModel, upstreamResponseModel)
}

func channelTestFailureSummary(message string, total int) channelProbeSummary {
	return channelProbeSummary{
		OK:      false,
		Message: message,
		Source:  "cli_runner",
		Total:   total,
		Success: 0,
		Results: []channelModelProbeResult{},
	}
}

func runChannelCLITest(ctx context.Context, opts Options, channelID int64) (bool, int, string, channelProbeSummary) {
	startedAt := time.Now()
	finish := func(ok bool, message string, summary channelProbeSummary) (bool, int, string, channelProbeSummary) {
		latencyMS := int(time.Since(startedAt) / time.Millisecond)
		if latencyMS < 0 {
			latencyMS = 0
		}
		return ok, latencyMS, message, summary
	}

	st := opts.Store

	ch, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		return finish(false, "channel 不存在", channelProbeSummary{})
	}
	ep, err := st.GetUpstreamEndpointByChannelID(ctx, ch.ID)
	if err != nil || ep.ID <= 0 {
		return finish(false, "endpoint 不存在", channelProbeSummary{})
	}

	// 获取所有“已配置模型”（默认：以绑定模型为准；如设置 test_model，则优先插入到首位）。
	// 注意：这里测试的是 upstream_model（为空时回退为 public_id），与真实转发路径一致。
	models := make([]string, 0, 8)
	if ch.TestModel != nil && strings.TrimSpace(*ch.TestModel) != "" {
		models = append(models, strings.TrimSpace(*ch.TestModel))
	}
	if bindings, err := st.ListChannelModelsByChannelID(ctx, ch.ID); err == nil && len(bindings) > 0 {
		for _, b := range bindings {
			if b.Status != 1 {
				continue
			}
			m := strings.TrimSpace(b.UpstreamModel)
			if m == "" {
				m = strings.TrimSpace(b.PublicID)
			}
			if m == "" {
				continue
			}
			models = append(models, m)
		}
	}
	{
		seen := make(map[string]struct{}, len(models))
		uniq := make([]string, 0, len(models))
		for _, m := range models {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			if _, ok := seen[m]; ok {
				continue
			}
			seen[m] = struct{}{}
			uniq = append(uniq, m)
		}
		models = uniq
	}
	if len(models) == 0 {
		models = []string{""}
	}

	total := len(models)
	modelLabels := make([]string, 0, total)
	for _, m := range models {
		label := strings.TrimSpace(m)
		if label == "" {
			label = "(default)"
		}
		modelLabels = append(modelLabels, label)
	}

	// 其他渠道：走 CLI runner。
	if opts.ChannelTestCLIRunnerURL == "" {
		summary := channelTestFailureSummary("CLI runner 未配置，请设置 REALMS_CHANNEL_TEST_CLI_RUNNER_URL", total)
		return finish(false, summary.Message, summary)
	}

	cliType := channelTypeToCLIType(ch.Type)
	if cliType == "" {
		summary := channelTestFailureSummary(fmt.Sprintf("渠道类型 %s 暂不支持 CLI 测试", ch.Type), total)
		return finish(false, summary.Message, summary)
	}

	// 准备 CLI runner 凭证（仅用于测试请求，不写数据库、不影响调度）。
	apiKey := ""
	baseURL := ep.BaseURL
	profileKey := ""
	chatgptAccountID := ""
	accessToken := ""
	refreshToken := ""
	idToken := ""
	switch ch.Type {
	case store.UpstreamTypeOpenAICompatible:
		if creds, err := st.ListOpenAICompatibleCredentialsByEndpoint(ctx, ep.ID); err == nil && len(creds) > 0 {
			if sec, err := st.GetOpenAICompatibleCredentialSecret(ctx, creds[0].ID); err == nil {
				apiKey = sec.APIKey
			}
		}
		if apiKey == "" {
			summary := channelTestFailureSummary("未找到可用凭证", total)
			return finish(false, summary.Message, summary)
		}
	case store.UpstreamTypeAnthropic:
		if creds, err := st.ListAnthropicCredentialsByEndpoint(ctx, ep.ID); err == nil && len(creds) > 0 {
			if sec, err := st.GetAnthropicCredentialSecret(ctx, creds[0].ID); err == nil {
				apiKey = sec.APIKey
			}
		}
		if apiKey == "" {
			summary := channelTestFailureSummary("未找到可用凭证", total)
			return finish(false, summary.Message, summary)
		}
	case store.UpstreamTypeCodexOAuth:
		accounts, err := st.ListCodexOAuthAccountsByEndpoint(ctx, ep.ID)
		if err != nil {
			summary := channelTestFailureSummary("读取 Codex OAuth 账号失败", total)
			return finish(false, summary.Message, summary)
		}
		now := time.Now()
		var selected store.CodexOAuthAccount
		found := false
		for _, a := range accounts {
			if a.Status != 1 {
				continue
			}
			if a.CooldownUntil != nil && now.Before(*a.CooldownUntil) {
				continue
			}
			selected = a
			found = true
			break
		}
		if !found {
			summary := channelTestFailureSummary("暂无可用 Codex OAuth 账号（可能被禁用或冷却中）", total)
			return finish(false, summary.Message, summary)
		}
		sec, err := st.GetCodexOAuthSecret(ctx, selected.ID)
		if err != nil {
			summary := channelTestFailureSummary("读取 Codex OAuth 账号密钥失败", total)
			return finish(false, summary.Message, summary)
		}
		chatgptAccountID = strings.TrimSpace(sec.AccountID)
		accessToken = strings.TrimSpace(sec.AccessToken)
		refreshToken = strings.TrimSpace(sec.RefreshToken)
		if sec.IDToken != nil {
			idToken = strings.TrimSpace(*sec.IDToken)
		}
		// codex_oauth 的测试连接走 Codex CLI 内置 ChatGPT 上游，因此无需 base_url；
		// 但为避免不同 endpoint/account 的并发测试互相覆盖 runner 的 $CODEX_HOME，
		// 这里通过 profile_key 显式隔离 profile 目录。
		baseURL = ""
		profileKey = fmt.Sprintf("codex_oauth|endpoint:%d|account:%d", ep.ID, selected.ID)
		if chatgptAccountID == "" || accessToken == "" || refreshToken == "" || idToken == "" {
			summary := channelTestFailureSummary("Codex OAuth 账号缺少必要凭据（account_id/access_token/refresh_token/id_token），请重新授权", total)
			return finish(false, summary.Message, summary)
		}
	}

	runnerURL := strings.TrimRight(opts.ChannelTestCLIRunnerURL, "/") + "/v1/test"
	client := &http.Client{Timeout: 60 * time.Second}
	results := make([]channelModelProbeResult, total)

	success := 0
	runnerLatencySum := 0
	ttftSum := 0
	ttftCount := 0
	responsesOK := 0
	chatOK := 0
	fallbackCount := 0
	modelCheckOK := 0
	modelCheckMismatch := 0
	modelCheckUnknown := 0
	sample := ""
	firstError := ""
	firstErrorIdx := total + 1

	limit := opts.ChannelTestCLIConcurrency
	if limit <= 0 {
		limit = 4
	}
	if limit > total {
		limit = total
	}

	type job struct {
		idx        int
		model      string
		modelLabel string
	}

	type jobOutcome struct {
		result           channelModelProbeResult
		latencyMS        int
		ttftMS           int
		modelCheckStatus modelcheck.Status
	}

	var mu sync.Mutex
	jobs := make(chan job)
	var wg sync.WaitGroup

	runJob := func(j job) jobOutcome {
		runnerReq := struct {
			CLIType          string `json:"cli_type"`
			BaseURL          string `json:"base_url,omitempty"`
			ProfileKey       string `json:"profile_key,omitempty"`
			APIKey           string `json:"api_key"`
			ChatGPTAccountID string `json:"chatgpt_account_id,omitempty"`
			AccessToken      string `json:"access_token,omitempty"`
			RefreshToken     string `json:"refresh_token,omitempty"`
			IDToken          string `json:"id_token,omitempty"`
			Model            string `json:"model,omitempty"`
			Prompt           string `json:"prompt"`
			TimeoutSeconds   int    `json:"timeout_seconds"`
		}{
			CLIType:          cliType,
			BaseURL:          baseURL,
			ProfileKey:       profileKey,
			APIKey:           apiKey,
			ChatGPTAccountID: chatgptAccountID,
			AccessToken:      accessToken,
			RefreshToken:     refreshToken,
			IDToken:          idToken,
			Model:            j.model,
			Prompt:           "Reply with exactly: OK",
			TimeoutSeconds:   30,
		}
		body, _ := json.Marshal(runnerReq)

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, runnerURL, bytes.NewReader(body))
		if err != nil {
			result := channelModelProbeResult{
				Model:            j.modelLabel,
				OK:               false,
				Message:          "构建 CLI runner 请求失败",
				ForwardedModel:   strings.TrimSpace(j.model),
				ModelCheckStatus: string(modelcheck.StatusUnknown),
			}
			return jobOutcome{result: result, modelCheckStatus: modelcheck.StatusUnknown}
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			forwardedModel, upstreamResponseModel, modelCheckStatus := resolveRunnerModelCheck(channelTestRunnerResponse{}, j.model)
			msg := "CLI runner 不可达: " + err.Error()
			result := channelModelProbeResult{
				Model:            j.modelLabel,
				OK:               false,
				Message:          msg,
				ModelCheckStatus: string(modelCheckStatus),
			}
			if forwardedModel != nil {
				result.ForwardedModel = *forwardedModel
			}
			if upstreamResponseModel != nil {
				result.UpstreamResponseModel = *upstreamResponseModel
			}
			return jobOutcome{result: result, modelCheckStatus: modelCheckStatus}
		}

		runnerResp, decodeErr := readChannelTestRunnerResponse(resp)
		_ = resp.Body.Close()
		if decodeErr != nil {
			forwardedModel, upstreamResponseModel, modelCheckStatus := resolveRunnerModelCheck(channelTestRunnerResponse{}, j.model)
			result := channelModelProbeResult{
				Model:            j.modelLabel,
				OK:               false,
				Message:          "CLI runner 响应解析失败",
				ModelCheckStatus: string(modelCheckStatus),
			}
			if forwardedModel != nil {
				result.ForwardedModel = *forwardedModel
			}
			if upstreamResponseModel != nil {
				result.UpstreamResponseModel = *upstreamResponseModel
			}
			return jobOutcome{result: result, latencyMS: runnerResp.LatencyMS, modelCheckStatus: modelCheckStatus}
		}

		msg := runnerResp.Output
		if !runnerResp.OK {
			msg = runnerResp.Error
			if strings.TrimSpace(msg) == "" {
				msg = runnerResp.Output
			}
			forwardedModel, upstreamResponseModel, modelCheckStatus := resolveRunnerModelCheck(runnerResp, j.model)
			result := channelModelProbeResult{
				Model:            j.modelLabel,
				OK:               false,
				Message:          msg,
				Sample:           runnerResp.Output,
				ModelCheckStatus: string(modelCheckStatus),
				SuccessPath:      strings.TrimSpace(runnerResp.SuccessPath),
				UsedFallback:     runnerResp.UsedFallback,
			}
			if forwardedModel != nil {
				result.ForwardedModel = *forwardedModel
			}
			if upstreamResponseModel != nil {
				result.UpstreamResponseModel = *upstreamResponseModel
			}
			return jobOutcome{result: result, modelCheckStatus: modelCheckStatus}
		}

		forwardedModel, upstreamResponseModel, modelCheckStatus := resolveRunnerModelCheck(runnerResp, j.model)
		ttftMS := runnerResp.TTFTMS
		if ttftMS <= 0 {
			ttftMS = runnerResp.LatencyMS
		}

		result := channelModelProbeResult{
			Model:            j.modelLabel,
			OK:               true,
			Message:          msg,
			TTFTMS:           ttftMS,
			Sample:           runnerResp.Output,
			SuccessPath:      strings.TrimSpace(runnerResp.SuccessPath),
			UsedFallback:     runnerResp.UsedFallback,
			ModelCheckStatus: string(modelCheckStatus),
		}
		if forwardedModel != nil {
			result.ForwardedModel = *forwardedModel
		}
		if upstreamResponseModel != nil {
			result.UpstreamResponseModel = *upstreamResponseModel
		}

		return jobOutcome{
			result:           result,
			latencyMS:        runnerResp.LatencyMS,
			ttftMS:           ttftMS,
			modelCheckStatus: modelCheckStatus,
		}
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			outcome := runJob(j)
			result := outcome.result

			mu.Lock()
			results[j.idx] = result
			runnerLatencySum += outcome.latencyMS
			if result.OK && outcome.ttftMS > 0 {
				ttftSum += outcome.ttftMS
				ttftCount++
			}
			switch outcome.modelCheckStatus {
			case modelcheck.StatusOK:
				modelCheckOK++
			case modelcheck.StatusMismatch:
				modelCheckMismatch++
			default:
				modelCheckUnknown++
			}
			switch result.SuccessPath {
			case "/v1/responses":
				responsesOK++
			case "/v1/chat/completions":
				chatOK++
			}
			if result.UsedFallback {
				fallbackCount++
			}
			if sample == "" && strings.TrimSpace(result.Sample) != "" {
				sample = result.Sample
			}
			if result.OK {
				success++
			} else if j.idx < firstErrorIdx {
				firstErrorIdx = j.idx
				firstError = result.Message
			}
			mu.Unlock()
		}
	}

	wg.Add(limit)
	for i := 0; i < limit; i++ {
		go worker()
	}
	for idx, model := range models {
		jobs <- job{idx: idx, model: model, modelLabel: modelLabels[idx]}
	}
	close(jobs)
	wg.Wait()

	avgTTFT := 0
	if ttftCount > 0 {
		avgTTFT = ttftSum / ttftCount
	}
	okAll := total > 0 && success == total
	msgParts := []string{fmt.Sprintf("成功 %d/%d", success, total)}
	if modelCheckMismatch > 0 {
		msgParts = append(msgParts, fmt.Sprintf("模型不一致 %d", modelCheckMismatch))
	}
	if modelCheckUnknown > 0 {
		msgParts = append(msgParts, fmt.Sprintf("模型未知 %d", modelCheckUnknown))
	}
	message := strings.Join(msgParts, "，")
	if !okAll && firstError != "" {
		message = fmt.Sprintf("%s：%s", message, firstError)
	}

	summary := channelProbeSummary{
		OK:                 okAll,
		Message:            message,
		Source:             "cli_runner",
		Total:              total,
		Success:            success,
		ResponsesOK:        responsesOK,
		ChatOK:             chatOK,
		FallbackCount:      fallbackCount,
		AvgTTFTMS:          avgTTFT,
		LatencyMS:          runnerLatencySum,
		Sample:             sample,
		Results:            results,
		ModelCheckOK:       modelCheckOK,
		ModelCheckMismatch: modelCheckMismatch,
		ModelCheckUnknown:  modelCheckUnknown,
	}

	// CRITICAL: 不调用 UpdateUpstreamChannelTest，不落库任何测试结果。
	return finish(okAll, message, summary)
}

type channelModelProbeResult struct {
	Model                 string `json:"model"`
	OK                    bool   `json:"ok"`
	Message               string `json:"message"`
	SuccessPath           string `json:"success_path,omitempty"`
	UsedFallback          bool   `json:"used_fallback,omitempty"`
	TTFTMS                int    `json:"ttft_ms,omitempty"`
	Sample                string `json:"sample,omitempty"`
	ForwardedModel        string `json:"forwarded_model,omitempty"`
	UpstreamResponseModel string `json:"upstream_response_model,omitempty"`
	ModelCheckStatus      string `json:"model_check_status,omitempty"`
}

type channelProbeSummary struct {
	OK                 bool                      `json:"ok"`
	Message            string                    `json:"message"`
	Source             string                    `json:"source,omitempty"`
	Total              int                       `json:"total"`
	Success            int                       `json:"success"`
	ResponsesOK        int                       `json:"responses_ok"`
	ChatOK             int                       `json:"chat_ok"`
	FallbackCount      int                       `json:"fallback_count"`
	AvgTTFTMS          int                       `json:"avg_ttft_ms,omitempty"`
	Sample             string                    `json:"sample,omitempty"`
	LatencyMS          int                       `json:"latency_ms,omitempty"`
	ModelCheckOK       int                       `json:"model_check_ok"`
	ModelCheckMismatch int                       `json:"model_check_mismatch"`
	ModelCheckUnknown  int                       `json:"model_check_unknown"`
	Results            []channelModelProbeResult `json:"results"`
}
