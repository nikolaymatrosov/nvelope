package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// CreateWorkspace is the request to create a new workspace owned by a user.
type CreateWorkspace struct {
	OwnerID string
	Name    string
	Slug    string
}

// CreateWorkspaceResult carries the created workspace.
type CreateWorkspaceResult struct {
	TenantID string
	Slug     string
	Name     string
	Status   string
}

// CreateWorkspaceHandler handles the CreateWorkspace command.
type CreateWorkspaceHandler struct {
	tenants domain.TenantRepository
}

// NewCreateWorkspaceHandler builds the handler, failing fast on a nil
// dependency.
func NewCreateWorkspaceHandler(tenants domain.TenantRepository) CreateWorkspaceHandler {
	if tenants == nil {
		panic("nil tenants repository")
	}
	return CreateWorkspaceHandler{tenants: tenants}
}

// Handle validates the workspace name and slug and creates the workspace, its
// owner membership, and its initial settings.
func (h CreateWorkspaceHandler) Handle(ctx context.Context, cmd CreateWorkspace) (CreateWorkspaceResult, error) {
	tenant, err := domain.NewTenant(cmd.Name, cmd.Slug)
	if err != nil {
		return CreateWorkspaceResult{}, err
	}
	created, err := h.tenants.CreateWorkspace(ctx, tenant, cmd.OwnerID)
	if err != nil {
		return CreateWorkspaceResult{}, err
	}
	return CreateWorkspaceResult{
		TenantID: created.ID(),
		Slug:     created.Slug().String(),
		Name:     created.Name(),
		Status:   string(created.Status()),
	}, nil
}
