package command

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// ConfirmSubscription is the request to confirm a pending subscription from
// the token in a confirmation link.
type ConfirmSubscription struct {
	TenantID string
	Token    string
}

// ConfirmSubscriptionResult reports the outcome of a confirmation.
type ConfirmSubscriptionResult struct {
	// AlreadyConfirmed is true when no pending subscription matched the token
	// because it was already confirmed (a benign repeat visit).
	AlreadyConfirmed bool
}

// ConfirmSubscriptionHandler handles the ConfirmSubscription command.
type ConfirmSubscriptionHandler struct {
	pending     domain.PendingSubscriptionRepository
	subscribers domain.SubscriberRepository
	memberships domain.MembershipRepository
	suppression domain.SuppressionLookup
}

// NewConfirmSubscriptionHandler builds the handler, failing fast on a nil
// dependency.
func NewConfirmSubscriptionHandler(pending domain.PendingSubscriptionRepository,
	subscribers domain.SubscriberRepository, memberships domain.MembershipRepository,
	suppression domain.SuppressionLookup) ConfirmSubscriptionHandler {

	if pending == nil || subscribers == nil || memberships == nil || suppression == nil {
		panic("nil dependency")
	}
	return ConfirmSubscriptionHandler{
		pending: pending, subscribers: subscribers, memberships: memberships, suppression: suppression,
	}
}

// Handle confirms the pending subscription: it promotes the address to a
// subscriber, confirms the target-list memberships, and deletes the pending
// row. It is idempotent — a repeat visit reports AlreadyConfirmed.
func (h ConfirmSubscriptionHandler) Handle(ctx context.Context,
	cmd ConfirmSubscription) (ConfirmSubscriptionResult, error) {

	pending, err := h.pending.GetByTokenHash(ctx, cmd.TenantID, token.Hash(cmd.Token))
	if errors.Is(err, domain.ErrPendingSubscriptionNotFound) {
		return ConfirmSubscriptionResult{AlreadyConfirmed: true}, nil
	}
	if err != nil {
		return ConfirmSubscriptionResult{}, err
	}
	if pending.IsExpired(time.Now()) {
		return ConfirmSubscriptionResult{}, domain.ErrConfirmationExpired
	}

	suppressed, err := h.suppression.Suppressed(ctx, cmd.TenantID, []string{pending.Email()})
	if err != nil {
		return ConfirmSubscriptionResult{}, err
	}
	if _, ok := suppressed[strings.ToLower(pending.Email())]; ok {
		return ConfirmSubscriptionResult{}, domain.ErrAddressSuppressed
	}

	name, _ := pending.Attributes().Get("name")
	nameStr, _ := name.(string)
	sub, err := domain.NewSubscriber(cmd.TenantID, pending.Email(), nameStr, pending.Attributes())
	if err != nil {
		return ConfirmSubscriptionResult{}, err
	}
	if _, err := h.subscribers.UpsertByEmail(ctx, cmd.TenantID, sub); err != nil {
		return ConfirmSubscriptionResult{}, err
	}

	subscriberID, err := h.resolveID(ctx, cmd.TenantID, pending.Email())
	if err != nil {
		return ConfirmSubscriptionResult{}, err
	}
	for _, listID := range pending.TargetListIDs() {
		if err := h.confirmMembership(ctx, cmd.TenantID, subscriberID, listID); err != nil {
			return ConfirmSubscriptionResult{}, err
		}
	}

	if err := h.pending.Delete(ctx, cmd.TenantID, pending.ID()); err != nil {
		return ConfirmSubscriptionResult{}, err
	}
	return ConfirmSubscriptionResult{}, nil
}

// resolveID re-loads an upserted subscriber by email to obtain its id.
func (h ConfirmSubscriptionHandler) resolveID(ctx context.Context, tenantID, email string) (string, error) {
	subs, _, err := h.subscribers.Search(ctx, tenantID, email, domain.Page{Limit: 1})
	if err != nil {
		return "", err
	}
	if len(subs) == 0 {
		return "", domain.ErrSubscriberNotFound
	}
	return subs[0].ID(), nil
}

// confirmMembership attaches the subscriber to a target list as confirmed, or
// moves an existing membership to confirmed.
func (h ConfirmSubscriptionHandler) confirmMembership(ctx context.Context,
	tenantID, subscriberID, listID string) error {

	err := h.memberships.Attach(ctx, tenantID, subscriberID, listID, domain.SubscriptionConfirmed)
	if errors.Is(err, domain.ErrMembershipExists) {
		return h.memberships.SetStatus(ctx, tenantID, subscriberID, listID, domain.SubscriptionConfirmed)
	}
	return err
}
