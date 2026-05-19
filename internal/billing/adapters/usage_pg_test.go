package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// campaignSendEvent builds a campaign-send usage event for tests.
func campaignSendEvent(tenantID, ref string, periodStart time.Time) *domain.UsageEvent {
	return domain.NewUsageEvent(tenantID, domain.UsageCampaignSend, ref, periodStart, time.Now())
}

func TestUsageRecordAndCurrentUsage(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewUsage(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	period := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, repo.RecordEvents(ctx, tenantID, []*domain.UsageEvent{
		campaignSendEvent(tenantID, "r1", period),
		campaignSendEvent(tenantID, "r2", period),
	}))

	used, err := repo.CurrentUsage(ctx, tenantID, period)
	require.NoError(t, err)
	require.Equal(t, int64(2), used, "the un-rolled tail counts toward current usage")
}

func TestUsageRecordIsIdempotent(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewUsage(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	period := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, repo.RecordEvents(ctx, tenantID,
		[]*domain.UsageEvent{campaignSendEvent(tenantID, "r1", period)}))
	// Re-recording the same source_ref is a no-op.
	require.NoError(t, repo.RecordEvents(ctx, tenantID,
		[]*domain.UsageEvent{campaignSendEvent(tenantID, "r1", period)}))

	used, err := repo.CurrentUsage(ctx, tenantID, period)
	require.NoError(t, err)
	require.Equal(t, int64(1), used)
}

func TestUsageRollupAggregatesIntoCounter(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewUsage(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	period := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, repo.RecordEvents(ctx, tenantID, []*domain.UsageEvent{
		campaignSendEvent(tenantID, "r1", period),
		campaignSendEvent(tenantID, "r2", period),
		campaignSendEvent(tenantID, "r3", period),
	}))
	require.NoError(t, repo.Rollup(ctx, tenantID, 50000, domain.NewBillingPeriod(1, 0)))

	// After the rollup the counter carries the total; current usage is unchanged.
	used, err := repo.CurrentUsage(ctx, tenantID, period)
	require.NoError(t, err)
	require.Equal(t, int64(3), used)
}

func TestUsageCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewUsage(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)
	period := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, repo.RecordEvents(ctx, tenantA,
		[]*domain.UsageEvent{campaignSendEvent(tenantA, "r1", period)}))

	usedB, err := repo.CurrentUsage(ctx, tenantB, period)
	require.NoError(t, err)
	require.Equal(t, int64(0), usedB, "tenant B sees none of tenant A's usage")
}
