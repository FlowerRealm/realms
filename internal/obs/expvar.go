package obs

import (
	"expvar"
	"sync/atomic"
)

var (
	activeSSEConnections int64
	totalSSEConnections  int64
)

func init() {
	expvar.Publish("active_sse_connections", expvar.Func(func() any {
		return atomic.LoadInt64(&activeSSEConnections)
	}))
	expvar.Publish("total_sse_connections", expvar.Func(func() any {
		return atomic.LoadInt64(&totalSSEConnections)
	}))
}

// TrackSSEConnection increments active/total SSE connection counters and returns
// a function that should be deferred to decrement the active counter.
func TrackSSEConnection() func() {
	atomic.AddInt64(&activeSSEConnections, 1)
	atomic.AddInt64(&totalSSEConnections, 1)
	return func() {
		atomic.AddInt64(&activeSSEConnections, -1)
	}
}
