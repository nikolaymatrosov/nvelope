package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// seedListAndSubscriber creates one list and one subscriber, returning their
// ids — the pair needed to exercise a membership.
func seedListAndSubscriber(t *testing.T, b backend, tenantID string) (listID, subscriberID string) {
	t.Helper()
	listID = addList(t, b.lists(t), tenantID, "L")
	subscriberID = addSubscriber(t, b.subscribers(t), tenantID, "m@example.com")
	return listID, subscriberID
}

func TestMembershipAttachDetach(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		listID, subscriberID := seedListAndSubscriber(t, b, tenantID)

		require.NoError(t, repo.Attach(ctx, tenantID, subscriberID, listID, domain.SubscriptionUnconfirmed))

		got, err := repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, tenantID, got[0].TenantID())
		require.Equal(t, subscriberID, got[0].SubscriberID())
		require.Equal(t, listID, got[0].ListID())
		require.Equal(t, domain.SubscriptionUnconfirmed, got[0].Status())

		require.ErrorIs(t, repo.Attach(ctx, tenantID, subscriberID, listID, domain.SubscriptionConfirmed),
			domain.ErrMembershipExists)
		got, err = repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Equal(t, domain.SubscriptionUnconfirmed, got[0].Status(), "a rejected attach writes nothing")

		require.NoError(t, repo.Detach(ctx, tenantID, subscriberID, listID))
		got, err = repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

func TestMembershipAttachUnknownSubscriberOrListFails(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		listID, subscriberID := seedListAndSubscriber(t, b, tenantID)

		// Postgres rejects both through its foreign keys; YDB checks the rows
		// in the attaching transaction.
		require.Error(t, repo.Attach(ctx, tenantID, uuid.NewString(), listID, domain.SubscriptionConfirmed))
		require.Error(t, repo.Attach(ctx, tenantID, subscriberID, uuid.NewString(), domain.SubscriptionConfirmed))
		got, err := repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

func TestMembershipSetStatus(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		listID, subscriberID := seedListAndSubscriber(t, b, tenantID)
		require.NoError(t, repo.Attach(ctx, tenantID, subscriberID, listID, domain.SubscriptionUnconfirmed))

		require.NoError(t, repo.SetStatus(ctx, tenantID, subscriberID, listID, domain.SubscriptionConfirmed))
		got, err := repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Equal(t, domain.SubscriptionConfirmed, got[0].Status())

		require.Error(t,
			repo.SetStatus(ctx, tenantID, subscriberID, listID, domain.SubscriptionUnconfirmed),
			"an invalid transition is rejected by the domain state machine")
		got, err = repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Equal(t, domain.SubscriptionConfirmed, got[0].Status())

		require.ErrorIs(t,
			repo.SetStatus(ctx, tenantID, subscriberID, uuid.NewString(), domain.SubscriptionConfirmed),
			domain.ErrMembershipNotFound)
	})
}

func TestMembershipDetachMissing(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		err := repo.Detach(ctx, tenantID, "not-a-uuid", "also-not")
		require.ErrorIs(t, err, domain.ErrMembershipNotFound)
	})
}

func TestMembershipTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.memberships(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)
		listID, subscriberID := seedListAndSubscriber(t, b, tenantA)
		require.NoError(t, repo.Attach(ctx, tenantA, subscriberID, listID, domain.SubscriptionConfirmed))

		got, err := repo.ForSubscriber(ctx, tenantB, subscriberID)
		require.NoError(t, err)
		require.Empty(t, got)
		require.ErrorIs(t, repo.Detach(ctx, tenantB, subscriberID, listID), domain.ErrMembershipNotFound)
		got, err = repo.ForSubscriber(ctx, tenantA, subscriberID)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})
}

func TestListsDeleteRemovesMemberships(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		lists, subs, repo := b.lists(t), b.subscribers(t), b.memberships(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		gone := addList(t, lists, tenantID, "Gone")
		kept := addList(t, lists, tenantID, "Kept")
		subscriberID := addSubscriber(t, subs, tenantID, "m@example.com")
		require.NoError(t, repo.Attach(ctx, tenantID, subscriberID, gone, domain.SubscriptionConfirmed))
		require.NoError(t, repo.Attach(ctx, tenantID, subscriberID, kept, domain.SubscriptionConfirmed))

		require.NoError(t, lists.Delete(ctx, tenantID, gone))

		got, err := repo.ForSubscriber(ctx, tenantID, subscriberID)
		require.NoError(t, err)
		require.Len(t, got, 1, "only the deleted list's membership goes")
		require.Equal(t, kept, got[0].ListID())
		_, total, err := subs.InList(ctx, tenantID, gone, domain.DefaultPage)
		require.NoError(t, err)
		require.Zero(t, total)
	})
}
