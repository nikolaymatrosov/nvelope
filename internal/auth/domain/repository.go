package domain

import (
	"context"
	"time"
)

// UserRepository persists platform users. It is declared here, by the domain
// that depends on it; the pgx implementation lives in the adapters layer.
type UserRepository interface {
	// Create inserts a new user with an already-hashed password and returns the
	// persisted user with its database-assigned id. It returns ErrEmailTaken
	// when the email is already registered.
	Create(ctx context.Context, u *User, passwordHash string) (*User, error)
	// CreateWithSession atomically inserts a new user and issues an initial
	// session. issueSession is called with the new user's id and returns the
	// session to persist together with its token hash. The whole operation is
	// one transaction, so a failure leaves neither the user nor the session.
	CreateWithSession(ctx context.Context, u *User, passwordHash string,
		issueSession func(userID string) (s *Session, tokenHash string, err error)) (*User, error)
	// CreateWithVerification atomically inserts a new user and its first
	// email-verification challenge. issueVerification is called with the new
	// user's id and returns the challenge to persist together with its token
	// hash. The whole operation is one transaction, so a failure leaves neither
	// the user nor the challenge.
	CreateWithVerification(ctx context.Context, u *User, passwordHash string,
		issueVerification func(userID string) (v *EmailVerification, tokenHash string, err error)) (*User, error)
	// MarkEmailVerified records that the user verified their email address at
	// now. It is idempotent: marking an already-verified account is a no-op.
	MarkEmailVerified(ctx context.Context, userID string, now time.Time) error
	// GetByID returns the user with the given id, or ErrUserNotFound.
	GetByID(ctx context.Context, id string) (*User, error)
	// UpdateLocale persists the user's interface-language preference. It
	// returns ErrUserNotFound when no user has the given id.
	UpdateLocale(ctx context.Context, userID string, locale Locale) error
	// LookupByEmail returns the user with the given email, or ErrUserNotFound.
	LookupByEmail(ctx context.Context, email string) (*User, error)
	// GetCredentials returns the user and the stored bcrypt hash for an email,
	// or ErrUserNotFound. The hash never leaves the adapter/app boundary.
	GetCredentials(ctx context.Context, email string) (*User, string, error)
}

// SessionRepository persists login sessions. Only the hash of a session token
// is ever stored.
type SessionRepository interface {
	// Issue persists a new session, storing only the token's hash.
	Issue(ctx context.Context, s *Session, tokenHash string) error
	// ResolveByTokenHash returns the live session for a token hash, or
	// ErrSessionInvalid when the token is unknown, expired, or revoked.
	ResolveByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	// RevokeByTokenHash revokes the session for a token hash. Revoking an
	// unknown or already-revoked token is a no-op.
	RevokeByTokenHash(ctx context.Context, tokenHash string) error
}

// EmailVerificationRepository persists email-verification challenges. Only the
// hash of a verification token is ever stored. It is declared here, by the
// domain that depends on it; the pgx implementation lives in the adapters
// layer.
type EmailVerificationRepository interface {
	// Issue persists a new verification challenge for v.UserID, first deleting
	// any still-pending (unconsumed) challenge for that user so a freshly
	// issued link supersedes earlier ones.
	Issue(ctx context.Context, v *EmailVerification, tokenHash string) (*EmailVerification, error)
	// GetByTokenHash returns the verification for a token hash, or
	// ErrVerificationLinkInvalid when the token is unknown.
	GetByTokenHash(ctx context.Context, tokenHash string) (*EmailVerification, error)
	// Consume marks the verification challenge used at now. Consuming an
	// already-consumed challenge is a no-op.
	Consume(ctx context.Context, verificationID string, now time.Time) error
}
