package router

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/codexoauth"
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
	admin := requireRootSession(opts)

	r.GET("/channel", admin, listChannelsHandler(opts))
	r.GET("/channel/", admin, listChannelsHandler(opts))
	r.GET("/channel/page", admin, channelsPageHandler(opts))
	r.GET("/channel/page/", admin, channelsPageHandler(opts))
	r.GET("/channel/pinned", admin, pinnedChannelInfoHandler(opts))

	r.POST("/channel", admin, createChannelHandler(opts))
	r.POST("/channel/", admin, createChannelHandler(opts))

	r.PUT("/channel", admin, updateChannelHandler(opts))
	r.PUT("/channel/", admin, updateChannelHandler(opts))

	r.GET("/channel/:channel_id", admin, getChannelHandler(opts))
	r.DELETE("/channel/:channel_id", admin, deleteChannelHandler(opts))
	r.DELETE("/channel/:channel_id/", admin, deleteChannelHandler(opts))
	r.POST("/channel/:channel_id/promote", admin, pinChannelHandler(opts))

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

	r.GET("/channel/test", admin, testAllChannelsHandler(opts))
	r.GET("/channel/test/:channel_id", admin, testChannelHandler(opts))

	r.POST("/channel/reorder", admin, reorderChannelsHandler(opts))
	r.POST("/channel/reorder/", admin, reorderChannelsHandler(opts))
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
	Requests             int64              `json:"requests"`
	Tokens               int64              `json:"tokens"`
	CommittedUSD         string             `json:"committed_usd"`
	CacheRatio           string             `json:"cache_ratio"`
	AvgFirstTokenLatency string             `json:"avg_first_token_latency"`
	TokensPerSecond      string             `json:"tokens_per_second"`
	BindingRuntime       bindingRuntimeInfo `json:"binding_runtime"`
}

type channelAdminListItem struct {
	channelView
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

			out = append(out, channelAdminListItem{
				channelView: view,
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
					BindingRuntime:       bindingRuntimeForAPI(opts),
				},
				Channels: out,
			},
		})
	}
}

func reorderChannelsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		var ids []int64
		if err := json.NewDecoder(c.Request.Body).Decode(&ids); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		seen := make(map[int64]struct{}, len(ids))
		cleaned := make([]int64, 0, len(ids))
		for _, id := range ids {
			if id <= 0 {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			cleaned = append(cleaned, id)
		}
		if len(cleaned) == 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "ids 不能为空"})
			return
		}

		channels, err := opts.Store.ListUpstreamChannels(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询渠道失败"})
			return
		}
		if len(cleaned) != len(channels) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "渠道列表不完整，请刷新后重试"})
			return
		}

		existing := make(map[int64]struct{}, len(channels))
		for _, ch := range channels {
			existing[ch.ID] = struct{}{}
		}
		for _, id := range cleaned {
			if _, ok := existing[id]; !ok {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "存在未知的 channel_id，请刷新后重试"})
				return
			}
		}
		if len(seen) != len(existing) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "渠道列表不完整，请刷新后重试"})
			return
		}

		if err := opts.Store.ReorderUpstreamChannels(c.Request.Context(), cleaned); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存排序失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存排序"})
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
	AllowServiceTier      bool    `json:"allow_service_tier"`
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

		id, err := opts.Store.CreateUpstreamChannel(c.Request.Context(), req.Type, req.Name, req.Groups, req.Priority, req.Promotion, req.AllowServiceTier, req.DisableStore, req.AllowSafetyIdentifier)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败"})
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
	DisableStore          *bool   `json:"disable_store,omitempty"`
	AllowSafetyIdentifier *bool   `json:"allow_safety_identifier,omitempty"`
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
		disableStore := ch.DisableStore
		if req.DisableStore != nil {
			disableStore = *req.DisableStore
		}
		allowSafetyIdentifier := ch.AllowSafetyIdentifier
		if req.AllowSafetyIdentifier != nil {
			allowSafetyIdentifier = *req.AllowSafetyIdentifier
		}

		if err := opts.Store.UpdateUpstreamChannelBasics(c.Request.Context(), ch.ID, name, status, priority, promotion, allowServiceTier, disableStore, allowSafetyIdentifier); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新失败"})
			return
		}

		if req.Groups != nil {
			if err := opts.Store.SetUpstreamChannelGroups(c.Request.Context(), ch.ID, strings.TrimSpace(*req.Groups)); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新分组失败"})
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
		actorUserID, ok := userIDFromContext(c)
		if !ok || actorUserID <= 0 {
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
		actorUserID, ok := userIDFromContext(c)
		if !ok || actorUserID <= 0 {
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

type channelTestResult struct {
	ChannelID int64  `json:"channel_id"`
	OK        bool   `json:"ok"`
	LatencyMS int    `json:"latency_ms"`
	Message   string `json:"message"`
}

func testAllChannelsHandler(opts Options) gin.HandlerFunc {
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
		out := make([]channelTestResult, 0, len(channels))
		for _, ch := range channels {
			if ch.ID <= 0 || ch.Type == store.UpstreamTypeCodexOAuth {
				continue
			}
			ok, latency, msg := testChannelOnce(c.Request.Context(), opts.Store, ch.ID)
			out = append(out, channelTestResult{ChannelID: ch.ID, OK: ok, LatencyMS: latency, Message: msg})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
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
		ok, latency, msg := testChannelOnce(c.Request.Context(), opts.Store, channelID)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": msg, "data": gin.H{"latency_ms": latency}})
	}
}

func testChannelOnce(ctx context.Context, st *store.Store, channelID int64) (ok bool, latencyMS int, message string) {
	if st == nil {
		return false, 0, "store 未初始化"
	}
	ch, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, 0, "channel 不存在"
		}
		return false, 0, "查询 channel 失败"
	}
	if ch.Type == store.UpstreamTypeCodexOAuth {
		return false, 0, "codex_oauth Channel 不支持测试"
	}

	ep, err := st.GetUpstreamEndpointByChannelID(ctx, ch.ID)
	if err != nil || ep.ID <= 0 {
		return false, 0, "endpoint 不存在"
	}

	start := time.Now()
	ok, msg := probeUpstream(ctx, st, ch, ep)
	latencyMS = int(time.Since(start).Milliseconds())
	if latencyMS < 0 {
		latencyMS = 0
	}
	_ = st.UpdateUpstreamChannelTest(ctx, ch.ID, ok, latencyMS)
	return ok, latencyMS, msg
}

func probeUpstream(ctx context.Context, st *store.Store, ch store.UpstreamChannel, ep store.UpstreamEndpoint) (bool, string) {
	targetPath := "/v1/models"
	method := http.MethodGet
	var body io.Reader

	h := make(http.Header)
	h.Set("Accept", "application/json")
	h.Set("User-Agent", "realms-channel-test/1.0")

	switch ch.Type {
	case store.UpstreamTypeOpenAICompatible:
		creds, err := st.ListOpenAICompatibleCredentialsByEndpoint(ctx, ep.ID)
		if err != nil || len(creds) == 0 {
			return false, "暂无可用 key"
		}
		sec, err := st.GetOpenAICompatibleCredentialSecret(ctx, creds[0].ID)
		if err != nil || strings.TrimSpace(sec.APIKey) == "" {
			return false, "读取 key 失败"
		}
		h.Set("Authorization", "Bearer "+strings.TrimSpace(sec.APIKey))
	case store.UpstreamTypeAnthropic:
		// 只做连通性探测：POST /v1/messages 发送空 JSON，期望返回 400（参数错误）或 2xx。
		method = http.MethodPost
		targetPath = "/v1/messages"
		body = strings.NewReader(`{}`)
		h.Set("Content-Type", "application/json; charset=utf-8")
		h.Set("anthropic-version", "2023-06-01")
		creds, err := st.ListAnthropicCredentialsByEndpoint(ctx, ep.ID)
		if err != nil || len(creds) == 0 {
			return false, "暂无可用 key"
		}
		sec, err := st.GetAnthropicCredentialSecret(ctx, creds[0].ID)
		if err != nil || strings.TrimSpace(sec.APIKey) == "" {
			return false, "读取 key 失败"
		}
		h.Set("x-api-key", strings.TrimSpace(sec.APIKey))
	default:
		return false, "不支持的渠道类型"
	}

	u, err := buildUpstreamURL(ep.BaseURL, targetPath)
	if err != nil {
		return false, "base_url 不合法"
	}

	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return false, "创建请求失败"
	}
	for k, vs := range h {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, "请求失败"
	}
	defer resp.Body.Close()

	switch ch.Type {
	case store.UpstreamTypeAnthropic:
		// 400 表示服务可达但参数不完整；401/403 表示 key 不可用。
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return true, "OK"
		}
		if resp.StatusCode == http.StatusBadRequest {
			return true, "OK（400 参数错误，连通性正常）"
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return false, "鉴权失败（" + strconv.Itoa(resp.StatusCode) + "）"
		}
		if len(b) > 0 {
			return false, "失败（" + strconv.Itoa(resp.StatusCode) + "）: " + strings.TrimSpace(string(b))
		}
		return false, "失败（" + strconv.Itoa(resp.StatusCode) + "）"
	default:
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return true, "OK"
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return false, "鉴权失败（" + strconv.Itoa(resp.StatusCode) + "）"
		}
		if len(b) > 0 {
			return false, "失败（" + strconv.Itoa(resp.StatusCode) + "）: " + strings.TrimSpace(string(b))
		}
		return false, "失败（" + strconv.Itoa(resp.StatusCode) + "）"
	}
}

func buildUpstreamURL(baseURL string, targetPath string) (string, error) {
	base, err := security.ValidateBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	basePath := strings.TrimRight(base.Path, "/")
	if strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(targetPath, "/v1/") {
		targetPath = strings.TrimPrefix(targetPath, "/v1")
		if targetPath == "" {
			targetPath = "/"
		}
	}
	return base.ResolveReference(&url.URL{Path: targetPath}).String(), nil
}
