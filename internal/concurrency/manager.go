package concurrency

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	mathrand "math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrQueueFull   = errors.New("concurrency queue full")
	ErrWaitTimeout = errors.New("concurrency wait timeout")
)

type Options struct {
	Addr      string
	Password  string
	DB        int
	KeyPrefix string

	SlotTTL     time.Duration
	WaitTTL     time.Duration
	WaitTimeout time.Duration

	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	WaitQueueExtra int
}

func deriveWaitTTL(waitTimeout, maxBackoff time.Duration) time.Duration {
	if waitTimeout <= 0 {
		waitTimeout = 30 * time.Second
	}
	if maxBackoff <= 0 {
		maxBackoff = 2 * time.Second
	}
	return waitTimeout + maxBackoff + time.Second
}

func DefaultOptions() Options {
	return Options{
		KeyPrefix:      "realms",
		SlotTTL:        30 * time.Minute,
		WaitTimeout:    30 * time.Second,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
		WaitQueueExtra: 20,
		WaitTTL:        deriveWaitTTL(30*time.Second, 2*time.Second),
	}
}

type Manager struct {
	client *redis.Client
	opts   Options

	seq atomic.Uint64
}

func NewManager(opts Options) *Manager {
	opts.Addr = strings.TrimSpace(opts.Addr)
	if strings.TrimSpace(opts.KeyPrefix) == "" {
		opts.KeyPrefix = "realms"
	}
	if opts.SlotTTL <= 0 {
		opts.SlotTTL = 30 * time.Minute
	}
	if opts.WaitTimeout <= 0 {
		opts.WaitTimeout = 30 * time.Second
	}
	if opts.InitialBackoff <= 0 {
		opts.InitialBackoff = 100 * time.Millisecond
	}
	if opts.MaxBackoff <= 0 {
		opts.MaxBackoff = 2 * time.Second
	}
	if opts.MaxBackoff < opts.InitialBackoff {
		opts.MaxBackoff = opts.InitialBackoff
	}
	if opts.WaitTTL <= 0 {
		opts.WaitTTL = deriveWaitTTL(opts.WaitTimeout, opts.MaxBackoff)
	}
	if opts.WaitQueueExtra < 0 {
		opts.WaitQueueExtra = 0
	}
	if opts.Addr == "" {
		return &Manager{opts: opts}
	}

	client := redis.NewClient(&redis.Options{
		Addr:     opts.Addr,
		Password: opts.Password,
		DB:       opts.DB,
	})
	return &Manager{
		client: client,
		opts:   opts,
	}
}

func (m *Manager) Enabled() bool {
	return m != nil && m.client != nil
}

func (m *Manager) PingContext(ctx context.Context) error {
	if m == nil || m.client == nil {
		return nil
	}
	return m.client.Ping(ctx).Err()
}

func (m *Manager) Close() error {
	if m == nil || m.client == nil {
		return nil
	}
	return m.client.Close()
}

func (m *Manager) AcquireUserSlotWithWait(ctx context.Context, userID int64, maxConcurrency int, onPing func() error) (func(), error) {
	if maxConcurrency <= 0 || !m.Enabled() || userID <= 0 {
		return func() {}, nil
	}
	scope := "user:" + strconv.FormatInt(userID, 10)
	return m.acquireWithWait(ctx, scope, maxConcurrency, onPing)
}

func (m *Manager) AcquireCredentialSlotWithWait(ctx context.Context, credentialKey string, maxConcurrency int) (func(), error) {
	if maxConcurrency <= 0 || !m.Enabled() || strings.TrimSpace(credentialKey) == "" {
		return func() {}, nil
	}
	scope := "cred:" + strings.TrimSpace(credentialKey)
	return m.acquireWithWait(ctx, scope, maxConcurrency, nil)
}

