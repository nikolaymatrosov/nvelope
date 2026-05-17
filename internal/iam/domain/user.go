package domain

import (
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// UserStatus is a tenant-plane user's lifecycle status.
type UserStatus string

const (
	// UserActive is a user able to open a workspace session.
	UserActive UserStatus = "active"
	// UserSuspended is a user temporarily barred from the workspace.
	UserSuspended UserStatus = "suspended"
)

// TenantUser is one operator within a tenant. It links to the control-plane
// platform identity (research.md Decision 4) and owns the tenant-scoped state:
// status and TOTP enrolment.
type TenantUser struct {
	id             string
	tenantID       string
	platformUserID string
	email          string
	name           string
	status         UserStatus
	totpEnabled    bool
	totpSecret     []byte
	createdAt      time.Time
	updatedAt      time.Time
}

// NewTenantUser builds a tenant-plane user linked to a platform identity.
func NewTenantUser(tenantID, platformUserID, email, name string) (*TenantUser, error) {
	if tenantID == "" || platformUserID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed",
			"a tenant and a platform user are required")
	}
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "email is required")
	}
	return &TenantUser{
		tenantID: tenantID, platformUserID: platformUserID,
		email: email, name: strings.TrimSpace(name), status: UserActive,
	}, nil
}

// HydrateTenantUser reconstructs a user from a persisted row. Persistence only.
func HydrateTenantUser(id, tenantID, platformUserID, email, name string, status UserStatus,
	totpEnabled bool, totpSecret []byte, createdAt, updatedAt time.Time) *TenantUser {
	return &TenantUser{
		id: id, tenantID: tenantID, platformUserID: platformUserID,
		email: email, name: name, status: status,
		totpEnabled: totpEnabled, totpSecret: totpSecret,
		createdAt: createdAt, updatedAt: updatedAt,
	}
}

// ID returns the user's database-assigned id.
func (u *TenantUser) ID() string { return u.id }

// TenantID returns the owning tenant's id.
func (u *TenantUser) TenantID() string { return u.tenantID }

// PlatformUserID returns the linked control-plane identity.
func (u *TenantUser) PlatformUserID() string { return u.platformUserID }

// Email returns the user's email.
func (u *TenantUser) Email() string { return u.email }

// Name returns the user's name.
func (u *TenantUser) Name() string { return u.name }

// Status returns the user's lifecycle status.
func (u *TenantUser) Status() UserStatus { return u.status }

// TOTPEnabled reports whether the user has TOTP two-factor auth enabled.
func (u *TenantUser) TOTPEnabled() bool { return u.totpEnabled }

// TOTPSecret returns the encrypted TOTP shared secret, or nil when TOTP is off.
func (u *TenantUser) TOTPSecret() []byte { return u.totpSecret }

// CreatedAt returns when the user was created.
func (u *TenantUser) CreatedAt() time.Time { return u.createdAt }

// UpdatedAt returns when the user was last changed.
func (u *TenantUser) UpdatedAt() time.Time { return u.updatedAt }

// EnableTOTP turns on TOTP two-factor auth, storing the encrypted secret.
func (u *TenantUser) EnableTOTP(encryptedSecret []byte) error {
	if len(encryptedSecret) == 0 {
		return apperr.NewIncorrectInput("validation_failed", "a TOTP secret is required")
	}
	u.totpEnabled = true
	u.totpSecret = encryptedSecret
	return nil
}

// DisableTOTP turns off TOTP two-factor auth and clears the stored secret.
func (u *TenantUser) DisableTOTP() {
	u.totpEnabled = false
	u.totpSecret = nil
}

// Suspend bars the user from the workspace.
func (u *TenantUser) Suspend() { u.status = UserSuspended }

// Reactivate restores a suspended user.
func (u *TenantUser) Reactivate() { u.status = UserActive }
