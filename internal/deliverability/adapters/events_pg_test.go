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

func TestEventsStageInboundIsIdempotent(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewEvents(pool)
	ctx := context.Background()

	n := newInbound(t, domain.KindBounce, "pm-"+dbtest.RandString())

	id1, staged1, err := repo.StageInbound(ctx, n)
	require.NoError(t, err)
	require.True(t, staged1)
	require.NotEmpty(t, id1)

	id2, staged2, err := repo.StageInbound(ctx, n)
	require.NoError(t, err)
	require.False(t, staged2, "a duplicate dedupe key inserts nothing")
	require.Equal(t, id1, id2)
}

func TestEventsLoadInbound(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewEvents(pool)
	ctx := context.Background()

	n := newInbound(t, domain.KindComplaint, "pm-"+dbtest.RandString())
	id, _, err := repo.StageInbound(ctx, n)
	require.NoError(t, err)

	loaded, err := repo.LoadInbound(ctx, id)
	require.NoError(t, err)
	require.Equal(t, id, loaded.ID)
	require.Equal(t, domain.KindComplaint, loaded.Kind)
	require.Equal(t, domain.InboundPending, loaded.Status)
	require.Equal(t, n.ProviderMessageID, loaded.ProviderMessageID)
}

func TestEventsTenantForMessageResolvesCampaignSend(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewEvents(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	pm := "pm-" + dbtest.RandString()
	seedCampaignRecipient(t, pool, tenantID, pm)

	resolved, ok, err := repo.TenantForMessage(ctx, pm)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, tenantID, resolved)

	_, ok, err = repo.TenantForMessage(ctx, "pm-no-such-message")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestEventsAttributeMatchesCampaignAndTransactional(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewEvents(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	campaignPM := "pm-" + dbtest.RandString()
	campaignID, recipientID := seedCampaignRecipient(t, pool, tenantID, campaignPM)
	attr, ok, err := repo.Attribute(ctx, tenantID, campaignPM)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, campaignID, attr.CampaignID)
	require.Equal(t, recipientID, attr.CampaignRecipientID)

	txPM := "pm-" + dbtest.RandString()
	txID := seedTransactionalMessage(t, pool, tenantID, txPM)
	attr, ok, err = repo.Attribute(ctx, tenantID, txPM)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, txID, attr.TransactionalMessageID)
	require.Empty(t, attr.CampaignRecipientID)

	_, ok, err = repo.Attribute(ctx, tenantID, "pm-unmatched")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestEventsRecordEventIsIdempotent(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewEvents(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	pm := "pm-" + dbtest.RandString()
	campaignID, recipientID := seedCampaignRecipient(t, pool, tenantID, pm)

	inboundID, _, err := repo.StageInbound(ctx, newInbound(t, domain.KindBounce, pm))
	require.NoError(t, err)

	event, err := domain.NewDeliveryEvent(tenantID, inboundID, domain.KindBounce,
		"rcpt@acme.com", pm, time.Now(),
		domain.Attribution{CampaignID: campaignID, CampaignRecipientID: recipientID})
	require.NoError(t, err)

	recorded, err := repo.RecordEvent(ctx, event)
	require.NoError(t, err)
	require.True(t, recorded)

	recorded, err = repo.RecordEvent(ctx, event)
	require.NoError(t, err)
	require.False(t, recorded, "a second record for the same inbound event is a no-op")
}

func TestEventsDeliveryEventsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewEvents(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)
	pm := "pm-" + dbtest.RandString()
	campaignID, recipientID := seedCampaignRecipient(t, pool, tenantA, pm)

	inboundID, _, err := repo.StageInbound(ctx, newInbound(t, domain.KindBounce, pm))
	require.NoError(t, err)

	event, err := domain.NewDeliveryEvent(tenantA, inboundID, domain.KindBounce,
		"rcpt@acme.com", pm, time.Now(),
		domain.Attribution{CampaignID: campaignID, CampaignRecipientID: recipientID})
	require.NoError(t, err)
	recorded, err := repo.RecordEvent(ctx, event)
	require.NoError(t, err)
	require.True(t, recorded)

	// Tenant A sees its event; tenant B, bound to its own RLS context, sees
	// none of tenant A's delivery events.
	var countA, countB int
	require.NoError(t, withTenantQuery(t, pool, tenantA,
		"SELECT count(*) FROM delivery_events", &countA))
	require.NoError(t, withTenantQuery(t, pool, tenantB,
		"SELECT count(*) FROM delivery_events", &countB))
	require.Equal(t, 1, countA)
	require.Equal(t, 0, countB)
}

func TestEventsMarkInbound(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewEvents(pool)
	ctx := context.Background()

	id, _, err := repo.StageInbound(ctx, newInbound(t, domain.KindBounce, "pm-"+dbtest.RandString()))
	require.NoError(t, err)

	require.NoError(t, repo.MarkInbound(ctx, id, domain.InboundUnattributed))
	loaded, err := repo.LoadInbound(ctx, id)
	require.NoError(t, err)
	require.Equal(t, domain.InboundUnattributed, loaded.Status)
	require.True(t, loaded.IsProcessed())
}
