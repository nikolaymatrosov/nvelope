package domain

import (
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// SessionState is the lifecycle state of a tenant-plane working session.
type SessionState string

const (
	// SessionTOTPPending is a session opened by a TOTP-enrolled user that has
	// not yet met its two-factor challenge. It grants no permissions.
	SessionTOTPPending SessionState = "totp-pending"
	// SessionActive is an authenticated, usable session.
	SessionActive SessionState = "active"
	// SessionRevoked is a closed or expired session — it authenticates nothing.
	SessionRevoked SessionState = "revoked"
)

// Session is a tenant-plane working session: an authenticated presence inside
// one tenant workspace. The raw token is returned once at creation and stored
// only as a hash.
type Session struct {
	id        string
	tenantID  string
	userID    string
	tokenHash string
	state     SessionState
	createdAt time.Time
	expiresAt time.Time
	revokedAt *time.Time
}

// NewSession builds a session. It starts totp-pending when the user has TOTP
// enabled, otherwise active.
func NewSession(tenantID, userID, tokenHash string, totpEnabled bool, expiresAt time.Time) (*Session, error) {
	if tenantID == "" || userID == "" || tokenHash == "" {
		return nil, apperr.NewIncorrectInput("validation_failed",
			"tenant, user, and token are required")
	}
	state := SessionActive
	if totpEnabled {
		state = SessionTOTPPending
	}
	return &Session{
		tenantID: tenantID, userID: userID, tokenHash: tokenHash,
		state: state, expiresAt: expiresAt,
	}, nil
}

// HydrateSession reconstructs a session from a persisted row. Persistence only.
func HydrateSession(id, tenantID, userID, tokenHash string, state SessionState,
	createdAt, expiresAt time.Time, revokedAt *time.Time) *Session {
	return &Session{
		id: id, tenantID: tenantID, userID: userID, tokenHash: tokenHash,
		state: state, createdAt: createdAt, expiresAt: expiresAt, revokedAt: revokedAt,
	}
}

// ID returns the session's database-assigned id.
func (s *Session) ID() string { return s.id }

// TenantID returns the owning tenant's id.
func (s *Session) TenantID() string { return s.tenantID }

// UserID returns the authenticated user's id.
func (s *Session) UserID() string { return s.userID }

// TokenHash returns the stored hash of the session token.
func (s *Session) TokenHash() string { return s.tokenHash }

// State returns the session's lifecycle state.
func (s *Session) State() SessionState { return s.state }

// CreatedAt returns when the session was opened.
func (s *Session) CreatedAt() time.Time { return s.createdAt }

// ExpiresAt returns when the session expires.
func (s *Session) ExpiresAt() time.Time { return s.expiresAt }

// RevokedAt returns when the session was revoked, or nil.
func (s *Session) RevokedAt() *time.Time { return s.revokedAt }

// IsActive reports whether the session currently authenticates requests: it is
// in the active state and not past its expiry.
func (s *Session) IsActive(now time.Time) bool {
	return s.state == SessionActive && now.Before(s.expiresAt)
}

// CompleteTOTP moves a totp-pending session to active once its two-factor
// challenge has been met.
func (s *Session) CompleteTOTP() error {
	if s.state != SessionTOTPPending {
		return apperr.NewIncorrectInput("invalid_transition",
			"this session is not awaiting a two-factor challenge")
	}
	s.state = SessionActive
	return nil
}

// Revoke closes the session.
func (s *Session) Revoke(now time.Time) {
	s.state = SessionRevoked
	s.revokedAt = &now
}
