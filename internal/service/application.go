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
	campaignadapters "github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	campaignapp "github.com/nikolaymatrosov/nvelope/internal/campaign/app"
	campaigncommand "github.com/nikolaymatrosov/nvelope/internal/campaign/app/command"
	campaignquery "github.com/nikolaymatrosov/nvelope/internal/campaign/app/query"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/config"
	iamadapters "github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
	iamapp "github.com/nikolaymatrosov/nvelope/internal/iam/app"
	iamcommand "github.com/nikolaymatrosov/nvelope/internal/iam/app/command"
	iamquery "github.com/nikolaymatrosov/nvelope/internal/iam/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/platform/postbox"
	"github.com/nikolaymatrosov/nvelope/internal/platform/ratelimit"
	sendingadapters "github.com/nikolaymatrosov/nvelope/internal/sending/adapters"
	sendingapp "github.com/nikolaymatrosov/nvelope/internal/sending/app"
	sendingcommand "github.com/nikolaymatrosov/nvelope/internal/sending/app/command"
	sendingquery "github.com/nikolaymatrosov/nvelope/internal/sending/app/query"
	sendingdomain "github.com/nikolaymatrosov/nvelope/internal/sending/domain"
	tenantadapters "github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	tenantapp "github.com/nikolaymatrosov/nvelope/internal/tenant/app"
	tenantcommand "github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	tenantquery "github.com/nikolaymatrosov/nvelope/internal/tenant/app/query"
)

// Application is the fully wired use-case surface of the nvelope service — the
// auth, tenant, audience, iam, and sending contexts' command and query
// handlers.
type Application struct {
	Auth     authapp.Application
	Tenant   tenantapp.Application
	Audience audienceapp.Application
	IAM      iamapp.Application
	Sending  sendingapp.Application
	Campaign campaignapp.Application
	// Tracking is the campaign context's tracking repository, used directly by
	// the public open/click endpoints.
	Tracking campaigndomain.TrackingRepository
}

// overrides carries test-only substitutes for external integrations the
// composition root would otherwise build from configuration. Production code
// passes none; tests use them to swap a paid external service for a fake.
type overrides struct {
	sendingProvisioner sendingdomain.DomainProvisioner
	campaignMessenger  campaigndomain.Messenger
	campaignLimiter    campaigndomain.RateLimiter
}

// Option overrides a composition-root dependency. It exists so tests can
// substitute an external integration (the mail provider) with a fake.
type Option func(*overrides)

// WithSendingProvisioner substitutes the sending context's domain provisioner —
// used by tests to avoid calling the real mail provider.
func WithSendingProvisioner(p sendingdomain.DomainProvisioner) Option {
	return func(o *overrides) { o.sendingProvisioner = p }
}

// WithCampaignSender substitutes the campaign context's messenger and rate
// limiter — used by tests to avoid the real mail provider and Redis when
// exercising the synchronous transactional send.
func WithCampaignSender(messenger campaigndomain.Messenger, limiter campaigndomain.RateLimiter) Option {
	return func(o *overrides) {
		o.campaignMessenger = messenger
		o.campaignLimiter = limiter
	}
}

// NewApplication is the composition root. It constructs the pgx-backed
// adapters, builds every command and query handler with logging decorators
// applied, and returns the wired Application. Dependencies flow through plain
// constructors — there is no DI framework and no hidden global state.
func NewApplication(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger,
	opts ...Option) Application {

	var ov overrides
	for _, opt := range opts {
		opt(&ov)
	}
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
	sending := buildSending(pool, cfg, logger, ov)
	campaign, tracking := buildCampaign(pool, cfg, logger, ov)

	return Application{
		Auth: auth, Tenant: tenant, Audience: audience, IAM: iam,
		Sending: sending, Campaign: campaign, Tracking: tracking,
	}
}

