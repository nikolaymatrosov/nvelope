package adapters_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

func TestEventsStageInboundIsIdempotent(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()

		n := newInbound(t, domain.KindBounce, "pm-"+dbtest.RandString())

		id1 := stageInbound(t, repo, n)
		require.NotEmpty(t, id1)

		id2, staged2, err := repo.StageInbound(ctx, n)
		require.NoError(t, err)
		require.False(t, staged2, "a duplicate dedupe key inserts nothing")
		require.Equal(t, id1, id2)
	})
}

func TestEventsStageInboundIsRaceFree(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		n := newInbound(t, domain.KindBounce, "pm-"+dbtest.RandString())

		var staged atomic.Int32
		ids := make([]string, 6)
		var wg sync.WaitGroup
		for i := range ids {
			wg.Go(func() {
				id, ok, err := repo.StageInbound(context.Background(), n)
				if err == nil && ok {
					staged.Add(1)
				}
				if err == nil {
					ids[i] = id
				}
			})
		}
		wg.Wait()

		require.EqualValues(t, 1, staged.Load(), "exactly one of the concurrent stagings writes the row")
		for _, id := range ids {
			require.Equal(t, ids[0], id)
			require.NotEmpty(t, id)
		}
		// Retire the row, as stageInbound does for the other tests.
		require.NoError(t, repo.MarkInbound(context.Background(), ids[0], domain.InboundFailed))
	})
}

func TestEventsLoadInbound(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()

		n := newInbound(t, domain.KindComplaint, "pm-"+dbtest.RandString())
		id := stageInbound(t, repo, n)

		loaded, err := repo.LoadInbound(ctx, id)
		require.NoError(t, err)
		require.Equal(t, id, loaded.ID)
		require.Equal(t, n.DedupeKey, loaded.DedupeKey)
		require.Equal(t, domain.KindComplaint, loaded.Kind)
		require.Equal(t, domain.InboundPending, loaded.Status)
		require.Equal(t, n.ProviderMessageID, loaded.ProviderMessageID)
		require.Equal(t, n.RecipientEmail, loaded.RecipientEmail)
		require.JSONEq(t, string(n.RawPayload), string(loaded.RawPayload))
		require.WithinDuration(t, n.OccurredAt, loaded.OccurredAt, time.Millisecond)

		_, err = repo.LoadInbound(ctx, "00000000-0000-0000-0000-000000000000")
		require.Error(t, err, "an unknown id is an error")
	})
}

func TestEventsTenantForMessageResolvesCampaignSend(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		seedSend(t, b, tenantID, pm)

		resolved, ok, err := repo.TenantForMessage(ctx, pm)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, tenantID, resolved)

		txPM := "pm-" + dbtest.RandString()
		b.newTransactional(t, tenantID, txPM)
		resolved, ok, err = repo.TenantForMessage(ctx, txPM)
		require.NoError(t, err)
		require.True(t, ok, "a transactional send resolves too")
		require.Equal(t, tenantID, resolved)

		_, ok, err = repo.TenantForMessage(ctx, "pm-no-such-message")
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestEventsAttributeMatchesCampaignAndTransactional(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		campaignPM := "pm-" + dbtest.RandString()
		campaignID, recipientID := seedSend(t, b, tenantID, campaignPM)
		attr, ok, err := repo.Attribute(ctx, tenantID, campaignPM)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, campaignID, attr.CampaignID)
		require.Equal(t, recipientID, attr.CampaignRecipientID)

		txPM := "pm-" + dbtest.RandString()
		txID := b.newTransactional(t, tenantID, txPM)
		attr, ok, err = repo.Attribute(ctx, tenantID, txPM)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, txID, attr.TransactionalMessageID)
		require.Empty(t, attr.CampaignRecipientID)

		_, ok, err = repo.Attribute(ctx, tenantID, "pm-unmatched")
		require.NoError(t, err)
		require.False(t, ok)

		// Another tenant cannot attribute this tenant's send.
		_, ok, err = repo.Attribute(ctx, b.newTenant(t), campaignPM)
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestEventsRecordEventIsIdempotent(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		campaignID, recipientID := seedSend(t, b, tenantID, pm)

		inboundID := stageInbound(t, repo, newInbound(t, domain.KindBounce, pm))

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
		require.Equal(t, 1, b.eventCount(t, tenantID))
	})
}

func TestEventsRecordEventIsRaceFree(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		seedSend(t, b, tenantID, pm)
		inboundID := stageInbound(t, repo, newInbound(t, domain.KindOpen, pm))
		attr, ok, err := repo.Attribute(context.Background(), tenantID, pm)
		require.NoError(t, err)
		require.True(t, ok)
		event, err := domain.NewDeliveryEvent(tenantID, inboundID, domain.KindOpen,
			"rcpt@acme.com", pm, time.Now(), attr)
		require.NoError(t, err)

		var recorded atomic.Int32
		var failed atomic.Int32
		var wg sync.WaitGroup
		for range 6 {
			wg.Go(func() {
				ok, err := repo.RecordEvent(context.Background(), event)
				switch {
				case err != nil:
					failed.Add(1)
				case ok:
					recorded.Add(1)
				}
			})
		}
		wg.Wait()

		require.Zero(t, failed.Load())
		require.EqualValues(t, 1, recorded.Load())
		require.Equal(t, 1, b.eventCount(t, tenantID))
	})
}

func TestEventsDeliveryEventsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		campaignID, recipientID := seedSend(t, b, tenantA, pm)

		inboundID := stageInbound(t, repo, newInbound(t, domain.KindBounce, pm))

		event, err := domain.NewDeliveryEvent(tenantA, inboundID, domain.KindBounce,
			"rcpt@acme.com", pm, time.Now(),
			domain.Attribution{CampaignID: campaignID, CampaignRecipientID: recipientID})
		require.NoError(t, err)
		recorded, err := repo.RecordEvent(ctx, event)
		require.NoError(t, err)
		require.True(t, recorded)

		require.Equal(t, 1, b.eventCount(t, tenantA))
		require.Equal(t, 0, b.eventCount(t, tenantB))
	})
}

func TestEventsMarkInbound(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()

		id := stageInbound(t, repo, newInbound(t, domain.KindBounce, "pm-"+dbtest.RandString()))

		require.NoError(t, repo.MarkInbound(ctx, id, domain.InboundUnattributed))
		loaded, err := repo.LoadInbound(ctx, id)
		require.NoError(t, err)
		require.Equal(t, domain.InboundUnattributed, loaded.Status)
		require.True(t, loaded.IsProcessed())
	})
}

func TestEventsPendingInboundListsOnlyStalePendingNotifications(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.events(t)
		ctx := context.Background()

		pending := stageInbound(t, repo, newInbound(t, domain.KindBounce, "pm-"+dbtest.RandString()))
		done := stageInbound(t, repo, newInbound(t, domain.KindBounce, "pm-"+dbtest.RandString()))
		require.NoError(t, repo.MarkInbound(ctx, done, domain.InboundAttributed))

		ids, err := repo.PendingInbound(ctx, time.Now().Add(time.Minute), 5000)
		require.NoError(t, err)
		require.Contains(t, ids, pending)
		require.NotContains(t, ids, done, "a processed notification is not pending")

		ids, err = repo.PendingInbound(ctx, time.Now().Add(-time.Hour), 5000)
		require.NoError(t, err)
		require.NotContains(t, ids, pending, "a notification newer than the cutoff is not stale")

		ids, err = repo.PendingInbound(ctx, time.Now().Add(time.Minute), 1)
		require.NoError(t, err)
		require.Len(t, ids, 1, "the limit bounds the page")
	})
}
