package adapters_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// newCampaignID persists a draft campaign of the tenant and returns its id.
func newCampaignID(t *testing.T, b backend, tenantID string) string {
	t.Helper()
	id, err := b.campaigns(t).Add(context.Background(), tenantID,
		newCampaign(t, tenantID, "C-"+fmt.Sprint(time.Now().UnixNano()), b.newSendingDomain(t, tenantID)))
	require.NoError(t, err)
	return id
}

func TestRecipientsBulkInsertDedupesByEmail(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID := newCampaignID(t, b, tenantID)

		sub1 := b.newSubscriber(t, tenantID, "a@acme.com")
		sub2 := b.newSubscriber(t, tenantID, "b@acme.com")

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

		// A set that overlaps the persisted one inserts only the new rows.
		sub3 := b.newSubscriber(t, tenantID, "c@acme.com")
		inserted, err = recipients.BulkInsert(ctx, tenantID, campaignID, append(rs,
			domain.NewRecipient(tenantID, campaignID, sub3, "c@acme.com")))
		require.NoError(t, err)
		require.Equal(t, 1, inserted)

		// The same email is free in another campaign.
		other := newCampaignID(t, b, tenantID)
		inserted, err = recipients.BulkInsert(ctx, tenantID, other, rs[:1])
		require.NoError(t, err)
		require.Equal(t, 1, inserted)
	})
}

// TestRecipientsBulkInsertSpansChunks writes more recipients than one YDB
// transaction chunk holds, with duplicates across the chunk boundary.
func TestRecipientsBulkInsertSpansChunks(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID := newCampaignID(t, b, tenantID)

		const n = 2300
		subID := b.newSubscriber(t, tenantID, "s@acme.com")
		var rs []*domain.Recipient
		for i := 0; i < n; i++ {
			rs = append(rs, domain.NewRecipient(tenantID, campaignID, subID, fmt.Sprintf("u%d@acme.com", i)))
		}
		rs = append(rs, rs[0], rs[1500]) // repeats of a first-chunk and a later row

		inserted, err := recipients.BulkInsert(ctx, tenantID, campaignID, rs)
		require.NoError(t, err)
		require.Equal(t, n, inserted)

		sent, failed, pending, err := recipients.Counts(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Equal(t, []int{0, 0, n}, []int{sent, failed, pending})

		// A window past 1000 rows comes back whole.
		window, err := recipients.Pending(ctx, tenantID, campaignID, 100, 1500)
		require.NoError(t, err)
		require.Len(t, window, 1500)
		for i := 1; i < len(window); i++ {
			require.Less(t, window[i-1].ID(), window[i].ID(), "the window is in id order")
		}
	})
}

// TestRecipientsBulkInsertIsRaceFree runs overlapping BulkInserts of one
// campaign at once: every email lands once and none of the calls fails.
func TestRecipientsBulkInsertIsRaceFree(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID := newCampaignID(t, b, tenantID)
		subID := b.newSubscriber(t, tenantID, "s@acme.com")

		const workers, emails = 6, 40
		var rs []*domain.Recipient
		for i := 0; i < emails; i++ {
			rs = append(rs, domain.NewRecipient(tenantID, campaignID, subID, fmt.Sprintf("r%d@acme.com", i)))
		}
		var wg sync.WaitGroup
		errs := make([]error, workers)
		counts := make([]int, workers)
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				counts[w], errs[w] = recipients.BulkInsert(ctx, tenantID, campaignID, rs)
			}()
		}
		wg.Wait()

		total := 0
		for w := range errs {
			require.NoError(t, errs[w])
			total += counts[w]
		}
		require.Equal(t, emails, total, "each email is inserted by exactly one call")
		_, _, pending, err := recipients.Counts(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Equal(t, emails, pending)
	})
}

func TestRecipientsPendingAndProgress(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID := newCampaignID(t, b, tenantID)

		sub1 := b.newSubscriber(t, tenantID, "a@acme.com")
		sub2 := b.newSubscriber(t, tenantID, "b@acme.com")
		_, err := recipients.BulkInsert(ctx, tenantID, campaignID, []*domain.Recipient{
			domain.NewRecipient(tenantID, campaignID, sub1, "a@acme.com"),
			domain.NewRecipient(tenantID, campaignID, sub2, "b@acme.com"),
		})
		require.NoError(t, err)

		pending, err := recipients.Pending(ctx, tenantID, campaignID, 0, 10)
		require.NoError(t, err)
		require.Len(t, pending, 2)
		require.Equal(t, domain.RecipientPending, pending[0].Status())
		require.Equal(t, campaignID, pending[0].CampaignID())

		require.NoError(t, recipients.MarkSent(ctx, tenantID, pending[0].ID(), "pm-abc", time.Now()))
		require.NoError(t, recipients.MarkFailed(ctx, tenantID, pending[1].ID(), "bounced"))

		sent, failed, stillPending, err := recipients.Counts(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Equal(t, 1, sent)
		require.Equal(t, 1, failed)
		require.Equal(t, 0, stillPending)

		remaining, err := recipients.Pending(ctx, tenantID, campaignID, 0, 10)
		require.NoError(t, err)
		require.Empty(t, remaining, "sent and failed recipients are not re-selected")

		// The provider message id is persisted for the sent recipient so a
		// later bounce/complaint can be attributed to it.
		require.Equal(t, "pm-abc", b.providerMessageID(t, tenantID, pending[0].ID()))
		require.Empty(t, b.providerMessageID(t, tenantID, pending[1].ID()))

		// A recipient id that does not exist records nothing.
		missing := "00000000-0000-4000-8000-000000000000"
		require.NoError(t, recipients.MarkSent(ctx, tenantID, missing, "pm-x", time.Now()))
		require.NoError(t, recipients.MarkFailed(ctx, tenantID, missing, "x"))
	})
}

