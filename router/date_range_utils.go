package router

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func allTimeStartEndByFirst(first time.Time, loc *time.Location, todayStr string) (start string, end string) {
	if loc == nil {
		loc = time.UTC
	}
	firstLocal := first.In(loc)
	start = time.Date(firstLocal.Year(), firstLocal.Month(), firstLocal.Day(), 0, 0, 0, 0, loc).Format("2006-01-02")
	end = todayStr
	return start, end
}

func resolveAllTimeGlobalStartEnd(c *gin.Context, opts Options, loc *time.Location, todayStr string) (start string, end string, has bool, ok bool) {
	first, has, err := opts.Store.GetFirstUsageEventTimeGlobal(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
		return "", "", false, false
	}
	if !has {
		return "", "", false, true
	}
	start, end = allTimeStartEndByFirst(first, loc, todayStr)
	return start, end, true, true
}

func resolveAllTimeChannelStartEnd(c *gin.Context, opts Options, loc *time.Location, todayStr string, channelID int64) (start string, end string, has bool, ok bool) {
	first, has, err := opts.Store.GetFirstUsageEventTimeByChannel(c.Request.Context(), channelID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
		return "", "", false, false
	}
	if !has {
		return "", "", false, true
	}
	start, end = allTimeStartEndByFirst(first, loc, todayStr)
	return start, end, true, true
}

type usageResolvedRange struct {
	since      time.Time
	until      time.Time
	sinceLocal time.Time
	untilLocal time.Time
}

func parseUsageResolvedRange(c *gin.Context, nowUTC time.Time, startStr string, endStr string, loc *time.Location) (usageResolvedRange, bool) {
	since, until, sinceLocal, untilLocal, ok := parseDateRangeInLocation(nowUTC, startStr, endStr, loc)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "start/end 不合法（格式：YYYY-MM-DD）"})
		return usageResolvedRange{}, false
	}
	return usageResolvedRange{since: since, until: until, sinceLocal: sinceLocal, untilLocal: untilLocal}, true
}

func resolveUsageDateRange(
	c *gin.Context,
	opts Options,
	loc *time.Location,
	nowUTC time.Time,
	startStr string,
	endStr string,
	userID int64,
	tokenID int64,
	allTime bool,
) (usageResolvedRange, bool) {
	if !allTime {
		return parseUsageResolvedRange(c, nowUTC, startStr, endStr, loc)
	}

	var first time.Time
	var has bool
	var err error
	if tokenID > 0 {
		first, has, err = opts.Store.GetFirstUsageEventTimeByToken(c.Request.Context(), tokenID)
	} else {
		first, has, err = opts.Store.GetFirstUsageEventTimeByUser(c.Request.Context(), userID)
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
		return usageResolvedRange{}, false
	}
	if !has {
		// No data: keep old semantics (empty range means today).
		return parseUsageResolvedRange(c, nowUTC, startStr, endStr, loc)
	}

	firstLocal := first.In(loc)
	sinceLocal := time.Date(firstLocal.Year(), firstLocal.Month(), firstLocal.Day(), 0, 0, 0, 0, loc)
	untilLocal := nowUTC.In(loc)
	return usageResolvedRange{
		since:      sinceLocal.UTC(),
		until:      untilLocal.UTC(),
		sinceLocal: sinceLocal,
		untilLocal: untilLocal,
	}, true
}
