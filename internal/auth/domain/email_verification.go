package domain

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// EmailVerification is a pending, time-bounded, single-use proof that a
// registrant controls the email address on their account. The raw token is
// not a field: it is generated when the verification is issued, returned once
// to the caller for the email link, and persisted only as a hash.
type EmailVerification struct {
	id         string
	userID     string
	expiresAt  time.Time
	createdAt  time.Time
	consumedAt *time.Time
}

// NewEmailVerification builds a verification challenge for userID that expires
// ttl from now.
func NewEmailVerification(userID string, ttl time.Duration) (*EmailVerification, error) {
	if userID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a user is required")
	}
	if ttl <= 0 {
		return nil, apperr.NewIncorrectInput("validation_failed", "a positive lifetime is required")
	}
	now := time.Now()
	return &EmailVerification{userID: userID, createdAt: now, expiresAt: now.Add(ttl)}, nil
}

// HydrateEmailVerification reconstructs a verification from a persisted row.
// Persistence only — it is not a constructor and does not re-run validation.
func HydrateEmailVerification(id, userID string, expiresAt, createdAt time.Time,
	consumedAt *time.Time) *EmailVerification {
	return &EmailVerification{
		id:         id,
		userID:     userID,
		expiresAt:  expiresAt,
		createdAt:  createdAt,
		consumedAt: consumedAt,
	}
}

// ID returns the database-assigned identifier, empty for an unpersisted
// verification.
func (v *EmailVerification) ID() string { return v.id }

// UserID returns the id of the user the verification belongs to.
func (v *EmailVerification) UserID() string { return v.userID }

// ExpiresAt returns the instant after which the verification link is rejected.
func (v *EmailVerification) ExpiresAt() time.Time { return v.expiresAt }

// IsExpired reports whether the verification link is past its validity window
// at now.
func (v *EmailVerification) IsExpired(now time.Time) bool { return now.After(v.expiresAt) }

// IsConsumed reports whether the verification link has already been used.
func (v *EmailVerification) IsConsumed() bool { return v.consumedAt != nil }

// VerificationEmail is one rendered email-verification message handed to the
// mailer for delivery. FromAddress is composed from the configured service
// sender domain.
type VerificationEmail struct {
	FromName    string
	FromAddress string
	To          string
	Subject     string
	HTMLBody    string
	TextBody    string
	Headers     map[string]string
}

// VerificationMailer delivers email-verification messages. It is declared here,
// by the verification worker that depends on it, and implemented by a
// composition-root bridge over the campaign context's messenger — so the auth
// context depends on an interface it owns, not on the campaign package.
type VerificationMailer interface {
	// Send delivers one verification message.
	Send(ctx context.Context, msg VerificationEmail) error
}

// VerificationEnqueuer schedules the asynchronous delivery of a verification
// email. It is declared here, by the use cases that consume it, and
// implemented by the River job enqueuer.
type VerificationEnqueuer interface {
	// EnqueueVerificationSend schedules a verification email for one user. The
	// raw token rides the (transient) job payload — it is needed to build the
	// link and is held only as a hash at rest.
	EnqueueVerificationSend(ctx context.Context, userID, rawToken string) error
}

// ResendThrottle bounds how often a verification email may be re-sent for one
// account, so the resend endpoint cannot be used to flood an inbox. It is
// declared here, by the resend use case, and implemented by a Redis-backed
// adapter.
type ResendThrottle interface {
	// Allow reports whether a verification-email resend for key may proceed now.
	Allow(ctx context.Context, key string) (bool, error)
}
