package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestUsageRecorderRecordsToCurrentPeriod(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	periodStart := time.Now().UTC().Truncate(time.Second)
	seedActiveSubscription(t, pool, tenantID, planID, periodStart, periodStart.AddDate(0, 1, 0))

	usage := adapters.NewUsage(pool)
	recorder := adapters.NewUsageRecorder(adapters.NewSubscriptions(pool), usage)

	require.NoError(t, recorder.Record(ctx, tenantID, campaigndomain.UsageCampaignSend,
		[]string{"recipient-1", "recipient-2"}))

	used, err := usage.CurrentUsage(ctx, tenantID, periodStart)
	require.NoError(t, err)
	require.Equal(t, int64(2), used)
}

func TestUsageRecorderIsIdempotent(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlan(t, pool, "published")

	periodStart := time.Now().UTC().Truncate(time.Second)
	seedActiveSubscription(t, pool, tenantID, planID, periodStart, periodStart.AddDate(0, 1, 0))

	usage := adapters.NewUsage(pool)
	recorder := adapters.NewUsageRecorder(adapters.NewSubscriptions(pool), usage)

	// The same send ref recorded twice — a redelivered batch — is a no-op.
	require.NoError(t, recorder.Record(ctx, tenantID, campaigndomain.UsageCampaignSend,
		[]string{"recipient-1"}))
	require.NoError(t, recorder.Record(ctx, tenantID, campaigndomain.UsageCampaignSend,
		[]string{"recipient-1"}))

	used, err := usage.CurrentUsage(ctx, tenantID, periodStart)
	require.NoError(t, err)
	require.Equal(t, int64(1), used)
}

func TestUsageRecorderSkipsTenantWithoutSubscription(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	usage := adapters.NewUsage(pool)
	recorder := adapters.NewUsageRecorder(adapters.NewSubscriptions(pool), usage)

	// A tenant with no subscription is not metered — and the call still succeeds.
	require.NoError(t, recorder.Record(ctx, tenantID, campaigndomain.UsageCampaignSend,
		[]string{"recipient-1"}))
}
