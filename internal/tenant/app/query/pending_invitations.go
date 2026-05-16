package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// PendingInvitations is the request for a workspace's pending invitations.
type PendingInvitations struct {
	TenantID string
}

// PendingInvitationsHandler handles the PendingInvitations query.
type PendingInvitationsHandler struct {
	invitations domain.InvitationRepository
}

// NewPendingInvitationsHandler builds the handler, failing fast on a nil
// dependency.
func NewPendingInvitationsHandler(invitations domain.InvitationRepository) PendingInvitationsHandler {
	if invitations == nil {
		panic("nil invitations repository")
	}
	return PendingInvitationsHandler{invitations: invitations}
}

// Handle returns the workspace's pending invitations as flat views.
func (h PendingInvitationsHandler) Handle(ctx context.Context, q PendingInvitations) ([]InvitationView, error) {
	invitations, err := h.invitations.ListPending(ctx, q.TenantID)
	if err != nil {
		return nil, err
	}
	views := make([]InvitationView, 0, len(invitations))
	for _, inv := range invitations {
		views = append(views, InvitationView{
			ID:        inv.ID(),
			Email:     inv.Email().String(),
			Status:    string(inv.Status()),
			CreatedAt: inv.CreatedAt(),
			ExpiresAt: inv.ExpiresAt(),
		})
	}
	return views, nil
}
