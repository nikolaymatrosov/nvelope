package command

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// MemberDirectory answers whether an email already belongs to a member of a
// tenant. The lookup spans the auth and tenant contexts, so it is wired at the
// composition root.
type MemberDirectory interface {
	IsMember(ctx context.Context, tenantID, email string) (bool, error)
}

// InviteTeammate is the request to invite a teammate by email.
type InviteTeammate struct {
	TenantID  string
	InviterID string
	Email     string
}

// InviteTeammateResult carries the created invitation and its raw token. When
// AlreadyMember is true the email already belongs to a member and no
// invitation was created.
type InviteTeammateResult struct {
	InvitationID  string
	Email         string
	Status        string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	Token         string
	AlreadyMember bool
}

// InviteTeammateHandler handles the InviteTeammate command.
type InviteTeammateHandler struct {
	invitations domain.InvitationRepository
	directory   MemberDirectory
	inviteTTL   time.Duration
}

// NewInviteTeammateHandler builds the handler, failing fast on nil
// dependencies.
func NewInviteTeammateHandler(invitations domain.InvitationRepository,
	directory MemberDirectory, inviteTTL time.Duration) InviteTeammateHandler {
	if invitations == nil {
		panic("nil invitations repository")
	}
	if directory == nil {
		panic("nil member directory")
	}
	return InviteTeammateHandler{invitations: invitations, directory: directory, inviteTTL: inviteTTL}
}

// Handle creates a pending invitation. When the email already belongs to a
// member it creates nothing and reports AlreadyMember.
func (h InviteTeammateHandler) Handle(ctx context.Context, cmd InviteTeammate) (InviteTeammateResult, error) {
	email, err := domain.NewEmail(cmd.Email)
	if err != nil {
		return InviteTeammateResult{}, err
	}
	alreadyMember, err := h.directory.IsMember(ctx, cmd.TenantID, email.String())
	if err != nil {
		return InviteTeammateResult{}, err
	}
	if alreadyMember {
		return InviteTeammateResult{AlreadyMember: true}, nil
	}

	inv, err := domain.NewInvitation(cmd.TenantID, email, cmd.InviterID, h.inviteTTL)
	if err != nil {
		return InviteTeammateResult{}, err
	}
	raw, err := token.New()
	if err != nil {
		return InviteTeammateResult{}, err
	}
	created, err := h.invitations.Create(ctx, inv, token.Hash(raw))
	if err != nil {
		return InviteTeammateResult{}, err
	}
	return InviteTeammateResult{
		InvitationID: created.ID(),
		Email:        created.Email().String(),
		Status:       string(created.Status()),
		CreatedAt:    created.CreatedAt(),
		ExpiresAt:    created.ExpiresAt(),
		Token:        raw,
	}, nil
}