// TestRecipientsPendingWindowIsStable pins the batch contract: the window is
// taken over every recipient, so sending the first batch does not shift the
// second one.
func TestRecipientsPendingWindowIsStable(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID := newCampaignID(t, b, tenantID)
		subID := b.newSubscriber(t, tenantID, "s@acme.com")

		var rs []*domain.Recipient
		for i := 0; i < 6; i++ {
			rs = append(rs, domain.NewRecipient(tenantID, campaignID, subID, fmt.Sprintf("w%d@acme.com", i)))
		}
		_, err := recipients.BulkInsert(ctx, tenantID, campaignID, rs)
		require.NoError(t, err)

		all, err := recipients.Pending(ctx, tenantID, campaignID, 0, 6)
		require.NoError(t, err)
		require.Len(t, all, 6)
		secondBefore, err := recipients.Pending(ctx, tenantID, campaignID, 3, 3)
		require.NoError(t, err)
		require.Equal(t, ids(all[3:]), ids(secondBefore))

		for _, r := range all[:3] {
			require.NoError(t, recipients.MarkSent(ctx, tenantID, r.ID(), "pm", time.Now()))
		}
		secondAfter, err := recipients.Pending(ctx, tenantID, campaignID, 3, 3)
		require.NoError(t, err)
		require.Equal(t, ids(all[3:]), ids(secondAfter), "sending the first batch leaves the second where it was")
		firstAfter, err := recipients.Pending(ctx, tenantID, campaignID, 0, 3)
		require.NoError(t, err)
		require.Empty(t, firstAfter, "a redelivered first batch finds nothing left to send")
	})
}

func ids(rs []*domain.Recipient) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID()
	}
	return out
}

func TestRecipientsMarkSkipped(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		campaignID := newCampaignID(t, b, tenantID)
		sub := b.newSubscriber(t, tenantID, "skip@acme.com")
		_, err := recipients.BulkInsert(ctx, tenantID, campaignID, []*domain.Recipient{
			domain.NewRecipient(tenantID, campaignID, sub, "skip@acme.com"),
		})
		require.NoError(t, err)
		pending, err := recipients.Pending(ctx, tenantID, campaignID, 0, 10)
		require.NoError(t, err)
		require.Len(t, pending, 1)

		require.NoError(t, recipients.MarkSkipped(ctx, tenantID, pending[0].ID(), "hard_bounce"))

		// A skipped recipient is neither sent, failed, nor pending.
		sent, failed, stillPending, err := recipients.Counts(ctx, tenantID, campaignID)
		require.NoError(t, err)
		require.Equal(t, 0, sent+failed+stillPending)

		remaining, err := recipients.Pending(ctx, tenantID, campaignID, 0, 10)
		require.NoError(t, err)
		require.Empty(t, remaining, "a skipped recipient is not re-selected for sending")
	})
}

func TestRecipientsCrossTenantIsolation(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		recipients := b.recipients(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)
		campaignA := newCampaignID(t, b, tenantA)

		subA := b.newSubscriber(t, tenantA, "a@acme.com")
		_, err := recipients.BulkInsert(ctx, tenantA, campaignA, []*domain.Recipient{
			domain.NewRecipient(tenantA, campaignA, subA, "a@acme.com"),
		})
		require.NoError(t, err)
		own, err := recipients.Pending(ctx, tenantA, campaignA, 0, 10)
		require.NoError(t, err)
		require.Len(t, own, 1)

		// Tenant B sees none of tenant A's recipients, and cannot mark them.
		pending, err := recipients.Pending(ctx, tenantB, campaignA, 0, 10)
		require.NoError(t, err)
		require.Empty(t, pending)

		require.NoError(t, recipients.MarkFailed(ctx, tenantB, own[0].ID(), "not yours"))
		sent, failed, stillPending, err := recipients.Counts(ctx, tenantB, campaignA)
		require.NoError(t, err)
		require.Equal(t, 0, sent+failed+stillPending)
		_, failed, stillPending, err = recipients.Counts(ctx, tenantA, campaignA)
		require.NoError(t, err)
		require.Equal(t, []int{0, 1}, []int{failed, stillPending}, "tenant A's recipient is untouched")
	})
}
