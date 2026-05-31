package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

// resendWindow increments a fixed-window counter (KEYS[1]) and, on the first
// increment, sets its expiry. INCR and PEXPIRE run as one atomic unit, so the
// counter can never be stranded without a TTL — which would otherwise lock the
// key out permanently after a mid-operation failure. ARGV[1] is the window in
// milliseconds; the script returns the post-increment count.
var resendWindow = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`)

// ResendThrottle is a Redis-backed fixed-window implementation of
// domain.ResendThrottle. Counters live in Redis so the limit holds across every
// API pod. It is kept separate from the send rate limiter so a flood of resend
// requests cannot consume the platform's send budget.
type ResendThrottle struct {
	client *redis.Client
	max    int
	window time.Duration
}

var _ domain.ResendThrottle = (*ResendThrottle)(nil)

// NewResendThrottle builds a throttle admitting at most max resends per key per
// window. It fails fast on an unusable Redis DSN.
func NewResendThrottle(redisURL string, max int, window time.Duration) (*ResendThrottle, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("resend throttle: invalid redis url: %w", err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("resend throttle: redis unreachable: %w", err)
	}
	return &ResendThrottle{client: client, max: max, window: window}, nil
}

// Close releases the Redis connection pool.
func (t *ResendThrottle) Close() error { return t.client.Close() }

// Allow reports whether a verification-email resend for key may proceed now. It
// increments a fixed-window counter and admits the call while the count is
// within max.
func (t *ResendThrottle) Allow(ctx context.Context, key string) (bool, error) {
	redisKey := "verify-resend:throttle:" + key
	res, err := resendWindow.Run(ctx, t.client, []string{redisKey}, t.window.Milliseconds()).Result()
	if err != nil {
		return false, fmt.Errorf("resend throttle: redis eval failed: %w", err)
	}
	count, ok := res.(int64)
	if !ok {
		return false, fmt.Errorf("resend throttle: unexpected script result %v", res)
	}
	return count <= int64(t.max), nil
}
