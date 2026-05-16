package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// RevokeInvitation is the request to withdraw a pending invitation.
type RevokeInvitation struct {
	TenantID     string
	InvitationID string
}

// RevokeInvitationHandler handles the RevokeInvitation command.
type RevokeInvitationHandler struct {
	invitations domain.InvitationRepository
}

// NewRevokeInvitationHandler builds the handler, failing fast on a nil
// dependency.
func NewRevokeInvitationHandler(invitations domain.InvitationRepository) RevokeInvitationHandler {
	if invitations == nil {
		panic("nil invitations repository")
	}
	return RevokeInvitationHandler{invitations: invitations}
}

// Handle revokes the invitation. It returns domain.ErrInvitationNotFound when
// no pending invitation with that id exists in the tenant.
func (h RevokeInvitationHandler) Handle(ctx context.Context, cmd RevokeInvitation) error {
	return h.invitations.Update(ctx, cmd.InvitationID, cmd.TenantID,
		func(inv *domain.Invitation) (*domain.Invitation, error) {
			if err := inv.Revoke(); err != nil {
				return nil, err
			}
			return inv, nil
		})
}