func (m *Manager) acquireWithWait(ctx context.Context, scope string, maxConcurrency int, onPing func() error) (func(), error) {
	requestID := m.requestID()
	slotKey := m.key("slot:" + scope)
	waitKey := m.key("wait:" + scope)

	maxWait := maxConcurrency + m.opts.WaitQueueExtra
	if maxWait <= 0 {
		maxWait = maxConcurrency
	}

	waitAdded := false
	ok, err := m.incrementWaitCount(ctx, waitKey, maxWait)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrQueueFull
	}
	waitAdded = true
	defer func() {
		if waitAdded {
			m.decrementWaitCount(context.Background(), waitKey)
		}
	}()

	acquired, err := m.tryAcquireSlot(ctx, slotKey, maxConcurrency, requestID)
	if err != nil {
		return nil, err
	}
	if acquired {
		waitAdded = false
		m.decrementWaitCount(context.Background(), waitKey)
		return m.releaseFn(slotKey, requestID), nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, m.opts.WaitTimeout)
	defer cancel()

	backoff := m.opts.InitialBackoff
	timer := time.NewTimer(backoff)
	defer timer.Stop()

	var pingTicker *time.Ticker
	if onPing != nil {
		pingTicker = time.NewTicker(10 * time.Second)
		defer pingTicker.Stop()
	}

	for {
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return nil, ErrWaitTimeout
			}
			return nil, waitCtx.Err()
		case <-timer.C:
			acquired, err := m.tryAcquireSlot(waitCtx, slotKey, maxConcurrency, requestID)
			if err != nil {
				return nil, err
			}
			if acquired {
				waitAdded = false
				m.decrementWaitCount(context.Background(), waitKey)
				return m.releaseFn(slotKey, requestID), nil
			}
			backoff = m.nextBackoff(backoff)
			timer.Reset(backoff)
		default:
			if pingTicker != nil {
				select {
				case <-waitCtx.Done():
					if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
						return nil, ErrWaitTimeout
					}
					return nil, waitCtx.Err()
				case <-pingTicker.C:
					if err := onPing(); err != nil {
						return nil, err
					}
				default:
					time.Sleep(10 * time.Millisecond)
				}
				continue
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (m *Manager) releaseFn(slotKey, requestID string) func() {
	return func() {
		bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = redisReleaseSlot.Run(bg, m.client, []string{slotKey}, requestID).Result()
	}
}

func (m *Manager) key(suffix string) string {
	prefix := strings.TrimSpace(m.opts.KeyPrefix)
	if prefix == "" {
		prefix = "realms"
	}
	return prefix + ":" + suffix
}

func (m *Manager) requestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:]) + "-" + strconv.FormatUint(m.seq.Add(1), 36)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.FormatUint(m.seq.Add(1), 36)
}

func (m *Manager) nextBackoff(current time.Duration) time.Duration {
	next := time.Duration(float64(current) * 1.5)
	if next > m.opts.MaxBackoff {
		next = m.opts.MaxBackoff
	}
	jitter := 0.8 + mathrand.Float64()*0.4
	withJitter := time.Duration(float64(next) * jitter)
	if withJitter < m.opts.InitialBackoff {
		return m.opts.InitialBackoff
	}
	if withJitter > m.opts.MaxBackoff {
		return m.opts.MaxBackoff
	}
	return withJitter
}

func (m *Manager) incrementWaitCount(ctx context.Context, waitKey string, maxWait int) (bool, error) {
	if maxWait <= 0 {
		return false, nil
	}
	v, err := redisIncrementWaitCount.Run(
		ctx,
		m.client,
		[]string{waitKey},
		maxWait,
		m.opts.WaitTTL.Milliseconds(),
	).Int()
	if err != nil {
		return false, err
	}
	return v == 1, nil
}

func (m *Manager) decrementWaitCount(ctx context.Context, waitKey string) {
	if m == nil || m.client == nil {
		return
	}
	_, _ = redisDecrementWaitCount.Run(ctx, m.client, []string{waitKey}).Result()
}

func (m *Manager) tryAcquireSlot(ctx context.Context, slotKey string, maxConcurrency int, requestID string) (bool, error) {
	if maxConcurrency <= 0 {
		return false, nil
	}
	v, err := redisAcquireSlot.Run(
		ctx,
		m.client,
		[]string{slotKey},
		time.Now().UnixMilli(),
		m.opts.SlotTTL.Milliseconds(),
		maxConcurrency,
		requestID,
	).Int()
	if err != nil {
		return false, err
	}
	return v == 1, nil
}

var redisAcquireSlot = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local max = tonumber(ARGV[3])
local req = ARGV[4]
redis.call("ZREMRANGEBYSCORE", key, "-inf", now-ttl)
local current = redis.call("ZCARD", key)
if current >= max then
  return 0
end
redis.call("ZADD", key, now, req)
redis.call("PEXPIRE", key, ttl*2)
return 1
`)

var redisReleaseSlot = redis.NewScript(`
redis.call("ZREM", KEYS[1], ARGV[1])
return 1
`)

var redisIncrementWaitCount = redis.NewScript(`
local key = KEYS[1]
local max = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local current = tonumber(redis.call("GET", key) or "0")
if current >= max then
  return 0
end
current = redis.call("INCR", key)
redis.call("PEXPIRE", key, ttl)
return 1
`)

var redisDecrementWaitCount = redis.NewScript(`
local key = KEYS[1]
local current = tonumber(redis.call("GET", key) or "0")
if current <= 1 then
  redis.call("DEL", key)
  return 0
end
return redis.call("DECR", key)
`)
