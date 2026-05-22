package command

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// ResendEmailVerification is the request to re-send the verification email for
// an unverified account.
type ResendEmailVerification struct {
	Email string
}

// ResendEmailVerificationHandler handles the ResendEmailVerification command.
type ResendEmailVerificationHandler struct {
	users           domain.UserRepository
	verifications   domain.EmailVerificationRepository
	enqueuer        domain.VerificationEnqueuer
	throttle        domain.ResendThrottle
	verificationTTL time.Duration
}

// NewResendEmailVerificationHandler builds the handler, failing fast on nil
// dependencies.
func NewResendEmailVerificationHandler(users domain.UserRepository,
	verifications domain.EmailVerificationRepository, enqueuer domain.VerificationEnqueuer,
	throttle domain.ResendThrottle, verificationTTL time.Duration) ResendEmailVerificationHandler {

	if users == nil {
		panic("nil users repository")
	}
	if verifications == nil {
		panic("nil verifications repository")
	}
	if enqueuer == nil {
		panic("nil verification enqueuer")
	}
	if throttle == nil {
		panic("nil resend throttle")
	}
	return ResendEmailVerificationHandler{
		users:           users,
		verifications:   verifications,
		enqueuer:        enqueuer,
		throttle:        throttle,
		verificationTTL: verificationTTL,
	}
}

// Handle re-sends a verification email. To resist account enumeration it
// reports the same outcome for any syntactically valid address: an unknown or
// already-verified account simply yields no email. Only the per-account
// throttle surfaces an error. Issuing a fresh challenge supersedes any earlier
// pending one (FR-012).
func (h ResendEmailVerificationHandler) Handle(ctx context.Context, cmd ResendEmailVerification) error {
	email, err := domain.NewEmail(cmd.Email)
	if err != nil {
		return err
	}
	addr := email.String()

	allowed, err := h.throttle.Allow(ctx, strings.ToLower(addr))
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrVerificationResendThrottled
	}

	user, err := h.users.LookupByEmail(ctx, addr)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil // unknown address — no email, but indistinguishable to the caller
	}
	if err != nil {
		return err
	}
	if user.IsEmailVerified() {
		return nil // already verified — nothing to send
	}

	verification, err := domain.NewEmailVerification(user.ID(), h.verificationTTL)
	if err != nil {
		return err
	}
	raw, err := token.New()
	if err != nil {
		return err
	}
	if _, err := h.verifications.Issue(ctx, verification, token.Hash(raw)); err != nil {
		return err
	}
	if err := h.enqueuer.EnqueueVerificationSend(ctx, user.ID(), raw); err != nil {
		return err
	}
	return nil
}
