package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// WorkspaceMembers is the request for every member of a workspace.
type WorkspaceMembers struct {
	TenantID string
}

// WorkspaceMembersHandler handles the WorkspaceMembers query.
type WorkspaceMembersHandler struct {
	tenants domain.TenantRepository
}

// NewWorkspaceMembersHandler builds the handler, failing fast on a nil
// dependency.
func NewWorkspaceMembersHandler(tenants domain.TenantRepository) WorkspaceMembersHandler {
	if tenants == nil {
		panic("nil tenants repository")
	}
	return WorkspaceMembersHandler{tenants: tenants}
}

// Handle returns the workspace's members as flat views.
func (h WorkspaceMembersHandler) Handle(ctx context.Context, q WorkspaceMembers) ([]MemberView, error) {
	members, err := h.tenants.ListMembers(ctx, q.TenantID)
	if err != nil {
		return nil, err
	}
	views := make([]MemberView, 0, len(members))
	for _, m := range members {
		views = append(views, MemberView{
			UserID: m.UserID,
			Email:  m.Email,
			Name:   m.Name,
			Role:   m.Role.String(),
		})
	}
	return views, nil
}
