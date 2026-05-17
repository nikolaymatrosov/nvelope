package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// AddToList is the request to attach a subscriber to a list.
type AddToList struct {
	TenantID     string
	SubscriberID string
	ListID       string
}

// AddToListHandler handles the AddToList command.
type AddToListHandler struct {
	memberships domain.MembershipRepository
}

// NewAddToListHandler builds the handler, failing fast on a nil dependency.
func NewAddToListHandler(memberships domain.MembershipRepository) AddToListHandler {
	if memberships == nil {
		panic("nil membership repository")
	}
	return AddToListHandler{memberships: memberships}
}

// Handle attaches the subscriber to the list in the unconfirmed status.
func (h AddToListHandler) Handle(ctx context.Context, cmd AddToList) error {
	return h.memberships.Attach(ctx, cmd.TenantID, cmd.SubscriberID, cmd.ListID,
		domain.SubscriptionUnconfirmed)
}

// RemoveFromList is the request to detach a subscriber from a list.
type RemoveFromList struct {
	TenantID     string
	SubscriberID string
	ListID       string
}

// RemoveFromListHandler handles the RemoveFromList command.
type RemoveFromListHandler struct {
	memberships domain.MembershipRepository
}

// NewRemoveFromListHandler builds the handler, failing fast on a nil
// dependency.
func NewRemoveFromListHandler(memberships domain.MembershipRepository) RemoveFromListHandler {
	if memberships == nil {
		panic("nil membership repository")
	}
	return RemoveFromListHandler{memberships: memberships}
}

// Handle detaches the subscriber from the list.
func (h RemoveFromListHandler) Handle(ctx context.Context, cmd RemoveFromList) error {
	return h.memberships.Detach(ctx, cmd.TenantID, cmd.SubscriberID, cmd.ListID)
}

// ChangeSubscriptionState is the request to change a membership's subscription
// status.
type ChangeSubscriptionState struct {
	TenantID     string
	SubscriberID string
	ListID       string
	Status       string
}

// ChangeSubscriptionStateHandler handles the ChangeSubscriptionState command.
type ChangeSubscriptionStateHandler struct {
	memberships domain.MembershipRepository
}

// NewChangeSubscriptionStateHandler builds the handler, failing fast on a nil
// dependency.
func NewChangeSubscriptionStateHandler(memberships domain.MembershipRepository) ChangeSubscriptionStateHandler {
	if memberships == nil {
		panic("nil membership repository")
	}
	return ChangeSubscriptionStateHandler{memberships: memberships}
}

// Handle changes the membership's subscription status through the domain state
// machine.
func (h ChangeSubscriptionStateHandler) Handle(ctx context.Context, cmd ChangeSubscriptionState) error {
	return h.memberships.SetStatus(ctx, cmd.TenantID, cmd.SubscriberID, cmd.ListID,
		domain.SubscriptionStatus(cmd.Status))
}
