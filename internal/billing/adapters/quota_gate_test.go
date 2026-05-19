package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestQuotaGateAllowsWithinAllowance(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlanWith(t, pool, "published", "block", 10)
	seedSubscriptionState(t, pool, tenantID, planID, "active")

	gate := adapters.NewQuotaGate(adapters.NewSubscriptions(pool),
		adapters.NewPlans(pool), adapters.NewUsage(pool))

	decision, err := gate.Authorize(ctx, tenantID, campaigndomain.UsageCampaignSend, 10)
	require.NoError(t, err)
	require.True(t, decision.Allowed)
}

func TestQuotaGateBlocksOverAllowance(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlanWith(t, pool, "published", "block", 10)
	seedSubscriptionState(t, pool, tenantID, planID, "active")

	gate := adapters.NewQuotaGate(adapters.NewSubscriptions(pool),
		adapters.NewPlans(pool), adapters.NewUsage(pool))

	decision, err := gate.Authorize(ctx, tenantID, campaigndomain.UsageCampaignSend, 11)
	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.Equal(t, campaigndomain.QuotaReasonExceeded, decision.Reason)
}

func TestQuotaGateMeterModeAllowsOverAllowance(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlanWith(t, pool, "published", "meter", 10)
	seedSubscriptionState(t, pool, tenantID, planID, "active")

	gate := adapters.NewQuotaGate(adapters.NewSubscriptions(pool),
		adapters.NewPlans(pool), adapters.NewUsage(pool))

	// A meter-mode plan proceeds past its allowance — the excess is overage.
	decision, err := gate.Authorize(ctx, tenantID, campaigndomain.UsageCampaignSend, 9999)
	require.NoError(t, err)
	require.True(t, decision.Allowed)
}

func TestQuotaGateSuspendedSubscriptionBlocks(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	planID := seedPlanWith(t, pool, "published", "block", 10)
	seedSubscriptionState(t, pool, tenantID, planID, "suspended")

	gate := adapters.NewQuotaGate(adapters.NewSubscriptions(pool),
		adapters.NewPlans(pool), adapters.NewUsage(pool))

	decision, err := gate.Authorize(ctx, tenantID, campaigndomain.UsageCampaignSend, 1)
	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.Equal(t, campaigndomain.QuotaReasonSuspended, decision.Reason)
}

func TestQuotaGateNoSubscriptionBlocks(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	gate := adapters.NewQuotaGate(adapters.NewSubscriptions(pool),
		adapters.NewPlans(pool), adapters.NewUsage(pool))

	decision, err := gate.Authorize(ctx, tenantID, campaigndomain.UsageCampaignSend, 1)
	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.Equal(t, campaigndomain.QuotaReasonExceeded, decision.Reason)
}
