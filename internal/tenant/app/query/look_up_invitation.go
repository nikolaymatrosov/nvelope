package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// LookUpInvitation is the request to resolve a raw invitation token.
type LookUpInvitation struct {
	Token string
}

// InvitationLookup describes who an invitation is for and which workspace it
// grants access to.
type InvitationLookup struct {
	TenantSlug string
	TenantName string
	Email      string
}

// LookUpInvitationHandler handles the LookUpInvitation query.
type LookUpInvitationHandler struct {
	invitations domain.InvitationRepository
	tenants     domain.TenantRepository
}

// NewLookUpInvitationHandler builds the handler, failing fast on nil
// dependencies.
func NewLookUpInvitationHandler(invitations domain.InvitationRepository,
	tenants domain.TenantRepository) LookUpInvitationHandler {
	if invitations == nil {
		panic("nil invitations repository")
	}
	if tenants == nil {
		panic("nil tenants repository")
	}
	return LookUpInvitationHandler{invitations: invitations, tenants: tenants}
}

// Handle resolves a pending invitation token. It returns
// domain.ErrInvitationNotFound when the token is not usable, regardless of why.
func (h LookUpInvitationHandler) Handle(ctx context.Context, q LookUpInvitation) (InvitationLookup, error) {
	inv, err := h.invitations.GetPendingByTokenHash(ctx, token.Hash(q.Token))
	if err != nil {
		return InvitationLookup{}, err
	}
	tenant, err := h.tenants.GetByID(ctx, inv.TenantID())
	if err != nil {
		return InvitationLookup{}, err
	}
	return InvitationLookup{
		TenantSlug: tenant.Slug().String(),
		TenantName: tenant.Name(),
		Email:      inv.Email().String(),
	}, nil
}
