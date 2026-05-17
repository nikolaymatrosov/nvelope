package service

import (
	"encoding/hex"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	audienceadapters "github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	audienceapp "github.com/nikolaymatrosov/nvelope/internal/audience/app"
	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	authadapters "github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	authapp "github.com/nikolaymatrosov/nvelope/internal/auth/app"
	authcommand "github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	authquery "github.com/nikolaymatrosov/nvelope/internal/auth/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/config"
	iamadapters "github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
	iamapp "github.com/nikolaymatrosov/nvelope/internal/iam/app"
	iamcommand "github.com/nikolaymatrosov/nvelope/internal/iam/app/command"
	iamquery "github.com/nikolaymatrosov/nvelope/internal/iam/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	tenantadapters "github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	tenantapp "github.com/nikolaymatrosov/nvelope/internal/tenant/app"
	tenantcommand "github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	tenantquery "github.com/nikolaymatrosov/nvelope/internal/tenant/app/query"
)

// Application is the fully wired use-case surface of the nvelope service — the
// auth and tenant contexts' command and query handlers.
type Application struct {
	Auth     authapp.Application
	Tenant   tenantapp.Application
	Audience audienceapp.Application
	IAM      iamapp.Application
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
			LocateWorkspace: decorator.ApplyQueryDecorators(
				tenantquery.NewLocateWorkspaceHandler(tenants), "LocateWorkspace", logger),
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

	audience := buildAudience(pool, cfg, logger)
	iam := buildIAM(pool, cfg, logger)

	return Application{Auth: auth, Tenant: tenant, Audience: audience, IAM: iam}
}

// buildIAM wires the iam context — tenant-plane users, sessions, roles,
// permissions, and the audit log — with logging decorators applied.
func buildIAM(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger) iamapp.Application {
	users := iamadapters.NewUsers(pool)
	sessions := iamadapters.NewSessions(pool)
	roles := iamadapters.NewRoles(pool)
	audit := iamadapters.NewAudit(pool)
	apiKeys := iamadapters.NewAPIKeys(pool)
	recoveryCodes := iamadapters.NewRecoveryCodes(pool)

	totpKey, err := hex.DecodeString(cfg.TOTPEncryptionKey)
	if err != nil {
		panic("decoding TOTP encryption key: " + err.Error())
	}
	totp, err := iamadapters.NewTOTP(totpKey)
	if err != nil {
		panic("building TOTP capability: " + err.Error())
	}

	return iamapp.Application{
		Commands: iamapp.Commands{
			CreateRole: decorator.ApplyResultCommandDecorators(
				iamcommand.NewCreateRoleHandler(roles, audit), "CreateRole", logger),
			UpdateRole: decorator.ApplyCommandDecorators(
				iamcommand.NewUpdateRoleHandler(roles, audit), "UpdateRole", logger),
			DeleteRole: decorator.ApplyCommandDecorators(
				iamcommand.NewDeleteRoleHandler(roles, audit), "DeleteRole", logger),
			AssignRole: decorator.ApplyCommandDecorators(
				iamcommand.NewAssignRoleHandler(roles, audit), "AssignRole", logger),
			AssignListRole: decorator.ApplyCommandDecorators(
				iamcommand.NewAssignListRoleHandler(roles, audit), "AssignListRole", logger),
			RevokeRole: decorator.ApplyCommandDecorators(
				iamcommand.NewRevokeRoleHandler(roles, audit), "RevokeRole", logger),
			OpenWorkspaceSession: decorator.ApplyResultCommandDecorators(
				iamcommand.NewOpenWorkspaceSessionHandler(users, sessions, roles, cfg.SessionTTL),
				"OpenWorkspaceSession", logger),
			CloseSession: decorator.ApplyCommandDecorators(
				iamcommand.NewCloseSessionHandler(sessions), "CloseSession", logger),
			IssueAPIKey: decorator.ApplyResultCommandDecorators(
				iamcommand.NewIssueAPIKeyHandler(apiKeys, audit), "IssueAPIKey", logger),
			RevokeAPIKey: decorator.ApplyCommandDecorators(
				iamcommand.NewRevokeAPIKeyHandler(apiKeys, audit), "RevokeAPIKey", logger),
			EnableTOTP: decorator.ApplyResultCommandDecorators(
				iamcommand.NewEnableTOTPHandler(totp), "EnableTOTP", logger),
			ConfirmTOTP: decorator.ApplyResultCommandDecorators(
				iamcommand.NewConfirmTOTPHandler(users, recoveryCodes, totp), "ConfirmTOTP", logger),
			DisableTOTP: decorator.ApplyCommandDecorators(
				iamcommand.NewDisableTOTPHandler(users, recoveryCodes), "DisableTOTP", logger),
			VerifyTOTPChallenge: decorator.ApplyCommandDecorators(
				iamcommand.NewVerifyTOTPChallengeHandler(sessions, users, recoveryCodes, totp),
				"VerifyTOTPChallenge", logger),
		},
		Queries: iamapp.Queries{
			AuthenticatePrincipal: decorator.ApplyQueryDecorators(
				iamquery.NewAuthenticatePrincipalHandler(sessions, roles), "AuthenticatePrincipal", logger),
			AuthenticateAPIKey: decorator.ApplyQueryDecorators(
				iamquery.NewAuthenticateAPIKeyHandler(apiKeys), "AuthenticateAPIKey", logger),
			ListRoles: decorator.ApplyQueryDecorators(
				iamquery.NewListRolesHandler(roles), "ListRoles", logger),
			ListAPIKeys: decorator.ApplyQueryDecorators(
				iamquery.NewListAPIKeysHandler(apiKeys), "ListAPIKeys", logger),
			AuditTrail: decorator.ApplyQueryDecorators(
				iamquery.NewAuditTrailHandler(audit), "AuditTrail", logger),
		},
	}
}

