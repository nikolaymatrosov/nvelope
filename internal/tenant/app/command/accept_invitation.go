package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// Onboarding completes invitation acceptance atomically. Acceptance touches
// both the auth schema (account creation, session) and the tenant schema
// (invitation status, membership); an implementation wired at the composition
// root runs all of it in one transaction.
type Onboarding interface {
	// AcceptForExistingUser marks the invitation accepted and records the
	// membership for an already-registered user.
	AcceptForExistingUser(ctx context.Context, p ExistingUserAcceptance) error
	// AcceptForNewUser creates the account, issues a session, marks the
	// invitation accepted, and records the membership for a new user.
	AcceptForNewUser(ctx context.Context, p NewUserAcceptance) (NewlyOnboardedUser, error)
}

// ExistingUserAcceptance is the input to AcceptForExistingUser.
type ExistingUserAcceptance struct {
	InvitationID string
	TenantID     string
	UserID       string
}

// NewUserAcceptance is the input to AcceptForNewUser.
type NewUserAcceptance struct {
	InvitationID string
	TenantID     string
	Email        string
	Password     string
	Name         string
}

// NewlyOnboardedUser describes an account created while accepting an
// invitation.
type NewlyOnboardedUser struct {
	ID           string
	Email        string
	Name         string
	SessionToken string
}

// AcceptInvitation is the request to accept an invitation. An anonymous caller
// supplies a password and name to create an account; a logged-in caller leaves
// CurrentUserID set and those fields empty.
type AcceptInvitation struct {
	Token         string
	CurrentUserID string
	Password      string
	Name          string
}

// AcceptInvitationResult carries the joined workspace. NewUser is non-nil only
// when accepting created a new account.
type AcceptInvitationResult struct {
	TenantID     string
	TenantSlug   string
	TenantName   string
	TenantStatus string
	NewUser      *NewlyOnboardedUser
}

// AcceptInvitationHandler handles the AcceptInvitation command.
type AcceptInvitationHandler struct {
	invitations domain.InvitationRepository
	tenants     domain.TenantRepository
	onboarding  Onboarding
}

// NewAcceptInvitationHandler builds the handler, failing fast on nil
// dependencies.
func NewAcceptInvitationHandler(invitations domain.InvitationRepository,
	tenants domain.TenantRepository, onboarding Onboarding) AcceptInvitationHandler {
	if invitations == nil {
		panic("nil invitations repository")
	}
	if tenants == nil {
		panic("nil tenants repository")
	}
	if onboarding == nil {
		panic("nil onboarding")
	}
	return AcceptInvitationHandler{invitations: invitations, tenants: tenants, onboarding: onboarding}
}

// Handle looks up the pending invitation and joins the caller to the tenant,
// creating an account first when the caller is anonymous. It returns
// domain.ErrInvitationNotFound when the token is unknown, expired, revoked, or
// already accepted.
func (h AcceptInvitationHandler) Handle(ctx context.Context, cmd AcceptInvitation) (AcceptInvitationResult, error) {
	inv, err := h.invitations.GetPendingByTokenHash(ctx, token.Hash(cmd.Token))
	if err != nil {
		return AcceptInvitationResult{}, err
	}
	tenant, err := h.tenants.GetByID(ctx, inv.TenantID())
	if err != nil {
		return AcceptInvitationResult{}, err
	}

	result := AcceptInvitationResult{
		TenantID:     tenant.ID(),
		TenantSlug:   tenant.Slug().String(),
		TenantName:   tenant.Name(),
		TenantStatus: string(tenant.Status()),
	}

	if cmd.CurrentUserID != "" {
		if err := h.onboarding.AcceptForExistingUser(ctx, ExistingUserAcceptance{
			InvitationID: inv.ID(),
			TenantID:     inv.TenantID(),
			UserID:       cmd.CurrentUserID,
		}); err != nil {
			return AcceptInvitationResult{}, err
		}
		return result, nil
	}

	onboarded, err := h.onboarding.AcceptForNewUser(ctx, NewUserAcceptance{
		InvitationID: inv.ID(),
		TenantID:     inv.TenantID(),
		Email:        inv.Email().String(),
		Password:     cmd.Password,
		Name:         cmd.Name,
	})
	if err != nil {
		return AcceptInvitationResult{}, err
	}
	result.NewUser = &onboarded
	return result, nil
}
