package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// RoleView is the read model for one role.
type RoleView struct {
	ID          string
	Name        string
	Permissions []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ListRoles is the request for every role in the tenant.
type ListRoles struct {
	TenantID string
}

// ListRolesHandler handles the ListRoles query.
type ListRolesHandler struct {
	roles domain.RoleRepository
}

// NewListRolesHandler builds the handler, failing fast on a nil dependency.
func NewListRolesHandler(roles domain.RoleRepository) ListRolesHandler {
	if roles == nil {
		panic("nil role repository")
	}
	return ListRolesHandler{roles: roles}
}

// Handle returns every role in the tenant as read models.
func (h ListRolesHandler) Handle(ctx context.Context, q ListRoles) ([]RoleView, error) {
	roles, err := h.roles.All(ctx, q.TenantID)
	if err != nil {
		return nil, err
	}
	out := make([]RoleView, 0, len(roles))
	for _, r := range roles {
		perms := make([]string, 0, len(r.Permissions()))
		for _, p := range r.Permissions() {
			perms = append(perms, string(p))
		}
		out = append(out, RoleView{
			ID: r.ID(), Name: r.Name(), Permissions: perms,
			CreatedAt: r.CreatedAt(), UpdatedAt: r.UpdatedAt(),
		})
	}
	return out, nil
}

// AuditTrail is the request for a page of the tenant's audit records.
type AuditTrail struct {
	TenantID string
	Page     domain.Page
}

// AuditRecordView is the read model for one audit record.
type AuditRecordView struct {
	ID        string
	ActorID   string
	ActorKind string
	Action    string
	Target    string
	Metadata  map[string]any
	CreatedAt time.Time
}

// AuditTrailResult is a page of audit records with the total count.
type AuditTrailResult struct {
	Records []AuditRecordView
	Total   int
}

// AuditTrailHandler handles the AuditTrail query.
type AuditTrailHandler struct {
	audit domain.AuditRepository
}

// NewAuditTrailHandler builds the handler, failing fast on a nil dependency.
func NewAuditTrailHandler(audit domain.AuditRepository) AuditTrailHandler {
	if audit == nil {
		panic("nil audit repository")
	}
	return AuditTrailHandler{audit: audit}
}

// Handle returns a page of the tenant's audit records, newest first.
func (h AuditTrailHandler) Handle(ctx context.Context, q AuditTrail) (AuditTrailResult, error) {
	records, total, err := h.audit.All(ctx, q.TenantID, q.Page)
	if err != nil {
		return AuditTrailResult{}, err
	}
	out := AuditTrailResult{Total: total, Records: make([]AuditRecordView, 0, len(records))}
	for _, r := range records {
		out.Records = append(out.Records, AuditRecordView{
			ID: r.ID, ActorID: r.ActorID, ActorKind: string(r.ActorKind),
			Action: r.Action, Target: r.Target, Metadata: r.Metadata, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}