// buildAudience wires the audience context — lists, subscribers, memberships,
// and import/export jobs — with logging decorators applied to every handler.
func buildAudience(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger) audienceapp.Application {
	lists := audienceadapters.NewLists(pool)
	subscribers := audienceadapters.NewSubscribers(pool)
	memberships := audienceadapters.NewMemberships(pool)
	jobRepo := audienceadapters.NewJobs(pool)

	// The API service only enqueues jobs; the worker service consumes them.
	riverClient, err := jobs.NewInsertOnlyClient(pool)
	if err != nil {
		panic("building river client: " + err.Error())
	}
	enqueuer := jobs.NewEnqueuer(riverClient, cfg.WorkerQueue)

	return audienceapp.Application{
		Commands: audienceapp.Commands{
			CreateList: decorator.ApplyResultCommandDecorators(
				audiencecommand.NewCreateListHandler(lists), "CreateList", logger),
			UpdateList: decorator.ApplyCommandDecorators(
				audiencecommand.NewUpdateListHandler(lists), "UpdateList", logger),
			DeleteList: decorator.ApplyCommandDecorators(
				audiencecommand.NewDeleteListHandler(lists), "DeleteList", logger),
			CreateSubscriber: decorator.ApplyResultCommandDecorators(
				audiencecommand.NewCreateSubscriberHandler(subscribers, memberships), "CreateSubscriber", logger),
			UpdateSubscriber: decorator.ApplyCommandDecorators(
				audiencecommand.NewUpdateSubscriberHandler(subscribers), "UpdateSubscriber", logger),
			DeleteSubscriber: decorator.ApplyCommandDecorators(
				audiencecommand.NewDeleteSubscriberHandler(subscribers), "DeleteSubscriber", logger),
			AddToList: decorator.ApplyCommandDecorators(
				audiencecommand.NewAddToListHandler(memberships), "AddToList", logger),
			RemoveFromList: decorator.ApplyCommandDecorators(
				audiencecommand.NewRemoveFromListHandler(memberships), "RemoveFromList", logger),
			ChangeSubscriptionState: decorator.ApplyCommandDecorators(
				audiencecommand.NewChangeSubscriptionStateHandler(memberships), "ChangeSubscriptionState", logger),
			StartImport: decorator.ApplyResultCommandDecorators(
				audiencecommand.NewStartImportHandler(jobRepo, enqueuer), "StartImport", logger),
			StartExport: decorator.ApplyResultCommandDecorators(
				audiencecommand.NewStartExportHandler(jobRepo, enqueuer), "StartExport", logger),
		},
		Queries: audienceapp.Queries{
			ListLists: decorator.ApplyQueryDecorators(
				audiencequery.NewListListsHandler(lists), "ListLists", logger),
			GetList: decorator.ApplyQueryDecorators(
				audiencequery.NewGetListHandler(lists), "GetList", logger),
			SearchSubscribers: decorator.ApplyQueryDecorators(
				audiencequery.NewSearchSubscribersHandler(subscribers), "SearchSubscribers", logger),
			RunSegment: decorator.ApplyQueryDecorators(
				audiencequery.NewRunSegmentHandler(subscribers), "RunSegment", logger),
			GetSubscriber: decorator.ApplyQueryDecorators(
				audiencequery.NewGetSubscriberHandler(subscribers, memberships), "GetSubscriber", logger),
			GetJobStatus: decorator.ApplyQueryDecorators(
				audiencequery.NewGetJobStatusHandler(jobRepo), "GetJobStatus", logger),
			ExportFile: decorator.ApplyQueryDecorators(
				audiencequery.NewExportFileHandler(jobRepo), "ExportFile", logger),
		},
	}
}
