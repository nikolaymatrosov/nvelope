package domain

import "github.com/nikolaymatrosov/nvelope/internal/platform/apperr"

// Role is a member's role within a tenant.
type Role struct {
	value string
}

// The roles a membership may hold this phase.
var (
	// RoleOwner is the tenant's creator.
	RoleOwner = Role{"owner"}
	// RoleAdmin is a member who joined by accepting an invitation.
	RoleAdmin = Role{"admin"}
)

// NewRole parses a role name, rejecting any value outside the known set.
func NewRole(s string) (Role, error) {
	switch s {
	case RoleOwner.value:
		return RoleOwner, nil
	case RoleAdmin.value:
		return RoleAdmin, nil
	default:
		return Role{}, apperr.NewIncorrectInput("validation_failed", "invalid role")
	}
}

// String returns the role name.
func (r Role) String() string { return r.value }

// IsZero reports whether r is the unset zero value.
func (r Role) IsZero() bool { return r.value == "" }

// Membership is the association of a user with a tenant in a given role.
type Membership struct {
	userID   string
	tenantID string
	role     Role
}

// NewMembership builds a membership, validating its parts.
func NewMembership(userID, tenantID string, role Role) (*Membership, error) {
	if userID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a user is required")
	}
	if tenantID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a tenant is required")
	}
	if role.IsZero() {
		return nil, apperr.NewIncorrectInput("validation_failed", "a role is required")
	}
	return &Membership{userID: userID, tenantID: tenantID, role: role}, nil
}

// HydrateMembership reconstructs a Membership from a persisted row. Persistence
// only — it is not a constructor.
func HydrateMembership(userID, tenantID, role string) *Membership {
	return &Membership{userID: userID, tenantID: tenantID, role: Role{value: role}}
}

// UserID returns the platform user's id.
func (m *Membership) UserID() string { return m.userID }

// TenantID returns the tenant's id.
func (m *Membership) TenantID() string { return m.tenantID }

// Role returns the member's role.
func (m *Membership) Role() Role { return m.role }
