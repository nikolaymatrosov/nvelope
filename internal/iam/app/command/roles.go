// Package command holds the iam context's state-changing handlers, named in
// business language. Privileged actions also append an audit record.
package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// recordAudit appends an audit record for a privileged action, ignoring a
// recording failure so the primary action's outcome is what the caller sees.
func recordAudit(ctx context.Context, audit domain.AuditRepository, tenantID, actorID,
	action, target string, metadata map[string]any) {
	_ = audit.Record(ctx, tenantID,
		domain.NewAuditRecord(tenantID, actorID, domain.PrincipalSession, action, target, metadata))
}

// CreateRole is the request to create a role.
type CreateRole struct {
	TenantID    string
	ActorID     string
	Name        string
	Permissions []string
}

// CreateRoleResult carries the new role's id.
type CreateRoleResult struct {
	RoleID string
}

// CreateRoleHandler handles the CreateRole command.
type CreateRoleHandler struct {
	roles domain.RoleRepository
	audit domain.AuditRepository
}

// NewCreateRoleHandler builds the handler, failing fast on a nil dependency.
func NewCreateRoleHandler(roles domain.RoleRepository, audit domain.AuditRepository) CreateRoleHandler {
	if roles == nil || audit == nil {
		panic("nil dependency")
	}
	return CreateRoleHandler{roles: roles, audit: audit}
}

// Handle validates the permission set and persists the new role.
func (h CreateRoleHandler) Handle(ctx context.Context, cmd CreateRole) (CreateRoleResult, error) {
	perms, err := domain.ParsePermissions(cmd.Permissions)
	if err != nil {
		return CreateRoleResult{}, err
	}
	role, err := domain.NewRole(cmd.TenantID, cmd.Name, perms)
	if err != nil {
		return CreateRoleResult{}, err
	}
	id, err := h.roles.Add(ctx, cmd.TenantID, role)
	if err != nil {
		return CreateRoleResult{}, err
	}
	recordAudit(ctx, h.audit, cmd.TenantID, cmd.ActorID, "role.create", id,
		map[string]any{"name": cmd.Name})
	return CreateRoleResult{RoleID: id}, nil
}

// UpdateRole is the request to rename and re-permission a role.
type UpdateRole struct {
	TenantID    string
	ActorID     string
	RoleID      string
	Name        string
	Permissions []string
}

// UpdateRoleHandler handles the UpdateRole command.
type UpdateRoleHandler struct {
	roles domain.RoleRepository
	audit domain.AuditRepository
}

// NewUpdateRoleHandler builds the handler, failing fast on a nil dependency.
func NewUpdateRoleHandler(roles domain.RoleRepository, audit domain.AuditRepository) UpdateRoleHandler {
	if roles == nil || audit == nil {
		panic("nil dependency")
	}
	return UpdateRoleHandler{roles: roles, audit: audit}
}

// Handle applies the new name and permissions to the role.
func (h UpdateRoleHandler) Handle(ctx context.Context, cmd UpdateRole) error {
	perms, err := domain.ParsePermissions(cmd.Permissions)
	if err != nil {
		return err
	}
	if err := h.roles.Update(ctx, cmd.TenantID, cmd.RoleID,
		func(r *domain.Role) (*domain.Role, error) {
			if err := r.Rename(cmd.Name); err != nil {
				return nil, err
			}
			if err := r.SetPermissions(perms); err != nil {
				return nil, err
			}
			return r, nil
		}); err != nil {
		return err
	}
	recordAudit(ctx, h.audit, cmd.TenantID, cmd.ActorID, "role.update", cmd.RoleID, nil)
	return nil
}

// DeleteRole is the request to delete a role.
type DeleteRole struct {
	TenantID string
	ActorID  string
	RoleID   string
}

// DeleteRoleHandler handles the DeleteRole command.
type DeleteRoleHandler struct {
	roles domain.RoleRepository
	audit domain.AuditRepository
}

// NewDeleteRoleHandler builds the handler, failing fast on a nil dependency.
func NewDeleteRoleHandler(roles domain.RoleRepository, audit domain.AuditRepository) DeleteRoleHandler {
	if roles == nil || audit == nil {
		panic("nil dependency")
	}
	return DeleteRoleHandler{roles: roles, audit: audit}
}

