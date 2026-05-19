package command

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// ResendConfirmation is the request to issue a fresh confirmation link for a
// pending subscription whose previous link expired. Token is the (expired)
// token from that earlier link.
type ResendConfirmation struct {
	TenantID   string
	TenantSlug string
	Token      string
}

// ResendConfirmationHandler handles the ResendConfirmation command.
type ResendConfirmationHandler struct {
	pending  domain.PendingSubscriptionRepository
	enqueuer domain.OptinEnqueuer
	ttl      time.Duration
}

// NewResendConfirmationHandler builds the handler, failing fast on a nil
// dependency.
func NewResendConfirmationHandler(pending domain.PendingSubscriptionRepository,
	enqueuer domain.OptinEnqueuer, ttl time.Duration) ResendConfirmationHandler {

	if pending == nil || enqueuer == nil {
		panic("nil dependency")
	}
	if ttl <= 0 {
		panic("non-positive confirmation ttl")
	}
	return ResendConfirmationHandler{pending: pending, enqueuer: enqueuer, ttl: ttl}
}

// Handle issues a fresh token and expiry for the pending subscription and
// re-enqueues the confirmation email.
func (h ResendConfirmationHandler) Handle(ctx context.Context, cmd ResendConfirmation) error {
	pending, err := h.pending.GetByTokenHash(ctx, cmd.TenantID, token.Hash(cmd.Token))
	if err != nil {
		return err
	}
	rawToken, err := token.New()
	if err != nil {
		return apperr.Wrap(err, apperr.Unknown, "internal_error", "could not resend the confirmation")
	}
	if err := h.pending.RefreshToken(ctx, cmd.TenantID, pending.ID(),
		token.Hash(rawToken), time.Now().Add(h.ttl)); err != nil {
		return err
	}
	return h.enqueuer.EnqueueOptinSend(ctx, cmd.TenantID, cmd.TenantSlug, pending.ID(), rawToken)
}
