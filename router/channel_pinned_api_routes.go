package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"realms/internal/store"
)

type pinnedChannelInfo struct {
	Available       bool   `json:"available"`
	PinnedActive    bool   `json:"pinned_active"`
	PinnedChannelID int64  `json:"pinned_channel_id"`
	PinnedChannel   string `json:"pinned_channel"`
	PinnedNote      string `json:"pinned_note,omitempty"`
}

func pinnedChannelInfoFromPointerState(ctx context.Context, opts Options, st store.SchedulerChannelPointerState) pinnedChannelInfo {
	info := pinnedChannelInfo{Available: opts.Sched != nil}
	if st.ChannelID <= 0 {
		return info
	}

	id := st.ChannelID
	info.PinnedActive = true
	info.PinnedChannelID = id

	name := ""
	if opts.Store != nil {
		if ch, err := opts.Store.GetUpstreamChannelByID(ctx, id); err == nil {
			name = strings.TrimSpace(ch.Name)
		}
	}
	if name != "" {
		info.PinnedChannel = name
	} else {
		info.PinnedChannel = "渠道"
	}

	info.PinnedNote = pinnedNote(ctx, opts, st.MovedAt(), st.Reason)
	return info
}

func pinnedChannelInfoHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		info := pinnedChannelInfo{Available: opts.Sched != nil}

		// 多实例：优先使用 app_settings 中的全局渠道指针运行态。
		if opts.Store != nil {
			if raw, ok, err := opts.Store.GetAppSetting(c.Request.Context(), store.SettingSchedulerChannelPointer); err == nil && ok {
				if st, ok2, err := store.ParseSchedulerChannelPointerState(raw); err == nil && ok2 {
					info = pinnedChannelInfoFromPointerState(c.Request.Context(), opts, st)
					c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": info})
					return
				}
			}
		}

		if opts.Sched == nil {
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": info})
			return
		}

		id, movedAt, reason, ok := opts.Sched.PinnedChannelInfo()
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": info})
			return
		}
		info.PinnedActive = true
		info.PinnedChannelID = id

		name := ""
		if opts.Store != nil && id > 0 {
			if ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), id); err == nil {
				name = strings.TrimSpace(ch.Name)
			}
		}
		if name != "" {
			info.PinnedChannel = name
		} else {
			info.PinnedChannel = "渠道"
		}

		info.PinnedNote = pinnedNote(c.Request.Context(), opts, movedAt, reason)

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": info})
	}
}

func pinnedChannelStreamHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "当前连接不支持流式输出"})
			return
		}

		header := c.Writer.Header()
		header.Set("Content-Type", "text/event-stream")
		header.Set("Cache-Control", "no-cache, no-transform")
		header.Set("Connection", "keep-alive")
		header.Set("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)
		flusher.Flush()

		writeEvent := func(name string, payload any) bool {
			b, err := json.Marshal(payload)
			if err != nil {
				return false
			}
			if _, err := c.Writer.Write([]byte("event: " + name + "\n")); err != nil {
				return false
			}
			if _, err := c.Writer.Write([]byte("data: " + string(b) + "\n\n")); err != nil {
				return false
			}
			flusher.Flush()
			return true
		}

		// 初始推送一次（即使暂时没有指针记录，也推送 inactive 结构，便于前端统一处理）。
		lastRaw := ""
		if raw, ok, err := opts.Store.GetAppSetting(c.Request.Context(), store.SettingSchedulerChannelPointer); err == nil && ok {
			lastRaw = raw
			if st, ok2, err := store.ParseSchedulerChannelPointerState(raw); err == nil && ok2 {
				_ = writeEvent("pinned", pinnedChannelInfoFromPointerState(c.Request.Context(), opts, st))
			} else {
				_ = writeEvent("pinned", pinnedChannelInfo{Available: opts.Sched != nil})
			}
		} else {
			_ = writeEvent("pinned", pinnedChannelInfo{Available: opts.Sched != nil})
		}

		poll := time.NewTicker(1 * time.Second)
		defer poll.Stop()
		keepalive := time.NewTicker(15 * time.Second)
		defer keepalive.Stop()

		for {
			select {
			case <-c.Request.Context().Done():
				return
			case <-keepalive.C:
				_, _ = c.Writer.Write([]byte(": keepalive\n\n"))
				flusher.Flush()
			case <-poll.C:
				raw, ok, err := opts.Store.GetAppSetting(c.Request.Context(), store.SettingSchedulerChannelPointer)
				if err != nil || !ok {
					continue
				}
				if raw == lastRaw {
					continue
				}
				st, ok2, err := store.ParseSchedulerChannelPointerState(raw)
				if err != nil || !ok2 {
					continue
				}
				lastRaw = raw
				if !writeEvent("pinned", pinnedChannelInfoFromPointerState(c.Request.Context(), opts, st)) {
					return
				}
			}
		}
	}
}

func pinChannelHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if opts.Sched == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "调度器未初始化"})
			return
		}

		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}

		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil || ch.ID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
			return
		}

		if err := opts.Sched.RefreshPinnedRing(c.Request.Context()); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "构建渠道指针失败：" + err.Error()})
			return
		}
		opts.Sched.PinChannel(ch.ID)
		opts.Sched.ClearChannelBan(ch.ID)

		c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("已将当前渠道指针指向 %s", ch.Name)})
	}
}

func pinnedNote(ctx context.Context, opts Options, movedAt time.Time, reason string) string {
	reasonText := ""
	switch strings.TrimSpace(reason) {
	case "manual":
		reasonText = "手动设置"
	case "ban":
		reasonText = "因封禁轮转"
	case "invalid":
		reasonText = "指针无效修正"
	case "route":
		reasonText = "路由选中"
	default:
		reasonText = strings.TrimSpace(reason)
	}

	movedAtText := ""
	if !movedAt.IsZero() {
		loc, _ := adminTimeLocation(ctx, opts)
		movedAtText = formatTimeIn(movedAt, "2006-01-02 15:04:05", loc)
	}

	switch {
	case movedAtText != "" && reasonText != "":
		return "更新时间：" + movedAtText + "；原因：" + reasonText
	case movedAtText != "":
		return "更新时间：" + movedAtText
	case reasonText != "":
		return "原因：" + reasonText
	default:
		return ""
	}
}
