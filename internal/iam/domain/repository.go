package domain

import "context"

// Page is a pagination request for iam listings.
type Page struct {
	Offset int
	Limit  int
}

// Normalize clamps a page request to sane bounds.
func (p Page) Normalize() Page {
	out := p
	if out.Limit <= 0 {
		out.Limit = 50
	}
	if out.Limit > 200 {
		out.Limit = 200
	}
	if out.Offset < 0 {
		out.Offset = 0
	}
	return out
}

// UserRepository persists tenant-plane users. Every operation runs inside a
// tenant-bound (app.tenant_id) transaction.
type UserRepository interface {
	// Add persists a new user and returns its database-assigned id.
	Add(ctx context.Context, tenantID string, u *TenantUser) (string, error)
	// Update loads the user, runs fn, and persists the result.
	Update(ctx context.Context, tenantID, id string, fn func(*TenantUser) (*TenantUser, error)) error
	// Get returns the user, or ErrUserNotFound.
	Get(ctx context.Context, tenantID, id string) (*TenantUser, error)
	// ByPlatformUser returns the tenant-plane user for a control-plane
	// identity, or ErrUserNotFound.
	ByPlatformUser(ctx context.Context, tenantID, platformUserID string) (*TenantUser, error)
}

// SessionRepository persists tenant-plane working sessions.
type SessionRepository interface {
	// Add persists a new session and returns its database-assigned id.
	Add(ctx context.Context, tenantID string, s *Session) (string, error)
	// Update loads the session, runs fn, and persists the result.
	Update(ctx context.Context, tenantID, id string, fn func(*Session) (*Session, error)) error
	// ByTokenHash returns the session for a token hash, or ErrSessionNotFound.
	ByTokenHash(ctx context.Context, tenantID, tokenHash string) (*Session, error)
}

// RoleRepository persists roles and role assignments.
type RoleRepository interface {
	// Add persists a new role and returns its database-assigned id. It returns
	// ErrRoleNameTaken when the tenant already has a role with that name.
	Add(ctx context.Context, tenantID string, r *Role) (string, error)
	// Update loads the role, runs fn, and persists the result.
	Update(ctx context.Context, tenantID, id string, fn func(*Role) (*Role, error)) error
	// Delete removes the role. It returns ErrRoleInUse when the role is still
	// assigned.
	Delete(ctx context.Context, tenantID, id string) error
	// Get returns the role, or ErrRoleNotFound.
	Get(ctx context.Context, tenantID, id string) (*Role, error)
	// All returns every role in the tenant.
	All(ctx context.Context, tenantID string) ([]*Role, error)
	// AssignTenantRole sets a user's tenant-level role.
	AssignTenantRole(ctx context.Context, tenantID, userID, roleID string) error
	// AssignListRole grants a user a per-list role.
	AssignListRole(ctx context.Context, tenantID, userID, listID, roleID string) error
	// RemoveListRole removes a user's per-list role.
	RemoveListRole(ctx context.Context, tenantID, userID, listID string) error
	// EffectiveFor loads, in one round trip, a user's tenant-level permissions
	// and the permission set of each per-list role they hold.
	EffectiveFor(ctx context.Context, tenantID, userID string) (
		tenantPerms []Permission, listPerms map[string][]Permission, err error)
}

// APIKeyRepository persists scoped API keys. Every operation runs inside a
// tenant-bound transaction.
type APIKeyRepository interface {
	// Add persists a new API key and returns its database-assigned id.
	Add(ctx context.Context, tenantID string, k *APIKey) (string, error)
	// ByTokenHash returns the API key for a token hash, or ErrAPIKeyNotFound.
	ByTokenHash(ctx context.Context, tenantID, tokenHash string) (*APIKey, error)
	// Revoke marks the API key revoked. It returns ErrAPIKeyNotFound when absent.
	Revoke(ctx context.Context, tenantID, id string) error
	// TouchLastUsed records that the key was just used to authenticate.
	TouchLastUsed(ctx context.Context, tenantID, id string) error
	// All returns every API key in the tenant, newest first.
	All(ctx context.Context, tenantID string) ([]*APIKey, error)
}

// RecoveryCodeRepository persists single-use TOTP recovery codes.
type RecoveryCodeRepository interface {
	// AddBatch persists a fresh batch of recovery codes for a user, replacing
	// any codes the user previously held.
	AddBatch(ctx context.Context, tenantID, userID string, codeHashes []string) error
	// Consume marks a recovery code used, reporting whether a matching unused
	// code existed.
	Consume(ctx context.Context, tenantID, userID, codeHash string) (consumed bool, err error)
	// DeleteForUser removes every recovery code a user holds.
	DeleteForUser(ctx context.Context, tenantID, userID string) error
}

// AuditRepository appends and reads privileged-action audit records.
type AuditRepository interface {
	// Record appends one audit record.
	Record(ctx context.Context, tenantID string, r AuditRecord) error
	// All returns a page of the tenant's audit records, newest first.
	All(ctx context.Context, tenantID string, page Page) ([]AuditRecord, int, error)
}
