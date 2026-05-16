package domain

import (
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// Session is a login session bound to a user. The raw session token is not a
// field: it is generated when the session is issued, returned once to the
// caller, and persisted only as a hash.
type Session struct {
	id        string
	userID    string
	expiresAt time.Time
	revokedAt *time.Time
}

// NewSession builds a new session for userID that expires ttl from now.
func NewSession(userID string, ttl time.Duration) (*Session, error) {
	if userID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a user is required")
	}
	return &Session{userID: userID, expiresAt: time.Now().Add(ttl)}, nil
}

// HydrateSession reconstructs a Session from a persisted row. Persistence only
// — it is not a constructor.
func HydrateSession(id, userID string, expiresAt time.Time, revokedAt *time.Time) *Session {
	return &Session{id: id, userID: userID, expiresAt: expiresAt, revokedAt: revokedAt}
}

// ID returns the database-assigned identifier.
func (s *Session) ID() string { return s.id }

// UserID returns the id of the user the session belongs to.
func (s *Session) UserID() string { return s.userID }

// ExpiresAt returns the session's expiry instant.
func (s *Session) ExpiresAt() time.Time { return s.expiresAt }

// IsLive reports whether the session is usable at now — not revoked and not
// expired.
func (s *Session) IsLive(now time.Time) bool {
	return s.revokedAt == nil && now.Before(s.expiresAt)
}

// Revoke marks the session revoked at now. Revoking an already-revoked session
// is a no-op.
func (s *Session) Revoke(now time.Time) {
	if s.revokedAt == nil {
		s.revokedAt = &now
	}
}
