package service

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	authadapters "github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	authapp "github.com/nikolaymatrosov/nvelope/internal/auth/app"
	authcommand "github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	authquery "github.com/nikolaymatrosov/nvelope/internal/auth/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/config"
	"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"
	tenantadapters "github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	tenantapp "github.com/nikolaymatrosov/nvelope/internal/tenant/app"
	tenantcommand "github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	tenantquery "github.com/nikolaymatrosov/nvelope/internal/tenant/app/query"
)

// Application is the fully wired use-case surface of the nvelope service — the
// auth and tenant contexts' command and query handlers.
type Application struct {
	Auth   authapp.Application
	Tenant tenantapp.Application
}

// NewApplication is the composition root. It constructs the pgx-backed
// adapters, builds every command and query handler with logging decorators
// applied, and returns the wired Application. Dependencies flow through plain
// constructors — there is no DI framework and no hidden global state.
func NewApplication(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger) Application {
	users := authadapters.NewUsers(pool)
	sessions := authadapters.NewSessions(pool)
	hasher := authadapters.NewPasswordHasher()

	tenants := tenantadapters.NewTenants(pool)
	invitations := tenantadapters.NewInvitations(pool)
	settings := tenantadapters.NewSettings(pool)

	onboard := newOnboarding(pool, hasher, cfg.SessionTTL)
	directory := newMemberDirectory(users, tenants)

	auth := authapp.Application{
		Commands: authapp.Commands{
			SignUp: decorator.ApplyResultCommandDecorators(
				authcommand.NewSignUpHandler(users, hasher, cfg.SessionTTL), "SignUp", logger),
			LogIn: decorator.ApplyResultCommandDecorators(
				authcommand.NewLogInHandler(users, sessions, hasher, cfg.SessionTTL), "LogIn", logger),
			LogOut: decorator.ApplyCommandDecorators(
				authcommand.NewLogOutHandler(sessions), "LogOut", logger),
		},
		Queries: authapp.Queries{
			AuthenticateSession: decorator.ApplyQueryDecorators(
				authquery.NewAuthenticateSessionHandler(sessions, users), "AuthenticateSession", logger),
		},
	}

	tenant := tenantapp.Application{
		Commands: tenantapp.Commands{
			CreateWorkspace: decorator.ApplyResultCommandDecorators(
				tenantcommand.NewCreateWorkspaceHandler(tenants), "CreateWorkspace", logger),
			InviteTeammate: decorator.ApplyResultCommandDecorators(
				tenantcommand.NewInviteTeammateHandler(invitations, directory, cfg.InviteTTL), "InviteTeammate", logger),
			AcceptInvitation: decorator.ApplyResultCommandDecorators(
				tenantcommand.NewAcceptInvitationHandler(invitations, tenants, onboard), "AcceptInvitation", logger),
			RevokeInvitation: decorator.ApplyCommandDecorators(
				tenantcommand.NewRevokeInvitationHandler(invitations), "RevokeInvitation", logger),
			UpdateSettings: decorator.ApplyResultCommandDecorators(
				tenantcommand.NewUpdateSettingsHandler(settings), "UpdateSettings", logger),
		},
		Queries: tenantapp.Queries{
			ListWorkspaces: decorator.ApplyQueryDecorators(
				tenantquery.NewListWorkspacesHandler(tenants), "ListWorkspaces", logger),
			ResolveWorkspace: decorator.ApplyQueryDecorators(
				tenantquery.NewResolveWorkspaceHandler(tenants), "ResolveWorkspace", logger),
			WorkspaceMembers: decorator.ApplyQueryDecorators(
				tenantquery.NewWorkspaceMembersHandler(tenants), "WorkspaceMembers", logger),
			GetSettings: decorator.ApplyQueryDecorators(
				tenantquery.NewGetSettingsHandler(settings), "GetSettings", logger),
			PendingInvitations: decorator.ApplyQueryDecorators(
				tenantquery.NewPendingInvitationsHandler(invitations), "PendingInvitations", logger),
			LookUpInvitation: decorator.ApplyQueryDecorators(
				tenantquery.NewLookUpInvitationHandler(invitations, tenants), "LookUpInvitation", logger),
		},
	}

	return Application{Auth: auth, Tenant: tenant}
}
