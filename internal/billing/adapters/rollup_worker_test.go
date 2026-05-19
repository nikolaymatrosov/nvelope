package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/billing/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// usageEvents builds campaign-send usage events for the given refs.
func usageEvents(tenantID string, periodStart time.Time, refs ...string) []*domain.UsageEvent {
	out := make([]*domain.UsageEvent, 0, len(refs))
	for _, ref := range refs {
		out = append(out, campaignSendEvent(tenantID, ref, periodStart))
	}
	return out
}

func TestUsageRollupIsIdempotent(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	// An active subscription so the rollup resolves the plan allowance/period.
	periodStart := time.Now().UTC().Truncate(time.Second)
	seedActiveSubscription(t, pool, tenantID, planID, periodStart, periodStart.AddDate(0, 1, 0))

	usage := adapters.NewUsage(pool)
	require.NoError(t, usage.RecordEvents(ctx, tenantID,
		usageEvents(tenantID, periodStart, "r1", "r2", "r3")))

	handler := command.NewRollupUsageHandler(usage,
		adapters.NewSubscriptions(pool), adapters.NewPlans(pool))

	require.NoError(t, handler.Handle(ctx, command.RollupUsage{TenantID: tenantID}))
	used, err := usage.CurrentUsage(ctx, tenantID, periodStart)
	require.NoError(t, err)
	require.Equal(t, int64(3), used)

	// A second rollup must not double-count the already-rolled events.
	require.NoError(t, handler.Handle(ctx, command.RollupUsage{TenantID: tenantID}))
	used, err = usage.CurrentUsage(ctx, tenantID, periodStart)
	require.NoError(t, err)
	require.Equal(t, int64(3), used)

	// New events in the same period accumulate on the next rollup.
	require.NoError(t, usage.RecordEvents(ctx, tenantID,
		usageEvents(tenantID, periodStart, "r4", "r5")))
	require.NoError(t, handler.Handle(ctx, command.RollupUsage{TenantID: tenantID}))
	used, err = usage.CurrentUsage(ctx, tenantID, periodStart)
	require.NoError(t, err)
	require.Equal(t, int64(5), used)
}
