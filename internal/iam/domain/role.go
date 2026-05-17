package domain

import (
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// Role is a named set of permissions within a tenant. It is a tenant-plane
// aggregate reached only through the RLS-bound transaction owned by its
// repository adapter.
type Role struct {
	id          string
	tenantID    string
	name        string
	permissions []Permission
	createdAt   time.Time
	updatedAt   time.Time
}

// NewRole builds a role, rejecting an empty name or an unknown permission.
func NewRole(tenantID, name string, permissions []Permission) (*Role, error) {
	if tenantID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a tenant is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "role name is required")
	}
	for _, p := range permissions {
		if _, err := NewPermission(string(p)); err != nil {
			return nil, err
		}
	}
	return &Role{tenantID: tenantID, name: name, permissions: dedupePermissions(permissions)}, nil
}

// HydrateRole reconstructs a role from a persisted row. Persistence only.
func HydrateRole(id, tenantID, name string, permissions []Permission,
	createdAt, updatedAt time.Time) *Role {
	return &Role{
		id: id, tenantID: tenantID, name: name, permissions: permissions,
		createdAt: createdAt, updatedAt: updatedAt,
	}
}

// ID returns the role's database-assigned id.
func (r *Role) ID() string { return r.id }

// TenantID returns the owning tenant's id.
func (r *Role) TenantID() string { return r.tenantID }

// Name returns the role name.
func (r *Role) Name() string { return r.name }

// Permissions returns the role's permissions.
func (r *Role) Permissions() []Permission { return r.permissions }

// CreatedAt returns when the role was created.
func (r *Role) CreatedAt() time.Time { return r.createdAt }

// UpdatedAt returns when the role was last changed.
func (r *Role) UpdatedAt() time.Time { return r.updatedAt }

// Rename changes the role name, rejecting an empty value.
func (r *Role) Rename(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return apperr.NewIncorrectInput("validation_failed", "role name is required")
	}
	r.name = name
	return nil
}

// SetPermissions replaces the role's permissions, rejecting an unknown one.
func (r *Role) SetPermissions(permissions []Permission) error {
	for _, p := range permissions {
		if _, err := NewPermission(string(p)); err != nil {
			return err
		}
	}
	r.permissions = dedupePermissions(permissions)
	return nil
}

// dedupePermissions removes duplicate permissions, preserving order.
func dedupePermissions(in []Permission) []Permission {
	seen := make(map[Permission]bool, len(in))
	out := make([]Permission, 0, len(in))
	for _, p := range in {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
