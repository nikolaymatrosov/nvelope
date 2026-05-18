package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// seedCampaignWithRecipient persists a campaign and one recipient, returning
// both ids.
func seedCampaignWithRecipient(t *testing.T, tenantID string) (campaignID, recipientID string) {
	t.Helper()
	pool := dbtest.AppPool(t)
	campaigns := adapters.NewCampaigns(pool)
	recipients := adapters.NewRecipients(pool)
	ctx := context.Background()
	domainID := seedSendingDomain(t, pool, tenantID, "mail."+dbtest.RandString()+".com")
	cID, err := campaigns.Add(ctx, tenantID, newCampaign(t, tenantID, "C-"+dbtest.RandString(), domainID))
	require.NoError(t, err)
	subID := seedSubscriber(t, pool, tenantID, dbtest.RandString()+"@acme.com")
	_, err = recipients.BulkInsert(ctx, tenantID, cID, []*domain.Recipient{
		domain.NewRecipient(tenantID, cID, subID, "rcpt@acme.com"),
	})
	require.NoError(t, err)
	pending, err := recipients.Pending(ctx, tenantID, cID, 0, 1)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	return cID, pending[0].ID()
}

func TestTrackingUpsertLinksIsIdempotent(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTracking(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	campaignID, _ := seedCampaignWithRecipient(t, tenantID)

	urls := []string{"https://acme.com/a", "https://acme.com/b"}
	first, err := repo.UpsertLinks(ctx, tenantID, campaignID, urls)
	require.NoError(t, err)
	require.Len(t, first, 2)

	second, err := repo.UpsertLinks(ctx, tenantID, campaignID, urls)
	require.NoError(t, err)
	require.Equal(t, first, second, "re-upserting yields the same link ids")
}

func TestTrackingRecordClickAndView(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTracking(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	campaignID, recipientID := seedCampaignWithRecipient(t, tenantID)

	links, err := repo.UpsertLinks(ctx, tenantID, campaignID, []string{"https://acme.com/sale"})
	require.NoError(t, err)
	linkID := links["https://acme.com/sale"]

	url, err := repo.RecordClick(ctx, tenantID, linkID, recipientID)
	require.NoError(t, err)
	require.Equal(t, "https://acme.com/sale", url)

	require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, recipientID))
}

func TestTrackingRecordClickUnknownLink(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTracking(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	_, recipientID := seedCampaignWithRecipient(t, tenantID)

	_, err := repo.RecordClick(ctx, tenantID, "not-a-uuid", recipientID)
	require.ErrorIs(t, err, domain.ErrLinkNotFound)
}

func TestTrackingResolveTenant(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTracking(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	campaignID, _ := seedCampaignWithRecipient(t, tenantID)

	links, err := repo.UpsertLinks(ctx, tenantID, campaignID, []string{"https://acme.com/x"})
	require.NoError(t, err)
	linkID := links["https://acme.com/x"]

	// The resolvers run outside any tenant transaction, via the SECURITY
	// DEFINER functions.
	resolved, err := repo.ResolveTenantForLink(ctx, linkID)
	require.NoError(t, err)
	require.Equal(t, tenantID, resolved)

	resolved, err = repo.ResolveTenantForCampaign(ctx, campaignID)
	require.NoError(t, err)
	require.Equal(t, tenantID, resolved)

	_, err = repo.ResolveTenantForLink(ctx, "not-a-uuid")
	require.ErrorIs(t, err, domain.ErrLinkNotFound)
}

func TestTrackingCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTracking(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)
	campaignA, recipientA := seedCampaignWithRecipient(t, tenantA)

	links, err := repo.UpsertLinks(ctx, tenantA, campaignA, []string{"https://acme.com/x"})
	require.NoError(t, err)
	linkA := links["https://acme.com/x"]

	// Tenant B, bound to its own RLS context, cannot record against tenant A's
	// link or campaign.
	_, err = repo.RecordClick(ctx, tenantB, linkA, recipientA)
	require.ErrorIs(t, err, domain.ErrLinkNotFound)
}
