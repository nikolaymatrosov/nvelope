// Package domain holds the iam bounded context's business types: tenant-plane
// users, working sessions, roles, permissions, principals, API keys, TOTP, and
// audit records. It imports nothing from the app, adapters, or transport
// layers.
package domain

import "github.com/nikolaymatrosov/nvelope/internal/platform/apperr"

// Permission is a flat resource:action authorization string drawn from a fixed
// catalogue.
type Permission string

// The permission catalogue (contracts/permissions.md).
const (
	PermListsGet          Permission = "lists:get"
	PermListsManage       Permission = "lists:manage"
	PermSubscribersGet    Permission = "subscribers:get"
	PermSubscribersManage Permission = "subscribers:manage"
	PermSubscribersImport Permission = "subscribers:import"
	PermSubscribersExport Permission = "subscribers:export"
	PermRolesGet          Permission = "roles:get"
	PermRolesManage       Permission = "roles:manage"
	PermAPIKeysGet        Permission = "apikeys:get"
	PermAPIKeysManage     Permission = "apikeys:manage"
	PermAuditGet          Permission = "audit:get"
	PermSettingsGet       Permission = "settings:get"
	PermSettingsManage    Permission = "settings:manage"
)

// catalogue is the set of every known permission.
var catalogue = map[Permission]bool{
	PermListsGet: true, PermListsManage: true,
	PermSubscribersGet: true, PermSubscribersManage: true,
	PermSubscribersImport: true, PermSubscribersExport: true,
	PermRolesGet: true, PermRolesManage: true,
	PermAPIKeysGet: true, PermAPIKeysManage: true,
	PermAuditGet: true, PermSettingsGet: true, PermSettingsManage: true,
}

// listScoped is the subset of permissions that a per-list role can also grant.
var listScoped = map[Permission]bool{
	PermListsGet: true, PermListsManage: true,
	PermSubscribersGet: true, PermSubscribersManage: true,
}

// AllPermissions returns every permission in the catalogue — the permission
// set of the bootstrap Owner role.
func AllPermissions() []Permission {
	out := make([]Permission, 0, len(catalogue))
	for p := range catalogue {
		out = append(out, p)
	}
	return out
}

// NewPermission validates a raw string against the catalogue.
func NewPermission(s string) (Permission, error) {
	p := Permission(s)
	if !catalogue[p] {
		return "", apperr.NewIncorrectInput("unknown_permission",
			"unknown permission: "+s)
	}
	return p, nil
}

// ParsePermissions validates a slice of raw strings, rejecting the first that
// is not in the catalogue.
func ParsePermissions(raw []string) ([]Permission, error) {
	out := make([]Permission, 0, len(raw))
	for _, s := range raw {
		p, err := NewPermission(s)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// IsListScoped reports whether a per-list role granting p widens access for a
// single list.
func IsListScoped(p Permission) bool { return listScoped[p] }

// EffectivePermissions returns the union of a user's tenant-level permissions
// and the permissions of a per-list role — a per-list role only ever widens
// access for that list (research.md Decision 5).
func EffectivePermissions(tenant, list []Permission) []Permission {
	seen := make(map[Permission]bool, len(tenant)+len(list))
	out := make([]Permission, 0, len(tenant)+len(list))
	for _, p := range tenant {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, p := range list {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
