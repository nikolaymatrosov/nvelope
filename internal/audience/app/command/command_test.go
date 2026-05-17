package command_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

func TestCreateListHandler(t *testing.T) {
	t.Parallel()
	lists := newFakeLists()
	h := command.NewCreateListHandler(lists)

	res, err := h.Handle(context.Background(), command.CreateList{
		TenantID: "t1", Name: "Newsletter", Visibility: "private", OptIn: "single",
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.ListID)

	_, err = h.Handle(context.Background(), command.CreateList{
		TenantID: "t1", Name: "Newsletter", Visibility: "private", OptIn: "single",
	})
	require.ErrorIs(t, err, domain.ErrListNameTaken)

	_, err = h.Handle(context.Background(), command.CreateList{
		TenantID: "t1", Name: "  ", Visibility: "private", OptIn: "single",
	})
	require.Error(t, err, "a blank name is rejected by the domain constructor")
}

func TestUpdateAndDeleteListHandler(t *testing.T) {
	t.Parallel()
	lists := newFakeLists()
	created, err := command.NewCreateListHandler(lists).Handle(context.Background(),
		command.CreateList{TenantID: "t1", Name: "Old", Visibility: "private", OptIn: "single"})
	require.NoError(t, err)

	require.NoError(t, command.NewUpdateListHandler(lists).Handle(context.Background(),
		command.UpdateList{TenantID: "t1", ListID: created.ListID, Name: "New",
			Visibility: "public", OptIn: "double"}))
	got, err := lists.Get(context.Background(), "t1", created.ListID)
	require.NoError(t, err)
	require.Equal(t, "New", got.Name())
	require.Equal(t, domain.VisibilityPublic, got.Visibility())

	require.NoError(t, command.NewDeleteListHandler(lists).Handle(context.Background(),
		command.DeleteList{TenantID: "t1", ListID: created.ListID}))
	_, err = lists.Get(context.Background(), "t1", created.ListID)
	require.ErrorIs(t, err, domain.ErrListNotFound)
}

func TestCreateSubscriberHandlerAttachesLists(t *testing.T) {
	t.Parallel()
	subs, members := newFakeSubscribers(), newFakeMemberships()
	lists := newFakeLists()
	list, err := command.NewCreateListHandler(lists).Handle(context.Background(),
		command.CreateList{TenantID: "t1", Name: "L", Visibility: "private", OptIn: "single"})
	require.NoError(t, err)

	h := command.NewCreateSubscriberHandler(subs, members)
	res, err := h.Handle(context.Background(), command.CreateSubscriber{
		TenantID: "t1", Email: "a@b.com", Name: "Pat",
		Attributes: map[string]any{"plan": "pro"}, ListIDs: []string{list.ListID},
	})
	require.NoError(t, err)

	got, err := members.ForSubscriber(context.Background(), "t1", res.SubscriberID)
	require.NoError(t, err)
	require.Len(t, got, 1, "the subscriber was attached to the requested list")
}

func TestCreateSubscriberHandlerRejectsBadEmail(t *testing.T) {
	t.Parallel()
	h := command.NewCreateSubscriberHandler(newFakeSubscribers(), newFakeMemberships())
	_, err := h.Handle(context.Background(), command.CreateSubscriber{
		TenantID: "t1", Email: "not-an-email",
	})
	require.Error(t, err)
}

func TestUpdateSubscriberHandlerChangesState(t *testing.T) {
	t.Parallel()
	subs := newFakeSubscribers()
	created, err := command.NewCreateSubscriberHandler(subs, newFakeMemberships()).Handle(
		context.Background(), command.CreateSubscriber{TenantID: "t1", Email: "a@b.com"})
	require.NoError(t, err)

	require.NoError(t, command.NewUpdateSubscriberHandler(subs).Handle(context.Background(),
		command.UpdateSubscriber{TenantID: "t1", SubscriberID: created.SubscriberID,
			Name: "Renamed", State: "blocklisted"}))
	got, err := subs.Get(context.Background(), "t1", created.SubscriberID)
	require.NoError(t, err)
	require.Equal(t, domain.StateBlocklisted, got.State())
	require.Equal(t, "Renamed", got.Name())
}

func TestMembershipHandlers(t *testing.T) {
	t.Parallel()
	members := newFakeMemberships()
	ctx := context.Background()

	require.NoError(t, command.NewAddToListHandler(members).Handle(ctx,
		command.AddToList{TenantID: "t1", SubscriberID: "s1", ListID: "l1"}))

	require.NoError(t, command.NewChangeSubscriptionStateHandler(members).Handle(ctx,
		command.ChangeSubscriptionState{TenantID: "t1", SubscriberID: "s1", ListID: "l1",
			Status: "confirmed"}))

	require.NoError(t, command.NewRemoveFromListHandler(members).Handle(ctx,
		command.RemoveFromList{TenantID: "t1", SubscriberID: "s1", ListID: "l1"}))
	require.ErrorIs(t, command.NewRemoveFromListHandler(members).Handle(ctx,
		command.RemoveFromList{TenantID: "t1", SubscriberID: "s1", ListID: "l1"}),
		domain.ErrMembershipNotFound)
}
