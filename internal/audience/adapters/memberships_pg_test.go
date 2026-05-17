package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// seedListAndSubscriber creates one list and one subscriber, returning their
// ids — the pair needed to exercise a membership.
func seedListAndSubscriber(t *testing.T, tenantID string,
	lists *adapters.Lists, subs *adapters.Subscribers) (listID, subscriberID string) {
	t.Helper()
	ctx := context.Background()

	listID = addList(t, lists, tenantID, "L")
	subscriberID = addSubscriber(t, subs, tenantID, "m@example.com")
	_ = ctx
	return listID, subscriberID
}

func TestMembershipAttachDetach(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	lists, subs := adapters.NewLists(pool), adapters.NewSubscribers(pool)
	repo := adapters.NewMemberships(pool)

	listID, subscriberID := seedListAndSubscriber(t, tenantID, lists, subs)

	require.NoError(t, repo.Attach(ctx, tenantID, subscriberID, listID, domain.SubscriptionUnconfirmed))

	got, err := repo.ForSubscriber(ctx, tenantID, subscriberID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, domain.SubscriptionUnconfirmed, got[0].Status())

	require.ErrorIs(t, repo.Attach(ctx, tenantID, subscriberID, listID, domain.SubscriptionUnconfirmed),
		domain.ErrMembershipExists)

	require.NoError(t, repo.Detach(ctx, tenantID, subscriberID, listID))
	got, err = repo.ForSubscriber(ctx, tenantID, subscriberID)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestMembershipSetStatus(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	lists, subs := adapters.NewLists(pool), adapters.NewSubscribers(pool)
	repo := adapters.NewMemberships(pool)

	listID, subscriberID := seedListAndSubscriber(t, tenantID, lists, subs)
	require.NoError(t, repo.Attach(ctx, tenantID, subscriberID, listID, domain.SubscriptionUnconfirmed))

	require.NoError(t, repo.SetStatus(ctx, tenantID, subscriberID, listID, domain.SubscriptionConfirmed))
	got, err := repo.ForSubscriber(ctx, tenantID, subscriberID)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionConfirmed, got[0].Status())

	require.Error(t,
		repo.SetStatus(ctx, tenantID, subscriberID, listID, domain.SubscriptionUnconfirmed),
		"an invalid transition is rejected by the domain state machine")
}

func TestMembershipDetachMissing(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	repo := adapters.NewMemberships(pool)

	err := repo.Detach(ctx, tenantID, "not-a-uuid", "also-not")
	require.ErrorIs(t, err, domain.ErrMembershipNotFound)
}
