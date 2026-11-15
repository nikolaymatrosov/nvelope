package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// recordEvent stages and records one delivery event of the given kind for the
// provider message id, attributing it within the tenant. email is the
// recipient the event names.
func recordEvent(t *testing.T, b backend, tenantID, pm string, kind domain.EventKind, email string) {
	t.Helper()
	ctx := context.Background()
	events := b.events(t)
	n, err := domain.NewInboundNotification("dk-"+dbtest.RandString(), kind, pm, email,
		time.Now(), []byte(`{"k":"v"}`))
	require.NoError(t, err)
	inboundID := stageInbound(t, events, n)
	attr, ok, err := events.Attribute(ctx, tenantID, pm)
	require.NoError(t, err)
	require.True(t, ok)
	ev, err := domain.NewDeliveryEvent(tenantID, inboundID, kind, email, pm, time.Now(), attr)
	require.NoError(t, err)
	recorded, err := events.RecordEvent(ctx, ev)
	require.NoError(t, err)
	require.True(t, recorded)
}

func TestAnalyticsRefreshComputesCounts(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		analytics := b.analytics(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		campaignID, _ := seedSend(t, b, tenantID, pm)
		// A failed recipient is not a sent one.
		b.newRecipient(t, tenantID, campaignID, "failed", "pm-"+dbtest.RandString())

		// Counts are distinct recipients per kind: a repeated delivery to one
		// address counts once, and a second address counts again.
		recordEvent(t, b, tenantID, pm, domain.KindDelivery, "a@acme.com")
		recordEvent(t, b, tenantID, pm, domain.KindDelivery, "a@acme.com")
		recordEvent(t, b, tenantID, pm, domain.KindDelivery, "b@acme.com")
		recordEvent(t, b, tenantID, pm, domain.KindOpen, "a@acme.com")
		recordEvent(t, b, tenantID, pm, domain.KindClick, "a@acme.com")
		recordEvent(t, b, tenantID, pm, domain.KindBounce, "b@acme.com")
		recordEvent(t, b, tenantID, pm, domain.KindComplaint, "b@acme.com")

		require.NoError(t, analytics.Refresh(ctx, tenantID))

		got, ok, err := analytics.GetCampaign(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, domain.Counts{
			Sent: 1, Delivered: 2, Opened: 1, Clicked: 1, Bounced: 1, Complained: 1,
		}, got.Counts)
		require.False(t, got.RefreshedAt.IsZero())

		// Refresh is idempotent — a re-run produces the same counts.
		require.NoError(t, analytics.Refresh(ctx, tenantID))
		again, _, err := analytics.GetCampaign(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Equal(t, got.Counts, again.Counts)
	})
}

func TestAnalyticsRefreshSkipsDraftsAndZeroFillsQuietCampaigns(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		analytics := b.analytics(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		draft := b.newCampaign(t, tenantID, "draft")
		quiet := b.newCampaign(t, tenantID, "finished")

		require.NoError(t, analytics.Refresh(ctx, tenantID))

		_, ok, err := analytics.GetCampaign(ctx, tenantID, draft)
		require.NoError(t, err)
		require.False(t, ok, "a draft has no analytics row")

		got, ok, err := analytics.GetCampaign(ctx, tenantID, quiet)
		require.NoError(t, err)
		require.True(t, ok, "a campaign with no recipient or event still gets a row")
		require.Equal(t, domain.Counts{}, got.Counts)
	})
}

func TestAnalyticsGetCampaignMissingRow(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		analytics := b.analytics(t)
		tenantID := b.newTenant(t)

		_, ok, err := analytics.GetCampaign(context.Background(), tenantID, "00000000-0000-0000-0000-000000000000")
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestAnalyticsDashboardAndCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		analytics := b.analytics(t)
		ctx := context.Background()
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)

		pm := "pm-" + dbtest.RandString()
		seedSend(t, b, tenantA, pm)
		recordEvent(t, b, tenantA, pm, domain.KindBounce, "rcpt@acme.com")
		require.NoError(t, analytics.Refresh(ctx, tenantA))
		require.NoError(t, analytics.Refresh(ctx, tenantB))

		dashA, err := analytics.GetDashboard(ctx, tenantA)
		require.NoError(t, err)
		require.Equal(t, 1, dashA.Totals.Sent)
		require.Equal(t, 1, dashA.Totals.Bounced)
		require.Len(t, dashA.Recent, 1)
		require.NotEmpty(t, dashA.Recent[0].Name)
		require.Equal(t, 1, dashA.Recent[0].Counts.Sent)

		// Tenant B sees none of tenant A's analytics.
		dashB, err := analytics.GetDashboard(ctx, tenantB)
		require.NoError(t, err)
		require.Zero(t, dashB.Totals.Sent)
		require.Empty(t, dashB.Recent)
	})
}

func TestAnalyticsDashboardListsTheTenLatestCampaignsAndTotalsAll(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		analytics := b.analytics(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		var seeded []string
		for range 12 {
			campaignID, _ := seedSend(t, b, tenantID, "pm-"+dbtest.RandString())
			seeded = append(seeded, campaignID)
			time.Sleep(2 * time.Millisecond) // distinct created_at per campaign
		}
		require.NoError(t, analytics.Refresh(ctx, tenantID))

		dash, err := analytics.GetDashboard(ctx, tenantID)
		require.NoError(t, err)
		require.Equal(t, 12, dash.Totals.Sent, "totals cover every campaign")
		require.Len(t, dash.Recent, 10, "the recent list is capped")
		var got []string
		for _, r := range dash.Recent {
			got = append(got, r.CampaignID)
		}
		want := make([]string, 0, 10)
		for i := len(seeded) - 1; i >= 2; i-- {
			want = append(want, seeded[i])
		}
		require.Equal(t, want, got, "the ten latest campaigns, newest first")
	})
}
