package domain

import "context"

// TenantRepository persists tenants and their memberships. It is declared here,
// by the domain that depends on it; the pgx implementation lives in the
// adapters layer. Read-only listing operations shaped for the UI are not part
// of this interface — query handlers declare their own read-model interfaces.
type TenantRepository interface {
	// CreateWorkspace inserts the tenant, its owner Membership, and the initial
	// TenantSettings row in one transaction, and returns the persisted tenant
	// with its database-assigned id. The initial settings display name is the
	// tenant name. The settings insert runs after the new tenant id is bound to
	// app.tenant_id so the RLS WITH CHECK passes. It returns ErrSlugTaken when
	// the slug is in use.
	CreateWorkspace(ctx context.Context, t *Tenant, ownerID string) (*Tenant, error)
	// GetBySlug returns the tenant with the given slug, or ErrTenantNotFound.
	GetBySlug(ctx context.Context, slug string) (*Tenant, error)
	// GetByID returns the tenant with the given id, or ErrTenantNotFound.
	GetByID(ctx context.Context, id string) (*Tenant, error)
	// GetMembershipRole returns the user's role in the tenant, or ErrNotMember.
	GetMembershipRole(ctx context.Context, userID, tenantID string) (Role, error)
	// AddMembership records a membership. Re-adding an existing member is a
	// no-op.
	AddMembership(ctx context.Context, m *Membership) error
	// ListMembershipsForUser returns every tenant the user belongs to, with the
	// user's role in each, oldest membership first.
	ListMembershipsForUser(ctx context.Context, userID string) ([]MembershipDetail, error)
	// ListMembers returns every member of the tenant, oldest membership first.
	ListMembers(ctx context.Context, tenantID string) ([]Member, error)
}

// InvitationRepository persists tenant invitations. Only the hash of an
// invitation token is ever stored.
type InvitationRepository interface {
	// Create persists a new invitation, storing only the token's hash, and
	// returns the persisted invitation. It returns ErrInvitationExists when a
	// pending invitation for the same email already exists in the tenant.
	Create(ctx context.Context, inv *Invitation, tokenHash string) (*Invitation, error)
	// GetPendingByTokenHash returns the pending, unexpired invitation for a
	// token hash, or ErrInvitationNotFound.
	GetPendingByTokenHash(ctx context.Context, tokenHash string) (*Invitation, error)
	// ListPending returns the tenant's pending invitations, newest first.
	ListPending(ctx context.Context, tenantID string) ([]*Invitation, error)
	// Update loads the invitation, runs fn, and persists the result. The
	// closure is the transaction boundary — used by the accept and revoke
	// flows. It returns ErrInvitationNotFound when no invitation with that id
	// exists in the tenant.
	Update(ctx context.Context, id, tenantID string, fn func(*Invitation) (*Invitation, error)) error
}

// SettingsRepository persists per-tenant settings. Both operations run inside
// a tenant-bound (app.tenant_id) transaction — the adapter owns the
// Row-Level-Security binding; the application layer never sees it.
type SettingsRepository interface {
	// Get returns the bound tenant's settings, or ErrTenantNotFound.
	Get(ctx context.Context, tenantID string) (*TenantSettings, error)
	// Update loads the bound tenant's settings, runs fn, and persists the
	// result.
	Update(ctx context.Context, tenantID string, fn func(*TenantSettings) (*TenantSettings, error)) error
}
