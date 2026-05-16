package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// ListWorkspaces is the request for every workspace a user belongs to.
type ListWorkspaces struct {
	UserID string
}

// ListWorkspacesHandler handles the ListWorkspaces query.
type ListWorkspacesHandler struct {
	tenants domain.TenantRepository
}

// NewListWorkspacesHandler builds the handler, failing fast on a nil
// dependency.
func NewListWorkspacesHandler(tenants domain.TenantRepository) ListWorkspacesHandler {
	if tenants == nil {
		panic("nil tenants repository")
	}
	return ListWorkspacesHandler{tenants: tenants}
}

// Handle returns the user's memberships as flat views.
func (h ListWorkspacesHandler) Handle(ctx context.Context, q ListWorkspaces) ([]MembershipView, error) {
	details, err := h.tenants.ListMembershipsForUser(ctx, q.UserID)
	if err != nil {
		return nil, err
	}
	views := make([]MembershipView, 0, len(details))
	for _, d := range details {
		views = append(views, MembershipView{
			ID:     d.Tenant.ID(),
			Slug:   d.Tenant.Slug().String(),
			Name:   d.Tenant.Name(),
			Status: string(d.Tenant.Status()),
			Role:   d.Role.String(),
		})
	}
	return views, nil
}
