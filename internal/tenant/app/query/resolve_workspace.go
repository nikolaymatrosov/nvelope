package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// ResolveWorkspace is the request to resolve a slug to a workspace the user
// belongs to.
type ResolveWorkspace struct {
	Slug   string
	UserID string
}

// ResolvedWorkspace is a workspace the caller is confirmed a member of.
type ResolvedWorkspace struct {
	ID     string
	Slug   string
	Name   string
	Status string
	Role   string
}

// ResolveWorkspaceHandler handles the ResolveWorkspace query.
type ResolveWorkspaceHandler struct {
	tenants domain.TenantRepository
}

// NewResolveWorkspaceHandler builds the handler, failing fast on a nil
// dependency.
func NewResolveWorkspaceHandler(tenants domain.TenantRepository) ResolveWorkspaceHandler {
	if tenants == nil {
		panic("nil tenants repository")
	}
	return ResolveWorkspaceHandler{tenants: tenants}
}

// Handle resolves the slug and confirms membership. An unknown slug and a
// non-member both yield an opaque not-found error, so a non-member cannot
// learn whether a workspace exists.
func (h ResolveWorkspaceHandler) Handle(ctx context.Context, q ResolveWorkspace) (ResolvedWorkspace, error) {
	tenant, err := h.tenants.GetBySlug(ctx, q.Slug)
	if err != nil {
		return ResolvedWorkspace{}, err
	}
	role, err := h.tenants.GetMembershipRole(ctx, q.UserID, tenant.ID())
	if err != nil {
		return ResolvedWorkspace{}, err
	}
	return ResolvedWorkspace{
		ID:     tenant.ID(),
		Slug:   tenant.Slug().String(),
		Name:   tenant.Name(),
		Status: string(tenant.Status()),
		Role:   role.String(),
	}, nil
}

// LocateWorkspace is the request to resolve a slug to a workspace without a
// membership cross-check — used for credentials that are themselves
// tenant-scoped, such as API keys.
type LocateWorkspace struct {
	Slug string
}

// LocateWorkspaceHandler handles the LocateWorkspace query.
type LocateWorkspaceHandler struct {
	tenants domain.TenantRepository
}

// NewLocateWorkspaceHandler builds the handler, failing fast on a nil
// dependency.
func NewLocateWorkspaceHandler(tenants domain.TenantRepository) LocateWorkspaceHandler {
	if tenants == nil {
		panic("nil tenants repository")
	}
	return LocateWorkspaceHandler{tenants: tenants}
}

// Handle resolves the slug to a workspace. The returned ResolvedWorkspace
// carries no membership Role — the caller's authority is established
// separately (by the API-key Principal).
func (h LocateWorkspaceHandler) Handle(ctx context.Context, q LocateWorkspace) (ResolvedWorkspace, error) {
	tenant, err := h.tenants.GetBySlug(ctx, q.Slug)
	if err != nil {
		return ResolvedWorkspace{}, err
	}
	return ResolvedWorkspace{
		ID:     tenant.ID(),
		Slug:   tenant.Slug().String(),
		Name:   tenant.Name(),
		Status: string(tenant.Status()),
	}, nil
}

// LocateWorkspaceByID is the request to resolve a workspace by its id —
// used by the token-addressed public pages, whose tokens carry the tenant id.
type LocateWorkspaceByID struct {
	TenantID string
}

// LocateWorkspaceByIDHandler handles the LocateWorkspaceByID query.
type LocateWorkspaceByIDHandler struct {
	tenants domain.TenantRepository
}

// NewLocateWorkspaceByIDHandler builds the handler, failing fast on a nil
// dependency.
func NewLocateWorkspaceByIDHandler(tenants domain.TenantRepository) LocateWorkspaceByIDHandler {
	if tenants == nil {
		panic("nil tenants repository")
	}
	return LocateWorkspaceByIDHandler{tenants: tenants}
}

// Handle resolves the id to a workspace. The returned ResolvedWorkspace carries
// no membership Role.
func (h LocateWorkspaceByIDHandler) Handle(ctx context.Context, q LocateWorkspaceByID) (ResolvedWorkspace, error) {
	tenant, err := h.tenants.GetByID(ctx, q.TenantID)
	if err != nil {
		return ResolvedWorkspace{}, err
	}
	return ResolvedWorkspace{
		ID:     tenant.ID(),
		Slug:   tenant.Slug().String(),
		Name:   tenant.Name(),
		Status: string(tenant.Status()),
	}, nil
}
