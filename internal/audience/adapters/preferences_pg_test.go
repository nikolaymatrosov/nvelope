package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestUpdatePreferencesAppliesNameAndMemberships(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	lists := adapters.NewLists(pool)
	subscribers := adapters.NewSubscribers(pool)
	memberships := adapters.NewMemberships(pool)
	news := addList(t, lists, tenantID, "Newsletter")
	announce := addList(t, lists, tenantID, "Announcements")
	subID := addSubscriber(t, subscribers, tenantID, "alice@example.com")
	require.NoError(t, memberships.Attach(ctx, tenantID, subID, news, domain.SubscriptionConfirmed))
	require.NoError(t, memberships.Attach(ctx, tenantID, subID, announce, domain.SubscriptionConfirmed))

	update := audiencecommand.NewUpdatePreferencesHandler(subscribers, memberships)
	require.NoError(t, update.Handle(ctx, audiencecommand.UpdatePreferences{
		TenantID:     tenantID,
		SubscriberID: subID,
		Name:         "Alice A.",
		Lists:        map[string]bool{news: true, announce: false},
	}))

	get := audiencequery.NewGetPreferencesHandler(subscribers, memberships, lists)
	view, err := get.Handle(ctx, audiencequery.GetPreferences{
		TenantID: tenantID, SubscriberID: subID,
	})
	require.NoError(t, err)
	require.Equal(t, "Alice A.", view.Name)
	got := map[string]bool{}
	for _, l := range view.Lists {
		got[l.ListID] = l.Subscribed
	}
	require.True(t, got[news], "Newsletter remains subscribed")
	require.False(t, got[announce], "Announcements is unsubscribed")
}

func TestPublicUnsubscribeAllAndSingleList(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	lists := adapters.NewLists(pool)
	subscribers := adapters.NewSubscribers(pool)
	memberships := adapters.NewMemberships(pool)
	news := addList(t, lists, tenantID, "Newsletter")
	announce := addList(t, lists, tenantID, "Announcements")

	// Unsubscribing from one list leaves the other confirmed.
	subID := addSubscriber(t, subscribers, tenantID, "alice@example.com")
	require.NoError(t, memberships.Attach(ctx, tenantID, subID, news, domain.SubscriptionConfirmed))
	require.NoError(t, memberships.Attach(ctx, tenantID, subID, announce, domain.SubscriptionConfirmed))

	unsub := audiencecommand.NewPublicUnsubscribeHandler(memberships)
	require.NoError(t, unsub.Handle(ctx, audiencecommand.PublicUnsubscribe{
		TenantID: tenantID, SubscriberID: subID, ListID: news,
	}))
	ms, err := memberships.ForSubscriber(ctx, tenantID, subID)
	require.NoError(t, err)
	statuses := map[string]domain.SubscriptionStatus{}
	for _, m := range ms {
		statuses[m.ListID()] = m.Status()
	}
	require.Equal(t, domain.SubscriptionUnsubscribed, statuses[news])
	require.Equal(t, domain.SubscriptionConfirmed, statuses[announce])

	// Unsubscribing from all moves every membership to unsubscribed.
	other := addSubscriber(t, subscribers, tenantID, "bob@example.com")
	require.NoError(t, memberships.Attach(ctx, tenantID, other, news, domain.SubscriptionConfirmed))
	require.NoError(t, memberships.Attach(ctx, tenantID, other, announce, domain.SubscriptionConfirmed))
	require.NoError(t, unsub.Handle(ctx, audiencecommand.PublicUnsubscribe{
		TenantID: tenantID, SubscriberID: other,
	}))
	ms, err = memberships.ForSubscriber(ctx, tenantID, other)
	require.NoError(t, err)
	for _, m := range ms {
		require.Equal(t, domain.SubscriptionUnsubscribed, m.Status(),
			"every membership unsubscribed when no list is specified")
	}
}

func TestInListExcludesUnsubscribed(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	lists := adapters.NewLists(pool)
	subscribers := adapters.NewSubscribers(pool)
	memberships := adapters.NewMemberships(pool)
	listID := addList(t, lists, tenantID, "Newsletter")

	active := addSubscriber(t, subscribers, tenantID, "active@example.com")
	gone := addSubscriber(t, subscribers, tenantID, "gone@example.com")
	require.NoError(t, memberships.Attach(ctx, tenantID, active, listID, domain.SubscriptionConfirmed))
	require.NoError(t, memberships.Attach(ctx, tenantID, gone, listID, domain.SubscriptionConfirmed))
	require.NoError(t, memberships.SetStatus(ctx, tenantID, gone, listID, domain.SubscriptionUnsubscribed))

	got, _, err := subscribers.InList(ctx, tenantID, listID, domain.Page{Limit: 50})
	require.NoError(t, err)
	require.Len(t, got, 1, "an unsubscribed member is excluded from the list's recipient set")
	require.Equal(t, "active@example.com", got[0].Email())
}