// buildCampaign wires the campaign context's command and query handlers — the
// surface the API service uses — with logging decorators applied. It also
// returns the tracking repository, used directly by the public tracking
// endpoints. The send-pipeline workers are wired separately in cmd/worker.
func buildCampaign(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger, ov overrides) (
	campaignapp.Application, campaigndomain.TrackingRepository) {

	templates := campaignadapters.NewTemplates(pool)
	campaigns := campaignadapters.NewCampaigns(pool)
	tracking := campaignadapters.NewTracking(pool)
	lookup := NewSendingDomainLookup(sendingadapters.NewSendingDomains(pool))

	riverClient, err := jobs.NewInsertOnlyClient(pool)
	if err != nil {
		panic("building river client: " + err.Error())
	}
	enqueuer := jobs.NewSendEnqueuer(riverClient, cfg.WorkerSendQueue)

	// The synchronous transactional send needs a messenger and a rate limiter
	// inside the API process. Tests substitute both via WithCampaignSender.
	messenger := ov.campaignMessenger
	if messenger == nil {
		client, err := postbox.New(postbox.Config{
			Endpoint:        cfg.PostboxEndpoint,
			Region:          cfg.PostboxRegion,
			AccessKeyID:     cfg.PostboxAccessKeyID,
			SecretAccessKey: cfg.PostboxSecretAccessKey,
		})
		if err != nil {
			panic("building postbox client: " + err.Error())
		}
		messenger = campaignadapters.NewPostboxMessenger(client)
	}
	limiter := ov.campaignLimiter
	if limiter == nil {
		rl, err := ratelimit.New(cfg.RedisURL, ratelimit.Limit{
			Max:    cfg.GlobalSendRateLimit,
			Window: cfg.GlobalSendRateWindow,
		})
		if err != nil {
			panic("building rate limiter: " + err.Error())
		}
		limiter = campaignadapters.NewRateLimiter(rl)
	}
	perTenant := campaigndomain.Limit{
		Max:    cfg.DefaultTenantSendRateLimit,
		Window: cfg.DefaultTenantSendRateWindow,
	}

	app := campaignapp.Application{
		Commands: campaignapp.Commands{
			CreateTemplate: decorator.ApplyResultCommandDecorators(
				campaigncommand.NewCreateTemplateHandler(templates), "CreateTemplate", logger),
			UpdateTemplate: decorator.ApplyCommandDecorators(
				campaigncommand.NewUpdateTemplateHandler(templates), "UpdateTemplate", logger),
			DeleteTemplate: decorator.ApplyCommandDecorators(
				campaigncommand.NewDeleteTemplateHandler(templates), "DeleteTemplate", logger),
			CreateCampaign: decorator.ApplyResultCommandDecorators(
				campaigncommand.NewCreateCampaignHandler(campaigns, templates), "CreateCampaign", logger),
			UpdateCampaign: decorator.ApplyCommandDecorators(
				campaigncommand.NewUpdateCampaignHandler(campaigns), "UpdateCampaign", logger),
			StartCampaign: decorator.ApplyCommandDecorators(
				campaigncommand.NewStartCampaignHandler(campaigns, lookup, enqueuer), "StartCampaign", logger),
			PauseCampaign: decorator.ApplyCommandDecorators(
				campaigncommand.NewPauseCampaignHandler(campaigns), "PauseCampaign", logger),
			ResumeCampaign: decorator.ApplyCommandDecorators(
				campaigncommand.NewResumeCampaignHandler(campaigns, enqueuer), "ResumeCampaign", logger),
			CancelCampaign: decorator.ApplyCommandDecorators(
				campaigncommand.NewCancelCampaignHandler(campaigns), "CancelCampaign", logger),
			SendTransactional: decorator.ApplyResultCommandDecorators(
				campaigncommand.NewSendTransactionalHandler(templates, lookup, messenger, limiter, perTenant),
				"SendTransactional", logger),
		},
		Queries: campaignapp.Queries{
			ListTemplates: decorator.ApplyQueryDecorators(
				campaignquery.NewListTemplatesHandler(templates), "ListTemplates", logger),
			GetTemplate: decorator.ApplyQueryDecorators(
				campaignquery.NewGetTemplateHandler(templates), "GetTemplate", logger),
			ListCampaigns: decorator.ApplyQueryDecorators(
				campaignquery.NewListCampaignsHandler(campaigns), "ListCampaigns", logger),
			GetCampaign: decorator.ApplyQueryDecorators(
				campaignquery.NewGetCampaignHandler(campaigns), "GetCampaign", logger),
		},
	}
	return app, tracking
}

// buildSending wires the sending context — sending domains and their
// Postbox-backed verification — with logging decorators applied. When a test
// supplies a provisioner override the real Postbox client is not built.
func buildSending(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger,
	ov overrides) sendingapp.Application {

	domains := sendingadapters.NewSendingDomains(pool)

	provisioner := ov.sendingProvisioner
	if provisioner == nil {
		client, err := postbox.New(postbox.Config{
			Endpoint:        cfg.PostboxEndpoint,
			Region:          cfg.PostboxRegion,
			AccessKeyID:     cfg.PostboxAccessKeyID,
			SecretAccessKey: cfg.PostboxSecretAccessKey,
		})
		if err != nil {
			panic("building postbox client: " + err.Error())
		}
		provisioner = sendingadapters.NewPostboxProvisioner(client)
	}

	riverClient, err := jobs.NewInsertOnlyClient(pool)
	if err != nil {
		panic("building river client: " + err.Error())
	}
	enqueuer := jobs.NewSendEnqueuer(riverClient, cfg.WorkerSendQueue)

	return sendingapp.Application{
		Commands: sendingapp.Commands{
			AddDomain: decorator.ApplyResultCommandDecorators(
				sendingcommand.NewAddDomainHandler(domains, provisioner, enqueuer), "AddDomain", logger),
			RecheckDomain: decorator.ApplyCommandDecorators(
				sendingcommand.NewRecheckDomainHandler(domains, enqueuer), "RecheckDomain", logger),
		},
		Queries: sendingapp.Queries{
			ListDomains: decorator.ApplyQueryDecorators(
				sendingquery.NewListDomainsHandler(domains), "ListDomains", logger),
			GetDomain: decorator.ApplyQueryDecorators(
				sendingquery.NewGetDomainHandler(domains), "GetDomain", logger),
		},
	}
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