// Handle deletes the role, rejecting the deletion when it is still assigned.
func (h DeleteRoleHandler) Handle(ctx context.Context, cmd DeleteRole) error {
	if err := h.roles.Delete(ctx, cmd.TenantID, cmd.RoleID); err != nil {
		return err
	}
	recordAudit(ctx, h.audit, cmd.TenantID, cmd.ActorID, "role.delete", cmd.RoleID, nil)
	return nil
}

// AssignRole is the request to set a user's tenant-level role.
type AssignRole struct {
	TenantID string
	ActorID  string
	UserID   string
	RoleID   string
}

// AssignRoleHandler handles the AssignRole command.
type AssignRoleHandler struct {
	roles domain.RoleRepository
	audit domain.AuditRepository
}

// NewAssignRoleHandler builds the handler, failing fast on a nil dependency.
func NewAssignRoleHandler(roles domain.RoleRepository, audit domain.AuditRepository) AssignRoleHandler {
	if roles == nil || audit == nil {
		panic("nil dependency")
	}
	return AssignRoleHandler{roles: roles, audit: audit}
}

// Handle sets the user's tenant-level role.
func (h AssignRoleHandler) Handle(ctx context.Context, cmd AssignRole) error {
	if err := h.roles.AssignTenantRole(ctx, cmd.TenantID, cmd.UserID, cmd.RoleID); err != nil {
		return err
	}
	recordAudit(ctx, h.audit, cmd.TenantID, cmd.ActorID, "role.assign", cmd.UserID,
		map[string]any{"role_id": cmd.RoleID})
	return nil
}

// AssignListRole is the request to grant a user a per-list role.
type AssignListRole struct {
	TenantID string
	ActorID  string
	UserID   string
	ListID   string
	RoleID   string
}

// AssignListRoleHandler handles the AssignListRole command.
type AssignListRoleHandler struct {
	roles domain.RoleRepository
	audit domain.AuditRepository
}

// NewAssignListRoleHandler builds the handler, failing fast on a nil dependency.
func NewAssignListRoleHandler(roles domain.RoleRepository, audit domain.AuditRepository) AssignListRoleHandler {
	if roles == nil || audit == nil {
		panic("nil dependency")
	}
	return AssignListRoleHandler{roles: roles, audit: audit}
}

// Handle grants the user a per-list role.
func (h AssignListRoleHandler) Handle(ctx context.Context, cmd AssignListRole) error {
	if err := h.roles.AssignListRole(ctx, cmd.TenantID, cmd.UserID, cmd.ListID, cmd.RoleID); err != nil {
		return err
	}
	recordAudit(ctx, h.audit, cmd.TenantID, cmd.ActorID, "role.assign_list", cmd.UserID,
		map[string]any{"role_id": cmd.RoleID, "list_id": cmd.ListID})
	return nil
}

// RevokeRole is the request to remove a user's per-list role.
type RevokeRole struct {
	TenantID string
	ActorID  string
	UserID   string
	ListID   string
}

// RevokeRoleHandler handles the RevokeRole command.
type RevokeRoleHandler struct {
	roles domain.RoleRepository
	audit domain.AuditRepository
}

// NewRevokeRoleHandler builds the handler, failing fast on a nil dependency.
func NewRevokeRoleHandler(roles domain.RoleRepository, audit domain.AuditRepository) RevokeRoleHandler {
	if roles == nil || audit == nil {
		panic("nil dependency")
	}
	return RevokeRoleHandler{roles: roles, audit: audit}
}

// Handle removes the user's per-list role.
func (h RevokeRoleHandler) Handle(ctx context.Context, cmd RevokeRole) error {
	if err := h.roles.RemoveListRole(ctx, cmd.TenantID, cmd.UserID, cmd.ListID); err != nil {
		return err
	}
	recordAudit(ctx, h.audit, cmd.TenantID, cmd.ActorID, "role.revoke_list", cmd.UserID,
		map[string]any{"list_id": cmd.ListID})
	return nil
}
