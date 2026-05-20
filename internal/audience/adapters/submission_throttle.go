package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// SubmissionThrottle is a Redis-backed fixed-window implementation of
// domain.SubmissionThrottle. Counters live in Redis so the limit holds across
// every API pod. It is kept separate from the send rate limiter so a flood of
// public form submissions cannot consume the platform's send budget.
type SubmissionThrottle struct {
	client *redis.Client
	max    int
	window time.Duration
}

var _ domain.SubmissionThrottle = (*SubmissionThrottle)(nil)

// NewSubmissionThrottle builds a throttle admitting at most max submissions per
// key per window. It fails fast on an unusable Redis DSN.
func NewSubmissionThrottle(redisURL string, max int, window time.Duration) (*SubmissionThrottle, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("submission throttle: invalid redis url: %w", err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("submission throttle: redis unreachable: %w", err)
	}
	return &SubmissionThrottle{client: client, max: max, window: window}, nil
}

// Close releases the Redis connection pool.
func (t *SubmissionThrottle) Close() error { return t.client.Close() }

// Allow reports whether a submission for key may proceed now. It increments a
// fixed-window counter and admits the call while the count is within max.
func (t *SubmissionThrottle) Allow(ctx context.Context, key string) (bool, error) {
	redisKey := "optin:throttle:" + key
	count, err := t.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return false, fmt.Errorf("submission throttle: redis incr failed: %w", err)
	}
	if count == 1 {
		if err := t.client.Expire(ctx, redisKey, t.window).Err(); err != nil {
			return false, fmt.Errorf("submission throttle: redis expire failed: %w", err)
		}
	}
	return count <= int64(t.max), nil
}
