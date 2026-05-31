package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// newResendThrottle builds a ResendThrottle against the test Redis, flushed to
// a clean state. These tests are not parallel: they share one Redis instance
// and assert on raw keys.
func newResendThrottle(t *testing.T, max int, window time.Duration) *adapters.ResendThrottle {
	t.Helper()
	dsn := dbtest.RedisURL(t)
	dbtest.FlushRedis(t, dsn)
	throttle, err := adapters.NewResendThrottle(dsn, max, window)
	require.NoError(t, err)
	t.Cleanup(func() { _ = throttle.Close() })
	return throttle
}

func TestResendThrottleAdmitsUpToTheLimit(t *testing.T) {
	throttle := newResendThrottle(t, 3, time.Minute)
	ctx := context.Background()

	for i := range 3 {
		allowed, err := throttle.Allow(ctx, "ada@example.com")
		require.NoError(t, err)
		require.True(t, allowed, "resend %d is within the limit", i+1)
	}

	allowed, err := throttle.Allow(ctx, "ada@example.com")
	require.NoError(t, err)
	require.False(t, allowed, "the 4th resend exceeds the limit")
}

func TestResendThrottleIsolatesKeys(t *testing.T) {
	throttle := newResendThrottle(t, 1, time.Minute)
	ctx := context.Background()

	allowed, err := throttle.Allow(ctx, "ada@example.com")
	require.NoError(t, err)
	require.True(t, allowed)
	denied, err := throttle.Allow(ctx, "ada@example.com")
	require.NoError(t, err)
	require.False(t, denied, "ada is now at her limit")

	other, err := throttle.Allow(ctx, "grace@example.com")
	require.NoError(t, err)
	require.True(t, other, "a different address has its own window")
}

// TestResendThrottleSetsExpiryOnTheWindowKey is the regression guard for the
// fixed-window counter: the very first Allow must leave the key with a TTL, so
// a counter can never be stranded without one and lock the address out forever.
func TestResendThrottleSetsExpiryOnTheWindowKey(t *testing.T) {
	dsn := dbtest.RedisURL(t)
	dbtest.FlushRedis(t, dsn)
	throttle, err := adapters.NewResendThrottle(dsn, 5, time.Minute)
	require.NoError(t, err)
	t.Cleanup(func() { _ = throttle.Close() })

	ctx := context.Background()
	_, err = throttle.Allow(ctx, "ada@example.com")
	require.NoError(t, err)

	opts, err := redis.ParseURL(dsn)
	require.NoError(t, err)
	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })

	ttl, err := client.PTTL(ctx, "verify-resend:throttle:ada@example.com").Result()
	require.NoError(t, err)
	require.Positive(t, ttl, "the counter key carries a TTL after the first increment")
	require.LessOrEqual(t, ttl, time.Minute, "the TTL never exceeds the window")
}

func TestResendThrottleWindowExpiryResetsTheCounter(t *testing.T) {
	throttle := newResendThrottle(t, 1, 50*time.Millisecond)
	ctx := context.Background()

	allowed, err := throttle.Allow(ctx, "ada@example.com")
	require.NoError(t, err)
	require.True(t, allowed)
	denied, err := throttle.Allow(ctx, "ada@example.com")
	require.NoError(t, err)
	require.False(t, denied, "the second resend is denied within the window")

	require.Eventually(t, func() bool {
		ok, err := throttle.Allow(ctx, "ada@example.com")
		return err == nil && ok
	}, time.Second, 20*time.Millisecond, "the window expires and the counter resets")
}
