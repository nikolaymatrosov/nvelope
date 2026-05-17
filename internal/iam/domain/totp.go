package domain

import (
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// TOTPSecret is a validated TOTP shared secret. The raw secret is held only
// transiently — in storage it is always encrypted (see the iam app's TOTP
// capability and the adapters' config-keyed encryption).
type TOTPSecret struct {
	raw string
}

// NewTOTPSecret wraps a raw TOTP secret, rejecting an empty value.
func NewTOTPSecret(raw string) (TOTPSecret, error) {
	if raw == "" {
		return TOTPSecret{}, apperr.NewIncorrectInput("validation_failed",
			"a TOTP secret is required")
	}
	return TOTPSecret{raw: raw}, nil
}

// Raw returns the underlying secret string.
func (s TOTPSecret) Raw() string { return s.raw }

// RecoveryCode is one single-use TOTP recovery code. A batch is issued when a
// user enrols in TOTP; each code is stored hashed and consumed at most once.
type RecoveryCode struct {
	tenantID string
	userID   string
	codeHash string
	usedAt   *time.Time
}

// NewRecoveryCode builds an unused recovery code for a user.
func NewRecoveryCode(tenantID, userID, codeHash string) (*RecoveryCode, error) {
	if tenantID == "" || userID == "" || codeHash == "" {
		return nil, apperr.NewIncorrectInput("validation_failed",
			"a tenant, a user, and a code are required")
	}
	return &RecoveryCode{tenantID: tenantID, userID: userID, codeHash: codeHash}, nil
}

// HydrateRecoveryCode reconstructs a recovery code from a persisted row.
func HydrateRecoveryCode(tenantID, userID, codeHash string, usedAt *time.Time) *RecoveryCode {
	return &RecoveryCode{tenantID: tenantID, userID: userID, codeHash: codeHash, usedAt: usedAt}
}

// TenantID returns the owning tenant's id.
func (c *RecoveryCode) TenantID() string { return c.tenantID }

// UserID returns the id of the user the code belongs to.
func (c *RecoveryCode) UserID() string { return c.userID }

// CodeHash returns the stored hash of the recovery code.
func (c *RecoveryCode) CodeHash() string { return c.codeHash }

// UsedAt returns when the code was consumed, or nil.
func (c *RecoveryCode) UsedAt() *time.Time { return c.usedAt }

// IsUsed reports whether the code has already been consumed.
func (c *RecoveryCode) IsUsed() bool { return c.usedAt != nil }

// Use consumes the code. It returns an error when the code was already used.
func (c *RecoveryCode) Use(now time.Time) error {
	if c.usedAt != nil {
		return apperr.NewIncorrectInput("recovery_code_used",
			"that recovery code has already been used")
	}
	c.usedAt = &now
	return nil
}
