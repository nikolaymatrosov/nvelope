package command

import (
	"context"
	"fmt"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// SignUp is the request to create a platform account.
type SignUp struct {
	Email    string
	Password string
	Name     string
}

// SignUpResult carries the created, still-unverified account. No session is
// issued at registration — the account must verify its email address before
// it can sign in.
type SignUpResult struct {
	UserID    string
	UserEmail string
	UserName  string
}

// SignUpHandler handles the SignUp command.
type SignUpHandler struct {
	users           domain.UserRepository
	hasher          PasswordHasher
	enqueuer        domain.VerificationEnqueuer
	policy          domain.RegistrationPolicy
	verificationTTL time.Duration
}

// NewSignUpHandler builds a SignUpHandler, failing fast on nil dependencies.
// policy enforces the configured email-domain allowlist; the zero value
// (an unrestricted policy) is acceptable.
func NewSignUpHandler(users domain.UserRepository, hasher PasswordHasher,
	enqueuer domain.VerificationEnqueuer, policy domain.RegistrationPolicy,
	verificationTTL time.Duration) SignUpHandler {
	if users == nil {
		panic("nil users repository")
	}
	if hasher == nil {
		panic("nil password hasher")
	}
	if enqueuer == nil {
		panic("nil verification enqueuer")
	}
	return SignUpHandler{
		users:           users,
		hasher:          hasher,
		enqueuer:        enqueuer,
		policy:          policy,
		verificationTTL: verificationTTL,
	}
}

// Handle validates the credentials, creates the account in an unverified
// state, and schedules its verification email. It does not issue a session —
// the account must verify its email address first.
func (h SignUpHandler) Handle(ctx context.Context, cmd SignUp) (SignUpResult, error) {
	email, err := domain.NewEmail(cmd.Email)
	if err != nil {
		return SignUpResult{}, err
	}
	// The domain allowlist is checked before any password hashing, account
	// creation, or job enqueue, so a refused domain leaves no trace (FR-015).
	if !h.policy.Allows(email) {
		return SignUpResult{}, domain.ErrEmailDomainNotAllowed
	}
	if _, err := domain.NewPassword(cmd.Password); err != nil {
		return SignUpResult{}, err
	}
	user, err := domain.NewUser(email, cmd.Name)
	if err != nil {
		return SignUpResult{}, err
	}
	hash, err := h.hasher.Hash(cmd.Password)
	if err != nil {
		return SignUpResult{}, fmt.Errorf("hashing password: %w", err)
	}

	var rawToken string
	created, err := h.users.CreateWithVerification(ctx, user, hash,
		func(userID string) (*domain.EmailVerification, string, error) {
			verification, err := domain.NewEmailVerification(userID, h.verificationTTL)
			if err != nil {
				return nil, "", err
			}
			raw, err := token.New()
			if err != nil {
				return nil, "", err
			}
			rawToken = raw
			return verification, token.Hash(raw), nil
		})
	if err != nil {
		return SignUpResult{}, err
	}

	if err := h.enqueuer.EnqueueVerificationSend(ctx, created.ID(), rawToken); err != nil {
		return SignUpResult{}, fmt.Errorf("enqueuing verification email: %w", err)
	}

	return SignUpResult{
		UserID:    created.ID(),
		UserEmail: created.Email().String(),
		UserName:  created.Name(),
	}, nil
}
