package ratelimit_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/ratelimit"
)

// newLimiter builds a Limiter against the test Redis, flushed to a clean state.
// These tests are not parallel: they share the fixed global key "rl:global".
func newLimiter(t *testing.T, global ratelimit.Limit) *ratelimit.Limiter {
	t.Helper()
	dsn := dbtest.RedisURL(t)
	dbtest.FlushRedis(t, dsn)
	limiter, err := ratelimit.New(dsn, global)
	require.NoError(t, err)
	t.Cleanup(func() { _ = limiter.Close() })
	return limiter
}

func TestAllowAdmitsUpToTheTenantLimit(t *testing.T) {
	limiter := newLimiter(t, ratelimit.Limit{Max: 1000, Window: time.Minute})
	ctx := context.Background()
	perTenant := ratelimit.Limit{Max: 3, Window: time.Minute}

	for i := range 3 {
		allowed, _, err := limiter.Allow(ctx, "tenant-a", perTenant)
		require.NoError(t, err)
		require.True(t, allowed, "send %d is within the limit", i+1)
	}

	allowed, retryAfter, err := limiter.Allow(ctx, "tenant-a", perTenant)
	require.NoError(t, err)
	require.False(t, allowed, "the 4th send exceeds the per-tenant limit")
	require.Positive(t, retryAfter, "a denied send reports a positive retry delay")
}

func TestAllowIsolatesTenants(t *testing.T) {
	limiter := newLimiter(t, ratelimit.Limit{Max: 1000, Window: time.Minute})
	ctx := context.Background()
	perTenant := ratelimit.Limit{Max: 2, Window: time.Minute}

	for range 2 {
		allowed, _, err := limiter.Allow(ctx, "tenant-a", perTenant)
		require.NoError(t, err)
		require.True(t, allowed)
	}
	denied, _, err := limiter.Allow(ctx, "tenant-a", perTenant)
	require.NoError(t, err)
	require.False(t, denied, "tenant-a is now at its limit")

	allowed, _, err := limiter.Allow(ctx, "tenant-b", perTenant)
	require.NoError(t, err)
	require.True(t, allowed, "tenant-b is unaffected by tenant-a's limit")
}

func TestAllowEnforcesGlobalCap(t *testing.T) {
	limiter := newLimiter(t, ratelimit.Limit{Max: 3, Window: time.Minute})
	ctx := context.Background()
	generous := ratelimit.Limit{Max: 1000, Window: time.Minute}

	// Three different tenants, each well under its own limit, still hit the
	// global cap of 3.
	allowed := 0
	for _, tenant := range []string{"t1", "t2", "t3", "t4", "t5"} {
		ok, _, err := limiter.Allow(ctx, tenant, generous)
		require.NoError(t, err)
		if ok {
			allowed++
		}
	}
	require.Equal(t, 3, allowed, "the global cap bounds total sends across tenants")
}

func TestAllowWindowSlides(t *testing.T) {
	limiter := newLimiter(t, ratelimit.Limit{Max: 1000, Window: time.Minute})
	ctx := context.Background()
	perTenant := ratelimit.Limit{Max: 2, Window: 300 * time.Millisecond}

	for range 2 {
		allowed, _, err := limiter.Allow(ctx, "tenant-a", perTenant)
		require.NoError(t, err)
		require.True(t, allowed)
	}
	denied, _, err := limiter.Allow(ctx, "tenant-a", perTenant)
	require.NoError(t, err)
	require.False(t, denied)

	time.Sleep(350 * time.Millisecond)

	allowed, _, err := limiter.Allow(ctx, "tenant-a", perTenant)
	require.NoError(t, err)
	require.True(t, allowed, "after the window passes the tenant may send again")
}

func TestAllowIsConsistentUnderConcurrency(t *testing.T) {
	limiter := newLimiter(t, ratelimit.Limit{Max: 10000, Window: time.Minute})
	ctx := context.Background()
	perTenant := ratelimit.Limit{Max: 20, Window: time.Minute}

	const goroutines = 100
	var wg sync.WaitGroup
	var mu sync.Mutex
	admitted := 0
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			ok, _, err := limiter.Allow(ctx, "tenant-c", perTenant)
			require.NoError(t, err)
			if ok {
				mu.Lock()
				admitted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	require.Equal(t, 20, admitted,
		"concurrent callers never exceed the limit — the Lua script is atomic")
}
