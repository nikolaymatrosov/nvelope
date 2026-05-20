package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestCreateRoleHandlerWritesAudit(t *testing.T) {
	t.Parallel()
	roles, audit := newFakeRoles(), &fakeAudit{}
	h := command.NewCreateRoleHandler(roles, audit)

	res, err := h.Handle(context.Background(), command.CreateRole{
		TenantID: "t1", ActorID: "u1", Name: "Editor",
		Permissions: []string{"lists:manage"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.RoleID)
	require.Len(t, audit.records, 1, "creating a role writes an audit record")
	require.Equal(t, "role.create", audit.records[0].Action)
}

func TestCreateRoleHandlerRejectsUnknownPermission(t *testing.T) {
	t.Parallel()
	h := command.NewCreateRoleHandler(newFakeRoles(), &fakeAudit{})
	_, err := h.Handle(context.Background(), command.CreateRole{
		TenantID: "t1", Name: "R", Permissions: []string{"lists:explode"},
	})
	require.Error(t, err)
}

func TestDeleteRoleHandlerRejectsAssigned(t *testing.T) {
	t.Parallel()
	roles, audit := newFakeRoles(), &fakeAudit{}
	created, err := command.NewCreateRoleHandler(roles, audit).Handle(context.Background(),
		command.CreateRole{TenantID: "t1", Name: "R", Permissions: []string{"lists:get"}})
	require.NoError(t, err)

	require.NoError(t, command.NewAssignRoleHandler(roles, audit).Handle(context.Background(),
		command.AssignRole{TenantID: "t1", UserID: "u2", RoleID: created.RoleID}))

	err = command.NewDeleteRoleHandler(roles, audit).Handle(context.Background(),
		command.DeleteRole{TenantID: "t1", RoleID: created.RoleID})
	require.ErrorIs(t, err, domain.ErrRoleInUse)
}

func TestOpenWorkspaceSessionHandler(t *testing.T) {
	t.Parallel()
	users, sessions, roles := newFakeUsers(), newFakeSessions(), newFakeRoles()
	u, err := domain.NewTenantUser("t1", "platform-1", "a@b.com", "Pat")
	require.NoError(t, err)
	users.add("user-1", u)

	h := command.NewOpenWorkspaceSessionHandler(users, sessions, roles, time.Hour)
	res, err := h.Handle(context.Background(), command.OpenWorkspaceSession{
		TenantID: "t1", PlatformUserID: "platform-1",
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Token)
	require.Equal(t, string(domain.SessionActive), res.State)
}

func TestOpenWorkspaceSessionTOTPPending(t *testing.T) {
	t.Parallel()
	users, sessions, roles := newFakeUsers(), newFakeSessions(), newFakeRoles()
	u, err := domain.NewTenantUser("t1", "platform-1", "a@b.com", "Pat")
	require.NoError(t, err)
	require.NoError(t, u.EnableTOTP([]byte("secret")))
	users.add("user-1", u)

	res, err := command.NewOpenWorkspaceSessionHandler(users, sessions, roles, time.Hour).Handle(
		context.Background(), command.OpenWorkspaceSession{TenantID: "t1", PlatformUserID: "platform-1"})
	require.NoError(t, err)
	require.Equal(t, string(domain.SessionTOTPPending), res.State,
		"a TOTP-enrolled user gets a totp-pending session")
}

func TestOpenWorkspaceSessionProvisionsFirstUserAsOwner(t *testing.T) {
	t.Parallel()
	users, sessions, roles := newFakeUsers(), newFakeSessions(), newFakeRoles()
	h := command.NewOpenWorkspaceSessionHandler(users, sessions, roles, time.Hour)

	res, err := h.Handle(context.Background(), command.OpenWorkspaceSession{
		TenantID: "t1", PlatformUserID: "owner", Email: "owner@b.com", Name: "Owner",
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Token)

	tenantPerms, _, err := roles.EffectiveFor(context.Background(), "t1", "user-1")
	require.NoError(t, err)
	require.Len(t, tenantPerms, 24, "the first user is provisioned the Owner role")

	_, err = h.Handle(context.Background(), command.OpenWorkspaceSession{
		TenantID: "t1", PlatformUserID: "member", Email: "member@b.com", Name: "Member",
	})
	require.NoError(t, err)
	memberPerms, _, err := roles.EffectiveFor(context.Background(), "t1", "user-2")
	require.NoError(t, err)
	require.Empty(t, memberPerms, "a later user gets no role until one is assigned")
}

func TestCloseSessionHandler(t *testing.T) {
	t.Parallel()
	users, sessions, roles := newFakeUsers(), newFakeSessions(), newFakeRoles()
	u, _ := domain.NewTenantUser("t1", "platform-1", "a@b.com", "Pat")
	users.add("user-1", u)

	open, err := command.NewOpenWorkspaceSessionHandler(users, sessions, roles, time.Hour).Handle(
		context.Background(), command.OpenWorkspaceSession{TenantID: "t1", PlatformUserID: "platform-1"})
	require.NoError(t, err)

	require.NoError(t, command.NewCloseSessionHandler(sessions).Handle(context.Background(),
		command.CloseSession{TenantID: "t1", Token: open.Token}))

	// Closing an unknown token is a no-op.
	require.NoError(t, command.NewCloseSessionHandler(sessions).Handle(context.Background(),
		command.CloseSession{TenantID: "t1", Token: "ghost"}))
}
