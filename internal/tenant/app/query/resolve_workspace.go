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
