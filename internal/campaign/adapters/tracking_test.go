package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// seedCampaignWithRecipient persists a campaign and one recipient, returning
// both ids.
func seedCampaignWithRecipient(t *testing.T, b backend, tenantID string) (campaignID, recipientID string) {
	t.Helper()
	ctx := context.Background()
	campaignID = newCampaignID(t, b, tenantID)
	subID := b.newSubscriber(t, tenantID, "s-"+campaignID+"@acme.com")
	recipients := b.recipients(t)
	_, err := recipients.BulkInsert(ctx, tenantID, campaignID, []*domain.Recipient{
		domain.NewRecipient(tenantID, campaignID, subID, "rcpt@acme.com"),
	})
	require.NoError(t, err)
	pending, err := recipients.Pending(ctx, tenantID, campaignID, 0, 1)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	return campaignID, pending[0].ID()
}

func TestTrackingUpsertLinksIsIdempotent(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID, _ := seedCampaignWithRecipient(t, b, tenantID)

		urls := []string{"https://acme.com/a", "https://acme.com/b", "https://acme.com/a"}
		first, err := repo.UpsertLinks(ctx, tenantID, campaignID, urls)
		require.NoError(t, err)
		require.Len(t, first, 2)

		second, err := repo.UpsertLinks(ctx, tenantID, campaignID, urls)
		require.NoError(t, err)
		require.Equal(t, first, second, "re-upserting yields the same link ids")

		// A new URL joins the existing ones without changing their ids.
		third, err := repo.UpsertLinks(ctx, tenantID, campaignID,
			[]string{"https://acme.com/a", "https://acme.com/c"})
		require.NoError(t, err)
		require.Equal(t, first["https://acme.com/a"], third["https://acme.com/a"])
		require.NotEmpty(t, third["https://acme.com/c"])

		empty, err := repo.UpsertLinks(ctx, tenantID, campaignID, nil)
		require.NoError(t, err)
		require.Empty(t, empty)
	})
}

// TestTrackingUpsertLinksIsRaceFree upserts the same URLs from several
// goroutines: every caller gets the same ids.
func TestTrackingUpsertLinksIsRaceFree(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID, _ := seedCampaignWithRecipient(t, b, tenantID)

		urls := []string{"https://acme.com/a", "https://acme.com/b"}
		const workers = 6
		results := make([]map[string]string, workers)
		errs := make([]error, workers)
		done := make(chan int)
		for w := 0; w < workers; w++ {
			go func() {
				results[w], errs[w] = repo.UpsertLinks(ctx, tenantID, campaignID, urls)
				done <- w
			}()
		}
		for range results {
			<-done
		}
		for w := range results {
			require.NoError(t, errs[w])
			require.Equal(t, results[0], results[w])
		}
	})
}

func TestTrackingRecordClickAndView(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID, recipientID := seedCampaignWithRecipient(t, b, tenantID)

		links, err := repo.UpsertLinks(ctx, tenantID, campaignID, []string{"https://acme.com/sale"})
		require.NoError(t, err)
		linkID := links["https://acme.com/sale"]

		url, err := repo.RecordClick(ctx, tenantID, linkID, recipientID)
		require.NoError(t, err)
		require.Equal(t, "https://acme.com/sale", url)
		require.Equal(t, 1, b.clickCount(t, tenantID, linkID))

		require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, recipientID))
		require.Equal(t, 1, b.viewCount(t, tenantID, campaignID))
	})
}

func TestTrackingRecordClickUnknownLink(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		tenantID := b.newTenant(t)
		_, recipientID := seedCampaignWithRecipient(t, b, tenantID)

		url, err := repo.RecordClick(context.Background(), tenantID, "not-a-uuid", recipientID)
		require.ErrorIs(t, err, domain.ErrLinkNotFound)
		require.Empty(t, url)
	})
}

func TestTrackingResolveTenant(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID, _ := seedCampaignWithRecipient(t, b, tenantID)

		links, err := repo.UpsertLinks(ctx, tenantID, campaignID, []string{"https://acme.com/x"})
		require.NoError(t, err)
		linkID := links["https://acme.com/x"]

		// The resolvers run with no tenant bound.
		resolved, err := repo.ResolveTenantForLink(ctx, linkID)
		require.NoError(t, err)
		require.Equal(t, tenantID, resolved)

		resolved, err = repo.ResolveTenantForCampaign(ctx, campaignID)
		require.NoError(t, err)
		require.Equal(t, tenantID, resolved)

		_, err = repo.ResolveTenantForLink(ctx, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrLinkNotFound)
		_, err = repo.ResolveTenantForCampaign(ctx, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
		_, err = repo.ResolveTenantForLink(ctx, "00000000-0000-4000-8000-000000000000")
		require.ErrorIs(t, err, domain.ErrLinkNotFound)
	})
}

func TestTrackingCrossTenantIsolation(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)
		campaignA, recipientA := seedCampaignWithRecipient(t, b, tenantA)

		links, err := repo.UpsertLinks(ctx, tenantA, campaignA, []string{"https://acme.com/x"})
		require.NoError(t, err)
		linkA := links["https://acme.com/x"]

		// Tenant B cannot record against tenant A's link or campaign.
		_, err = repo.RecordClick(ctx, tenantB, linkA, recipientA)
		require.ErrorIs(t, err, domain.ErrLinkNotFound)
		require.NoError(t, repo.RecordView(ctx, tenantB, campaignA, recipientA))
		require.Zero(t, b.viewCount(t, tenantA, campaignA))
		require.Zero(t, b.viewCount(t, tenantB, campaignA))
	})
}

func TestTrackingRecordClickAttributesOnlyOwnRecipients(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID, ownRecipient := seedCampaignWithRecipient(t, b, tenantID)
		_, otherRecipient := seedCampaignWithRecipient(t, b, tenantID)

		links, err := repo.UpsertLinks(ctx, tenantID, campaignID, []string{"https://acme.com/sale"})
		require.NoError(t, err)
		linkID := links["https://acme.com/sale"]

		for _, s := range []string{otherRecipient, "", "not-a-uuid"} {
			url, err := repo.RecordClick(ctx, tenantID, linkID, s)
			require.NoError(t, err, "recipient %q", s)
			require.Equal(t, "https://acme.com/sale", url, "the redirect target is returned regardless")
		}
		require.Zero(t, b.clickCount(t, tenantID, linkID),
			"no click is attributed to a foreign, missing, or malformed recipient")

		_, err = repo.RecordClick(ctx, tenantID, linkID, ownRecipient)
		require.NoError(t, err)
		require.Equal(t, 1, b.clickCount(t, tenantID, linkID), "the campaign's own recipient is attributed")
	})
}

func TestTrackingRecordViewIgnoresForeignRecipients(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.tracking(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID, ownRecipient := seedCampaignWithRecipient(t, b, tenantID)
		_, otherRecipient := seedCampaignWithRecipient(t, b, tenantID)

		require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, otherRecipient))
		require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, "not-a-uuid"))
		require.Zero(t, b.viewCount(t, tenantID, campaignID))

		require.NoError(t, repo.RecordView(ctx, tenantID, campaignID, ownRecipient))
		require.Equal(t, 1, b.viewCount(t, tenantID, campaignID))
	})
}
