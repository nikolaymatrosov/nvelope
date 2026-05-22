package command

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// VerifyEmail is the request to complete email verification by presenting the
// raw token carried in a verification link.
type VerifyEmail struct {
	Token string
}

// VerifyEmailResult reports the outcome of a verification attempt.
type VerifyEmailResult struct {
	// AlreadyVerified is true when the link had already been used — an
	// idempotent success rather than an error.
	AlreadyVerified bool
}

// VerifyEmailHandler handles the VerifyEmail command.
type VerifyEmailHandler struct {
	users         domain.UserRepository
	verifications domain.EmailVerificationRepository
}

// NewVerifyEmailHandler builds the handler, failing fast on nil dependencies.
func NewVerifyEmailHandler(users domain.UserRepository,
	verifications domain.EmailVerificationRepository) VerifyEmailHandler {
	if users == nil {
		panic("nil users repository")
	}
	if verifications == nil {
		panic("nil verifications repository")
	}
	return VerifyEmailHandler{users: users, verifications: verifications}
}

// Handle verifies the account that owns the token. An unknown or expired token
// yields domain.ErrVerificationLinkInvalid — the two are deliberately
// indistinguishable so the response cannot probe for accounts. Re-opening an
// already-used link is an idempotent success.
func (h VerifyEmailHandler) Handle(ctx context.Context, cmd VerifyEmail) (VerifyEmailResult, error) {
	if cmd.Token == "" {
		return VerifyEmailResult{}, domain.ErrVerificationLinkInvalid
	}
	verification, err := h.verifications.GetByTokenHash(ctx, token.Hash(cmd.Token))
	if err != nil {
		return VerifyEmailResult{}, err
	}
	if verification.IsConsumed() {
		return VerifyEmailResult{AlreadyVerified: true}, nil
	}
	now := time.Now()
	if verification.IsExpired(now) {
		return VerifyEmailResult{}, domain.ErrVerificationLinkInvalid
	}

	// Mark the account verified before consuming the token: a crash between the
	// two leaves the account verified and the token still live, which the next
	// link open recovers harmlessly; the reverse would strand a consumed token
	// on an account that can never sign in.
	if err := h.users.MarkEmailVerified(ctx, verification.UserID(), now); err != nil {
		return VerifyEmailResult{}, err
	}
	if err := h.verifications.Consume(ctx, verification.ID(), now); err != nil {
		return VerifyEmailResult{}, err
	}
	return VerifyEmailResult{}, nil
}
