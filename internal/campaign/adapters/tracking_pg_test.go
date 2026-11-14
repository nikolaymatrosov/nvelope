package adapters_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
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

// countRows returns the row count of a tracking table for one key column,
// read inside the tenant binding (the tables are under RLS).
func countRows(t *testing.T, pool *pgxpool.Pool, tenantID, query, arg string) int {
	t.Helper()
	var n int
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, query, arg).Scan(&n)
		}))
	return n
}

func TestTrackingRecordClickAttributesOnlyOwnRecipients(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTracking(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	campaignID, ownRecipient := seedCampaignWithRecipient(t, tenantID)
	_, otherRecipient := seedCampaignWithRecipient(t, tenantID)

	links, err := repo.UpsertLinks(ctx, tenantID, campaignID, []string{"https://acme.com/sale"})
	require.NoError(t, err)
	linkID := links["https://acme.com/sale"]
	const clicks = "SELECT count(*) FROM link_clicks WHERE link_id = $1"

	for _, s := range []string{otherRecipient, "", "not-a-uuid"} {
		url, err := repo.RecordClick(ctx, tenantID, linkID, s)
		require.NoError(t, err, "recipient %q", s)
		require.Equal(t, "https://acme.com/sale", url, "the redirect target is returned regardless")
	}
	require.Zero(t, countRows(t, pool, tenantID, clicks, linkID),
		"no click is attributed to a foreign, missing, or malformed recipient")

	_, err = repo.RecordClick(ctx, tenantID, linkID, ownRecipient)
	require.NoError(t, err)
	require.Equal(t, 1, countRows(t, pool, tenantID, clicks, linkID), "the campaign's own recipient is attributed")
}

func TestTrackingRecordViewIgnoresForeignRecipients(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTracking(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	campaignID, ownRecipient := seedCampaignWithRecipient(t, tenantID)
	_, otherRecipient := seedCampaignWithRecipient(t, tenantID)
	const views = "SELECT count(*) FROM campaign_views WHERE campaign_id = $1"

	require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, otherRecipient))
	require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, "not-a-uuid"))
	require.Zero(t, countRows(t, pool, tenantID, views, campaignID))

	require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, ownRecipient))
	require.Equal(t, 1, countRows(t, pool, tenantID, views, campaignID))
}
