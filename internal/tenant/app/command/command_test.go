package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

func TestCreateWorkspace(t *testing.T) {
	t.Parallel()
	h := command.NewCreateWorkspaceHandler(newFakeTenants())

	result, err := h.Handle(context.Background(), command.CreateWorkspace{
		OwnerID: "user-1", Name: "Acme", Slug: "acme",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.TenantID)
	require.Equal(t, "acme", result.Slug)
	require.Equal(t, "active", result.Status)

	_, err = h.Handle(context.Background(), command.CreateWorkspace{
		OwnerID: "user-1", Name: "Acme", Slug: "api",
	})
	require.Error(t, err, "a reserved slug is rejected")
}

func TestInviteTeammate(t *testing.T) {
	t.Parallel()
	invitations := newFakeInvitations()
	h := command.NewInviteTeammateHandler(invitations,
		fakeDirectory{members: map[string]bool{}}, time.Hour)

	result, err := h.Handle(context.Background(), command.InviteTeammate{
		TenantID: "tenant-1", InviterID: "user-1", Email: "grace@example.com",
	})
	require.NoError(t, err)
	require.False(t, result.AlreadyMember)
	require.NotEmpty(t, result.InvitationID)
	require.NotEmpty(t, result.Token)
}

func TestInviteTeammateRejectsExistingMember(t *testing.T) {
	t.Parallel()
	invitations := newFakeInvitations()
	directory := fakeDirectory{members: map[string]bool{"tenant-1|grace@example.com": true}}
	h := command.NewInviteTeammateHandler(invitations, directory, time.Hour)

	result, err := h.Handle(context.Background(), command.InviteTeammate{
		TenantID: "tenant-1", InviterID: "user-1", Email: "grace@example.com",
	})
	require.NoError(t, err)
	require.True(t, result.AlreadyMember, "the email already belongs to a member")
	require.False(t, invitations.created, "no invitation is created for an existing member")
}

func TestRevokeInvitation(t *testing.T) {
	t.Parallel()
	invitations := newFakeInvitations()
	email, err := domain.NewEmail("grace@example.com")
	require.NoError(t, err)
	inv, err := domain.NewInvitation("tenant-1", email, "user-1", time.Hour)
	require.NoError(t, err)
	created, err := invitations.Create(context.Background(), inv, "hash")
	require.NoError(t, err)

	h := command.NewRevokeInvitationHandler(invitations)
	require.NoError(t, h.Handle(context.Background(), command.RevokeInvitation{
		TenantID: "tenant-1", InvitationID: created.ID(),
	}))

	stored := invitations.byID[created.ID()]
	require.Equal(t, domain.InvitationRevoked, stored.Status())

	err = h.Handle(context.Background(), command.RevokeInvitation{
		TenantID: "tenant-1", InvitationID: "no-such-id",
	})
	require.ErrorIs(t, err, domain.ErrInvitationNotFound)
}

func TestUpdateSettings(t *testing.T) {
	t.Parallel()
	settings := newFakeSettings()
	settings.byTenant["tenant-1"] = domain.HydrateTenantSettings("tenant-1", "Old", "UTC")
	h := command.NewUpdateSettingsHandler(settings)

	result, err := h.Handle(context.Background(), command.UpdateSettings{
		TenantID: "tenant-1", DisplayName: "Renamed", Timezone: "Europe/Madrid",
	})
	require.NoError(t, err)
	require.Equal(t, "Renamed", result.DisplayName)
	require.Equal(t, "Europe/Madrid", result.Timezone)

	_, err = h.Handle(context.Background(), command.UpdateSettings{
		TenantID: "tenant-1", DisplayName: "  ", Timezone: "UTC",
	})
	require.Error(t, err, "an empty display name is rejected")
}

func TestAcceptInvitationExistingUser(t *testing.T) {
	t.Parallel()
	tenants := newFakeTenants()
	created, err := tenants.CreateWorkspace(context.Background(), mustTenant(t), "owner-1")
	require.NoError(t, err)
	invitations := newFakeInvitations()
	seedPendingInvitation(t, invitations, created.ID())
	onboard := &fakeOnboarding{}

	h := command.NewAcceptInvitationHandler(invitations, tenants, onboard)
	result, err := h.Handle(context.Background(), command.AcceptInvitation{
		Token: "raw-token", CurrentUserID: "user-9",
	})
	require.NoError(t, err)
	require.Nil(t, result.NewUser, "an existing user creates no account")
	require.Equal(t, created.ID(), result.TenantID)
	require.Equal(t, 1, onboard.existingCalls)
}

func TestAcceptInvitationNewUser(t *testing.T) {
	t.Parallel()
	tenants := newFakeTenants()
	created, err := tenants.CreateWorkspace(context.Background(), mustTenant(t), "owner-1")
	require.NoError(t, err)
	invitations := newFakeInvitations()
	seedPendingInvitation(t, invitations, created.ID())
	onboard := &fakeOnboarding{}

	h := command.NewAcceptInvitationHandler(invitations, tenants, onboard)
	result, err := h.Handle(context.Background(), command.AcceptInvitation{
		Token: "raw-token", Password: "a-good-password", Name: "Grace",
	})
	require.NoError(t, err)
	require.NotNil(t, result.NewUser, "an anonymous caller gets a new account")
	require.NotEmpty(t, result.NewUser.SessionToken)
	require.Equal(t, 1, onboard.newCalls)
}

func TestAcceptInvitationUnknownToken(t *testing.T) {
	t.Parallel()
	h := command.NewAcceptInvitationHandler(newFakeInvitations(), newFakeTenants(), &fakeOnboarding{})
	_, err := h.Handle(context.Background(), command.AcceptInvitation{Token: "no-such-token"})
	require.ErrorIs(t, err, domain.ErrInvitationNotFound)
}

func mustTenant(t *testing.T) *domain.Tenant {
	t.Helper()
	tn, err := domain.NewTenant("Workspace", "workspace")
	require.NoError(t, err)
	return tn
}

// seedPendingInvitation stores a pending invitation reachable by the raw token
// "raw-token" (its hash is computed by the production token package).
func seedPendingInvitation(t *testing.T, invitations *fakeInvitations, tenantID string) {
	t.Helper()
	email, err := domain.NewEmail("grace@example.com")
	require.NoError(t, err)
	inv, err := domain.NewInvitation(tenantID, email, "owner-1", time.Hour)
	require.NoError(t, err)
	_, err = invitations.Create(context.Background(), inv, token.Hash("raw-token"))
	require.NoError(t, err)
}
