package domain

import "context"

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
	// GetByID returns the user with the given id, or ErrUserNotFound.
	GetByID(ctx context.Context, id string) (*User, error)
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
