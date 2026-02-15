package obs

import (
	"expvar"
	"sync"
	"sync/atomic"
	"time"
)

var (
	cacheInvalidationPollTicks  int64
	cacheInvalidationPollErrors int64

	cacheInvalidationVersionMu sync.Mutex
	cacheInvalidationVersions  = expvar.NewMap("cache_invalidation_versions")
)

func init() {
	expvar.Publish("cache_invalidation_poller_ticks_total", expvar.Func(func() any {
		return atomic.LoadInt64(&cacheInvalidationPollTicks)
	}))
	expvar.Publish("cache_invalidation_poller_errors_total", expvar.Func(func() any {
		return atomic.LoadInt64(&cacheInvalidationPollErrors)
	}))
	expvar.Publish("cache_invalidation_poller_last_ok_unix", expvar.Func(func() any {
		return atomic.LoadInt64(&cacheInvalidationLastOKUnix)
	}))
}

var cacheInvalidationLastOKUnix int64

func RecordCacheInvalidationPollTick(ok bool) {
	atomic.AddInt64(&cacheInvalidationPollTicks, 1)
	if ok {
		atomic.StoreInt64(&cacheInvalidationLastOKUnix, time.Now().Unix())
	}
}

func RecordCacheInvalidationPollError() {
	atomic.AddInt64(&cacheInvalidationPollErrors, 1)
}

func SetCacheInvalidationVersion(key string, version int64) {
	if key == "" {
		return
	}
	v := new(expvar.Int)
	v.Set(version)
	cacheInvalidationVersionMu.Lock()
	cacheInvalidationVersions.Set(key, v)
	cacheInvalidationVersionMu.Unlock()
}
