package obs

import (
	"expvar"
	"sync/atomic"
	"time"
)

var (
	sseFirstWriteSamples int64
	sseFirstWriteSumMS   int64
	sseBytesStreamed     int64
	sseDoneMissing       int64

	ssePumpResults = expvar.NewMap("sse_pump_results_total")
)

func init() {
	expvar.Publish("sse_first_write_ms_sum", expvar.Func(func() any {
		return atomic.LoadInt64(&sseFirstWriteSumMS)
	}))
	expvar.Publish("sse_first_write_samples", expvar.Func(func() any {
		return atomic.LoadInt64(&sseFirstWriteSamples)
	}))
	expvar.Publish("sse_bytes_streamed_total", expvar.Func(func() any {
		return atomic.LoadInt64(&sseBytesStreamed)
	}))
	expvar.Publish("sse_done_missing_total", expvar.Func(func() any {
		return atomic.LoadInt64(&sseDoneMissing)
	}))
}

func RecordSSEFirstWriteLatency(d time.Duration) {
	if d <= 0 {
		return
	}
	atomic.AddInt64(&sseFirstWriteSamples, 1)
	atomic.AddInt64(&sseFirstWriteSumMS, d.Milliseconds())
}

func RecordSSEBytesStreamed(n int64) {
	if n <= 0 {
		return
	}
	atomic.AddInt64(&sseBytesStreamed, n)
}

func RecordSSEPumpResult(errorClass string, sawDone bool) {
	if !sawDone && errorClass == "" {
		atomic.AddInt64(&sseDoneMissing, 1)
	}

	key := "ok"
	if errorClass != "" {
		key = errorClass
	}
	if v := ssePumpResults.Get(key); v != nil {
		v.(*expvar.Int).Add(1)
		return
	}
	i := new(expvar.Int)
	i.Add(1)
	ssePumpResults.Set(key, i)
}
