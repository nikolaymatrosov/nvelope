package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// seedTenant inserts a tenant row directly and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"dlv-"+dbtest.RandString()).Scan(&id))
	return id
}

// seedCampaignRecipient inserts a campaign and one recipient carrying
// providerMsgID, returning the campaign and recipient ids.
func seedCampaignRecipient(t *testing.T, pool *pgxpool.Pool, tenantID, providerMsgID string) (
	campaignID, recipientID string) {

	t.Helper()
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			var subscriberID string
			if err := tx.QueryRow(ctx,
				`INSERT INTO subscribers (tenant_id, email, name) VALUES ($1, $2, 'Sub')
				 RETURNING id`, tenantID, dbtest.RandString()+"@acme.com").Scan(&subscriberID); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx,
				`INSERT INTO campaigns
				   (tenant_id, name, subject, body_html, from_name, from_local_part, status)
				 VALUES ($1, $2, 'Subject', '<p>hi</p>', 'Acme', 'news', 'finished') RETURNING id`,
				tenantID, "C-"+dbtest.RandString()).Scan(&campaignID); err != nil {
				return err
			}
			return tx.QueryRow(ctx,
				`INSERT INTO campaign_recipients
				   (tenant_id, campaign_id, subscriber_id, email, status, sent_at, provider_message_id)
				 VALUES (@tenant_id, @campaign_id, @subscriber_id, @email, 'sent', now(), @provider_message_id)
				 RETURNING id`,
				pgx.NamedArgs{
					"tenant_id":           tenantID,
					"campaign_id":         campaignID,
					"subscriber_id":       subscriberID,
					"email":               "rcpt@acme.com",
					"provider_message_id": providerMsgID,
				}).Scan(&recipientID)
		}))
	return campaignID, recipientID
}

// seedTransactionalMessage inserts a transactional_messages row carrying
// providerMsgID and returns its id.
func seedTransactionalMessage(t *testing.T, pool *pgxpool.Pool, tenantID, providerMsgID string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO transactional_messages (tenant_id, provider_message_id, recipient_email)
				 VALUES ($1, $2, $3) RETURNING id`,
				tenantID, providerMsgID, "tx@acme.com").Scan(&id)
		}))
	return id
}

// withTenantQuery runs a single-row query inside tenantID's RLS-bound
// transaction, scanning the result into dst.
func withTenantQuery(t *testing.T, pool *pgxpool.Pool, tenantID, sql string, dst any) error {
	t.Helper()
	return tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, sql).Scan(dst)
		})
}

// newInbound builds an inbound notification for tests.
func newInbound(t *testing.T, kind domain.EventKind, providerMsgID string) domain.InboundNotification {
	t.Helper()
	n, err := domain.NewInboundNotification("dk-"+dbtest.RandString(), kind,
		providerMsgID, "rcpt@acme.com", time.Now(), []byte(`{"k":"v"}`))
	require.NoError(t, err)
	return n
}
