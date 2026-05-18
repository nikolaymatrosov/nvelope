package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// recordEvent stages and records one delivery event of the given kind for the
// provider message id, attributing it within the tenant.
func recordEvent(t *testing.T, events *adapters.Events, tenantID, pm string, kind domain.EventKind) {
	t.Helper()
	ctx := context.Background()
	inboundID, _, err := events.StageInbound(ctx, newInbound(t, kind, pm))
	require.NoError(t, err)
	attr, ok, err := events.Attribute(ctx, tenantID, pm)
	require.NoError(t, err)
	require.True(t, ok)
	ev, err := domain.NewDeliveryEvent(tenantID, inboundID, kind, "rcpt@acme.com", pm, time.Now(), attr)
	require.NoError(t, err)
	_, err = events.RecordEvent(ctx, ev)
	require.NoError(t, err)
}

func TestAnalyticsRefreshComputesCounts(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	events := adapters.NewEvents(pool)
	analytics := adapters.NewAnalytics(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	pm := "pm-" + dbtest.RandString()
	campaignID, _ := seedCampaignRecipient(t, pool, tenantID, pm)

	recordEvent(t, events, tenantID, pm, domain.KindDelivery)
	recordEvent(t, events, tenantID, pm, domain.KindOpen)

	require.NoError(t, analytics.Refresh(ctx, tenantID))

	got, ok, err := analytics.GetCampaign(ctx, tenantID, campaignID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 1, got.Counts.Sent, "the seeded recipient is status 'sent'")
	require.Equal(t, 1, got.Counts.Delivered)
	require.Equal(t, 1, got.Counts.Opened)
	require.Equal(t, 0, got.Counts.Bounced)

	// Refresh is idempotent — a re-run produces the same counts.
	require.NoError(t, analytics.Refresh(ctx, tenantID))
	again, _, err := analytics.GetCampaign(ctx, tenantID, campaignID)
	require.NoError(t, err)
	require.Equal(t, got.Counts, again.Counts)
}

func TestAnalyticsGetCampaignMissingRow(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	analytics := adapters.NewAnalytics(pool)
	tenantID := seedTenant(t, pool)

	_, ok, err := analytics.GetCampaign(context.Background(), tenantID, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestAnalyticsDashboardAndCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	events := adapters.NewEvents(pool)
	analytics := adapters.NewAnalytics(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)

	pm := "pm-" + dbtest.RandString()
	seedCampaignRecipient(t, pool, tenantA, pm)
	recordEvent(t, events, tenantA, pm, domain.KindBounce)
	require.NoError(t, analytics.Refresh(ctx, tenantA))

	dashA, err := analytics.GetDashboard(ctx, tenantA)
	require.NoError(t, err)
	require.Equal(t, 1, dashA.Totals.Sent)
	require.Equal(t, 1, dashA.Totals.Bounced)
	require.Len(t, dashA.Recent, 1)

	// Tenant B sees none of tenant A's analytics.
	dashB, err := analytics.GetDashboard(ctx, tenantB)
	require.NoError(t, err)
	require.Zero(t, dashB.Totals.Sent)
	require.Empty(t, dashB.Recent)
}
