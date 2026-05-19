package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// CancelSubscription is the request to cancel a tenant's subscription at the
// end of its current period.
type CancelSubscription struct {
	TenantID string
	ActorID  string
}

// CancelSubscriptionHandler handles CancelSubscription. It flags the
// subscription to cancel at period end; the billing sweep terminates it once
// the period elapses.
type CancelSubscriptionHandler struct {
	subscriptions domain.SubscriptionRepository
	audit         AuditWriter
}

// NewCancelSubscriptionHandler builds the handler, failing fast on a nil
// dependency.
func NewCancelSubscriptionHandler(subscriptions domain.SubscriptionRepository,
	audit AuditWriter) CancelSubscriptionHandler {
	if subscriptions == nil || audit == nil {
		panic("nil dependency")
	}
	return CancelSubscriptionHandler{subscriptions: subscriptions, audit: audit}
}

// Handle flags the tenant's subscription for cancellation at period end.
func (h CancelSubscriptionHandler) Handle(ctx context.Context, cmd CancelSubscription) error {
	sub, found, err := h.subscriptions.Current(ctx, cmd.TenantID)
	if err != nil {
		return err
	}
	if !found {
		return domain.ErrNoSubscription
	}
	if err := h.subscriptions.Update(ctx, cmd.TenantID, sub.ID(),
		func(s *domain.Subscription) (*domain.Subscription, error) {
			return s, s.RequestCancellation()
		}); err != nil {
		return err
	}
	return h.audit.Record(ctx, cmd.TenantID, cmd.ActorID, "subscription.canceled", sub.ID())
}
