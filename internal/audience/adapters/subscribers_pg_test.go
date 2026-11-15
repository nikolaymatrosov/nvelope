package adapters_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// TestSubscribersDeleteKeepsDeliveryEvents covers the Postgres cascade from
// subscribers through campaign_recipients into delivery_events: the
// subscriber's recipient rows go, and their delivery events stay with
// campaign_recipient_id cleared. YDB has no foreign keys; its delete removes
// the recipient rows and leaves the event's id dangling.
func TestSubscribersDeleteKeepsDeliveryEvents(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	subscribers := adapters.NewSubscribers(pool)
	subscriberID := addSubscriber(t, subscribers, tenantID, "gone@acme.com")
	pm := "pm-" + dbtest.RandString()

	var inboundID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO inbound_feedback_events
		   (dedupe_key, event_kind, provider_message_id, recipient_email, occurred_at, raw_payload)
		 VALUES ($1, 'bounce', $2, 'gone@acme.com', now(), '{}') RETURNING id`,
		"dk-"+dbtest.RandString(), pm).Scan(&inboundID))

	var eventID string
	require.NoError(t, tenantdb.WithTenant(ctx, pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var campaignID, recipientID string
		if err := tx.QueryRow(ctx,
			`INSERT INTO campaigns (tenant_id, name, subject, body_html, from_name, from_local_part, status)
			 VALUES ($1, 'C', 'Subject', '<p>hi</p>', 'Acme', 'news', 'finished') RETURNING id`,
			tenantID).Scan(&campaignID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			`INSERT INTO campaign_recipients (tenant_id, campaign_id, subscriber_id, email, status)
			 VALUES ($1, $2, $3, 'gone@acme.com', 'sent') RETURNING id`,
			tenantID, campaignID, subscriberID).Scan(&recipientID); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`INSERT INTO delivery_events
			   (tenant_id, inbound_event_id, event_kind, recipient_email,
			    campaign_id, campaign_recipient_id, provider_message_id, occurred_at)
			 VALUES (@tenant_id, @inbound_event_id, 'bounce', 'gone@acme.com',
			         @campaign_id, @campaign_recipient_id, @provider_message_id, now())
			 RETURNING id`,
			pgx.NamedArgs{
				"tenant_id":             tenantID,
				"inbound_event_id":      inboundID,
				"campaign_id":           campaignID,
				"campaign_recipient_id": recipientID,
				"provider_message_id":   pm,
			}).Scan(&eventID)
	}))

	require.NoError(t, subscribers.Delete(ctx, tenantID, subscriberID))

	require.NoError(t, tenantdb.WithTenant(ctx, pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var recipientID *string
		if err := tx.QueryRow(ctx,
			`SELECT campaign_recipient_id::text FROM delivery_events WHERE id = $1`,
			eventID).Scan(&recipientID); err != nil {
			return err
		}
		require.Nil(t, recipientID, "the event stays, detached from the deleted recipient")
		return nil
	}))
}
