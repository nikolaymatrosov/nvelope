package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// PublicUnsubscribe is a subscriber's self-serve unsubscribe. The subscriber is
// identified by the caller after verifying the preference token. An empty
// ListID unsubscribes the subscriber from every list — the "unsubscribe from
// all" and one-click-unsubscribe action.
type PublicUnsubscribe struct {
	TenantID     string
	SubscriberID string
	ListID       string
}

// PublicUnsubscribeHandler handles the PublicUnsubscribe command.
type PublicUnsubscribeHandler struct {
	memberships domain.MembershipRepository
}

// NewPublicUnsubscribeHandler builds the handler, failing fast on a nil
// dependency.
func NewPublicUnsubscribeHandler(memberships domain.MembershipRepository) PublicUnsubscribeHandler {
	if memberships == nil {
		panic("nil membership repository")
	}
	return PublicUnsubscribeHandler{memberships: memberships}
}

// Handle moves the subscriber's membership(s) to unsubscribed. The change takes
// effect immediately, so the subscriber is excluded from any subsequent send.
func (h PublicUnsubscribeHandler) Handle(ctx context.Context, cmd PublicUnsubscribe) error {
	memberships, err := h.memberships.ForSubscriber(ctx, cmd.TenantID, cmd.SubscriberID)
	if err != nil {
		return err
	}
	for _, m := range memberships {
		if cmd.ListID != "" && m.ListID() != cmd.ListID {
			continue
		}
		if m.Status() == domain.SubscriptionUnsubscribed {
			continue
		}
		if err := h.memberships.SetStatus(ctx, cmd.TenantID, cmd.SubscriberID,
			m.ListID(), domain.SubscriptionUnsubscribed); err != nil {
			return err
		}
	}
	return nil
}
