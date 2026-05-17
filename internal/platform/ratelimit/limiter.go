// Package ratelimit is the shared, Redis-coordinated sliding-window rate
// limiter. Counters live in Redis so the limit holds across every worker pod,
// not just within one process. It enforces a per-tenant cap and a platform-wide
// global cap together, atomically, so a send is admitted only when both allow
// it.
//
// The package is concrete infrastructure: it imports no domain package. The
// campaign context wraps it to satisfy its own domain-owned RateLimiter
// interface.
package ratelimit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limit is a sliding-window allowance: at most Max events per Window.
type Limit struct {
	Max    int
	Window time.Duration
}

// slidingWindow checks the per-tenant key (KEYS[1]) and the global key
// (KEYS[2]) together. It admits the call only when BOTH windows have room, and
// only then records the event in both — so a partial admission can never
// over-count one key while the other is full.
//
// ARGV: now(ms), member, tenantWindow(ms), tenantLimit, globalWindow(ms),
// globalLimit. Returns {allowed, retryAfter(ms)}.
var slidingWindow = redis.NewScript(`
local now      = tonumber(ARGV[1])
local member   = ARGV[2]
local twindow  = tonumber(ARGV[3])
local tlimit   = tonumber(ARGV[4])
local gwindow  = tonumber(ARGV[5])
local glimit   = tonumber(ARGV[6])

redis.call('ZREMRANGEBYSCORE', KEYS[1], 0, now - twindow)
redis.call('ZREMRANGEBYSCORE', KEYS[2], 0, now - gwindow)

local tcount = redis.call('ZCARD', KEYS[1])
local gcount = redis.call('ZCARD', KEYS[2])

if tcount < tlimit and gcount < glimit then
    redis.call('ZADD', KEYS[1], now, member)
    redis.call('PEXPIRE', KEYS[1], twindow)
    redis.call('ZADD', KEYS[2], now, member)
    redis.call('PEXPIRE', KEYS[2], gwindow)
    return {1, 0}
end

local retry = 0
if tcount >= tlimit then
    local oldest = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
    if oldest[2] then
        local wait = (tonumber(oldest[2]) + twindow) - now
        if wait > retry then retry = wait end
    end
end
if gcount >= glimit then
    local oldest = redis.call('ZRANGE', KEYS[2], 0, 0, 'WITHSCORES')
    if oldest[2] then
        local wait = (tonumber(oldest[2]) + gwindow) - now
        if wait > retry then retry = wait end
    end
end
if retry < 1 then retry = 1 end
return {0, retry}
`)

// Limiter enforces per-tenant and global sliding-window send rates against
// Redis. It is safe for concurrent use.
type Limiter struct {
	client *redis.Client
	global Limit
}

// New builds a Limiter connected to the Redis instance at redisURL, with the
// platform-wide global cap. It fails fast on an unusable DSN or unreachable
// Redis.
func New(redisURL string, global Limit) (*Limiter, error) {
	if global.Max <= 0 || global.Window <= 0 {
		return nil, fmt.Errorf("ratelimit: global limit must be positive")
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("ratelimit: invalid redis url: %w", err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ratelimit: redis unreachable: %w", err)
	}
	return &Limiter{client: client, global: global}, nil
}

// Close releases the Redis connection pool.
func (l *Limiter) Close() error { return l.client.Close() }

// Allow reports whether one send for tenantID may proceed now, given the
// tenant's per-tenant limit and the limiter's global cap. When the send is
// denied, retryAfter is the delay before another attempt could succeed.
func (l *Limiter) Allow(ctx context.Context, tenantID string, perTenant Limit) (allowed bool, retryAfter time.Duration, err error) {
	if perTenant.Max <= 0 || perTenant.Window <= 0 {
		return false, 0, fmt.Errorf("ratelimit: per-tenant limit must be positive")
	}
	member, err := uniqueMember()
	if err != nil {
		return false, 0, err
	}
	now := time.Now().UnixMilli()
	keys := []string{"rl:tenant:" + tenantID, "rl:global"}
	args := []any{
		now, member,
		perTenant.Window.Milliseconds(), perTenant.Max,
		l.global.Window.Milliseconds(), l.global.Max,
	}
	res, err := slidingWindow.Run(ctx, l.client, keys, args...).Result()
	if err != nil {
		return false, 0, fmt.Errorf("ratelimit: redis eval failed: %w", err)
	}
	values, ok := res.([]any)
	if !ok || len(values) != 2 {
		return false, 0, fmt.Errorf("ratelimit: unexpected script result %v", res)
	}
	allowedFlag, _ := values[0].(int64)
	retryMs, _ := values[1].(int64)
	if allowedFlag == 1 {
		return true, 0, nil
	}
	return false, time.Duration(retryMs) * time.Millisecond, nil
}

// uniqueMember returns a value unique per call, so each admitted event is a
// distinct sorted-set entry rather than being deduplicated by score.
func uniqueMember() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("ratelimit: generating member: %w", err)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + hex.EncodeToString(b), nil
}
