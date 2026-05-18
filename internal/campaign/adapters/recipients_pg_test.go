package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestRecipientsBulkInsertDedupesByEmail(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	campaigns := adapters.NewCampaigns(pool)
	recipients := adapters.NewRecipients(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	domainID := seedSendingDomain(t, pool, tenantID, "mail.acme.com")
	campaignID, err := campaigns.Add(ctx, tenantID, newCampaign(t, tenantID, "C", domainID))
	require.NoError(t, err)

	sub1 := seedSubscriber(t, pool, tenantID, "a@acme.com")
	sub2 := seedSubscriber(t, pool, tenantID, "b@acme.com")

	rs := []*domain.Recipient{
		domain.NewRecipient(tenantID, campaignID, sub1, "a@acme.com"),
		domain.NewRecipient(tenantID, campaignID, sub2, "b@acme.com"),
		domain.NewRecipient(tenantID, campaignID, sub1, "a@acme.com"), // duplicate email
	}
	inserted, err := recipients.BulkInsert(ctx, tenantID, campaignID, rs)
	require.NoError(t, err)
	require.Equal(t, 2, inserted, "the UNIQUE (campaign_id, email) constraint dedupes")

	// A second BulkInsert of the same set inserts nothing.
	inserted, err = recipients.BulkInsert(ctx, tenantID, campaignID, rs)
	require.NoError(t, err)
	require.Equal(t, 0, inserted)
}

func TestRecipientsPendingAndProgress(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	campaigns := adapters.NewCampaigns(pool)
	recipients := adapters.NewRecipients(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	domainID := seedSendingDomain(t, pool, tenantID, "mail.acme.com")
	campaignID, err := campaigns.Add(ctx, tenantID, newCampaign(t, tenantID, "C", domainID))
	require.NoError(t, err)

	sub1 := seedSubscriber(t, pool, tenantID, "a@acme.com")
	sub2 := seedSubscriber(t, pool, tenantID, "b@acme.com")
	_, err = recipients.BulkInsert(ctx, tenantID, campaignID, []*domain.Recipient{
		domain.NewRecipient(tenantID, campaignID, sub1, "a@acme.com"),
		domain.NewRecipient(tenantID, campaignID, sub2, "b@acme.com"),
	})
	require.NoError(t, err)

	pending, err := recipients.Pending(ctx, tenantID, campaignID, 0, 10)
	require.NoError(t, err)
	require.Len(t, pending, 2)

	require.NoError(t, recipients.MarkSent(ctx, tenantID, pending[0].ID(), time.Now()))
	require.NoError(t, recipients.MarkFailed(ctx, tenantID, pending[1].ID(), "bounced"))

	sent, failed, stillPending, err := recipients.Counts(ctx, tenantID, campaignID)
	require.NoError(t, err)
	require.Equal(t, 1, sent)
	require.Equal(t, 1, failed)
	require.Equal(t, 0, stillPending)

	remaining, err := recipients.Pending(ctx, tenantID, campaignID, 0, 10)
	require.NoError(t, err)
	require.Empty(t, remaining, "sent and failed recipients are not re-selected")
}

func TestRecipientsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	campaigns := adapters.NewCampaigns(pool)
	recipients := adapters.NewRecipients(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)
	domainA := seedSendingDomain(t, pool, tenantA, "mail.acme.com")
	campaignA, err := campaigns.Add(ctx, tenantA, newCampaign(t, tenantA, "C", domainA))
	require.NoError(t, err)

	subA := seedSubscriber(t, pool, tenantA, "a@acme.com")
	_, err = recipients.BulkInsert(ctx, tenantA, campaignA, []*domain.Recipient{
		domain.NewRecipient(tenantA, campaignA, subA, "a@acme.com"),
	})
	require.NoError(t, err)

	// Tenant B sees none of tenant A's recipients.
	pending, err := recipients.Pending(ctx, tenantB, campaignA, 0, 10)
	require.NoError(t, err)
	require.Empty(t, pending)

	sent, failed, stillPending, err := recipients.Counts(ctx, tenantB, campaignA)
	require.NoError(t, err)
	require.Equal(t, 0, sent+failed+stillPending)
}
