package adapters_test

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/ydbthrottle"
)

func TestYDBRateLimiterGlobalWindowSpansTenants(t *testing.T) {
	t.Parallel()
	// A run-unique window size gives this test its own "rl:global" row: rows
	// are keyed by (throttle_key, window_start), and window_start is now
	// truncated to the size.
	size := time.Hour + time.Duration(rand.IntN(1_000_000))*time.Millisecond
	limiter := adapters.NewYDBRateLimiter(ydbthrottle.New(dbtest.YDBMigrated(t)),
		domain.Limit{Max: 2, Window: size})
	perTenant := domain.Limit{Max: 10, Window: size}
	ctx := context.Background()
	tenantA, tenantB := uuid.NewString(), uuid.NewString()

	ok, _, err := limiter.Allow(ctx, tenantA, perTenant)
	require.NoError(t, err)
	require.True(t, ok)
	ok, _, err = limiter.Allow(ctx, tenantB, perTenant)
	require.NoError(t, err)
	require.True(t, ok)

	ok, retryAfter, err := limiter.Allow(ctx, tenantB, perTenant)
	require.NoError(t, err)
	require.False(t, ok, "the global window is full even though tenant B's own window has room")
	require.Positive(t, retryAfter)
	require.LessOrEqual(t, retryAfter, size)
}
