package service

import (
	"context"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	audienceadapters "github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	audienceapp "github.com/nikolaymatrosov/nvelope/internal/audience/app"
	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	audiencedomain "github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	authadapters "github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	authapp "github.com/nikolaymatrosov/nvelope/internal/auth/app"
	authcommand "github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	authquery "github.com/nikolaymatrosov/nvelope/internal/auth/app/query"
	billingadapters "github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	billingapp "github.com/nikolaymatrosov/nvelope/internal/billing/app"
	billingcommand "github.com/nikolaymatrosov/nvelope/internal/billing/app/command"
	billingquery "github.com/nikolaymatrosov/nvelope/internal/billing/app/query"
	billingdomain "github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	campaignadapters "github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	campaignapp "github.com/nikolaymatrosov/nvelope/internal/campaign/app"
	campaigncommand "github.com/nikolaymatrosov/nvelope/internal/campaign/app/command"
	campaignquery "github.com/nikolaymatrosov/nvelope/internal/campaign/app/query"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/config"
	deliverabilityadapters "github.com/nikolaymatrosov/nvelope/internal/deliverability/adapters"
	deliverabilityapp "github.com/nikolaymatrosov/nvelope/internal/deliverability/app"
	deliverabilitycommand "github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	deliverabilityquery "github.com/nikolaymatrosov/nvelope/internal/deliverability/app/query"
	iamadapters "github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
	iamapp "github.com/nikolaymatrosov/nvelope/internal/iam/app"
	iamcommand "github.com/nikolaymatrosov/nvelope/internal/iam/app/command"
	iamquery "github.com/nikolaymatrosov/nvelope/internal/iam/app/query"
	mediaadapters "github.com/nikolaymatrosov/nvelope/internal/media/adapters"
	mediaapp "github.com/nikolaymatrosov/nvelope/internal/media/app"
	mediacommand "github.com/nikolaymatrosov/nvelope/internal/media/app/command"
	mediaquery "github.com/nikolaymatrosov/nvelope/internal/media/app/query"
	mediadomain "github.com/nikolaymatrosov/nvelope/internal/media/domain"
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
	Auth           authapp.Application
	Tenant         tenantapp.Application
	Audience       audienceapp.Application
	IAM            iamapp.Application
	Sending        sendingapp.Application
	Campaign       campaignapp.Application
	Deliverability deliverabilityapp.Application
	Billing        billingapp.Application
	Media          mediaapp.Application
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
	billingGateway     billingdomain.PaymentGateway
	mediaBlobStore     mediadomain.BlobStore
}

// WithMediaBlobStore substitutes the media context's blob store — used by
// tests to avoid booting an object store.
func WithMediaBlobStore(b mediadomain.BlobStore) Option {
	return func(o *overrides) { o.mediaBlobStore = b }
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

// WithBillingGateway substitutes the billing context's payment gateway — used
// by tests to program deterministic decline and error outcomes.
func WithBillingGateway(g billingdomain.PaymentGateway) Option {
	return func(o *overrides) { o.billingGateway = g }
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
	branding := tenantadapters.NewBranding(pool)

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
			SaveBranding: decorator.ApplyCommandDecorators(
				tenantcommand.NewSaveBrandingHandler(branding), "SaveBranding", logger),
		},
		Queries: tenantapp.Queries{
			ListWorkspaces: decorator.ApplyQueryDecorators(
				tenantquery.NewListWorkspacesHandler(tenants), "ListWorkspaces", logger),
			ResolveWorkspace: decorator.ApplyQueryDecorators(
				tenantquery.NewResolveWorkspaceHandler(tenants), "ResolveWorkspace", logger),
			LocateWorkspace: decorator.ApplyQueryDecorators(
				tenantquery.NewLocateWorkspaceHandler(tenants), "LocateWorkspace", logger),
			LocateWorkspaceByID: decorator.ApplyQueryDecorators(
				tenantquery.NewLocateWorkspaceByIDHandler(tenants), "LocateWorkspaceByID", logger),
			WorkspaceMembers: decorator.ApplyQueryDecorators(
				tenantquery.NewWorkspaceMembersHandler(tenants), "WorkspaceMembers", logger),
			GetSettings: decorator.ApplyQueryDecorators(
				tenantquery.NewGetSettingsHandler(settings), "GetSettings", logger),
			PendingInvitations: decorator.ApplyQueryDecorators(
				tenantquery.NewPendingInvitationsHandler(invitations), "PendingInvitations", logger),
			LookUpInvitation: decorator.ApplyQueryDecorators(
				tenantquery.NewLookUpInvitationHandler(invitations, tenants), "LookUpInvitation", logger),
			GetBranding: decorator.ApplyQueryDecorators(
				tenantquery.NewGetBrandingHandler(branding), "GetBranding", logger),
		},
	}

	audience := buildAudience(pool, cfg, logger)
	iam := buildIAM(pool, cfg, logger)
	sending := buildSending(pool, cfg, logger, ov)
	campaign, tracking := buildCampaign(pool, cfg, logger, ov)
	deliverability := buildDeliverability(pool, cfg, logger)
	billing := buildBilling(pool, cfg, logger, ov)
	media := buildMedia(pool, cfg, logger, ov)

	return Application{
		Auth: auth, Tenant: tenant, Audience: audience, IAM: iam,
		Sending: sending, Campaign: campaign, Deliverability: deliverability,
		Billing: billing, Media: media, Tracking: tracking,
	}
}

// buildMedia wires the media context — tenant media library uploads, listing,
// and deletion — with logging decorators applied. The blob store is built from
// configuration; tests substitute it via WithMediaBlobStore.
func buildMedia(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger,
	ov overrides) mediaapp.Application {

	assets := mediaadapters.NewAssets(pool)

	blobs := ov.mediaBlobStore
	if blobs == nil {
		s3blobs, err := mediaadapters.NewS3BlobStore(mediaadapters.S3Config{
			Endpoint:        cfg.ObjectStorageEndpoint,
			Region:          cfg.ObjectStorageRegion,
			Bucket:          cfg.ObjectStorageBucket,
			AccessKeyID:     cfg.ObjectStorageAccessKeyID,
			SecretAccessKey: cfg.ObjectStorageSecretAccessKey,
			PublicBaseURL:   cfg.ObjectStoragePublicBaseURL,
		})
		if err != nil {
			panic("building s3 blob store: " + err.Error())
		}
		blobs = s3blobs
	}

	return mediaapp.Application{
		Commands: mediaapp.Commands{
			UploadAsset: decorator.ApplyResultCommandDecorators(
				mediacommand.NewUploadAssetHandler(assets, blobs, cfg.MediaMaxBytes),
				"UploadAsset", logger),
			DeleteAsset: decorator.ApplyCommandDecorators(
				mediacommand.NewDeleteAssetHandler(assets, blobs), "DeleteAsset", logger),
		},
		Queries: mediaapp.Queries{
			ListAssets: decorator.ApplyQueryDecorators(
				mediaquery.NewListAssetsHandler(assets), "ListAssets", logger),
		},
	}
}

// buildBilling wires the billing context — plans, subscriptions, invoicing, and
// the synchronous first charge — with logging decorators applied. The billing
// workers (sweep, charge, rollup) are wired separately in cmd/worker; the
// scheduler ticks in cmd/scheduler.
func buildBilling(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger,
	ov overrides) billingapp.Application {
	plans := billingadapters.NewPlans(pool)
	subscriptions := billingadapters.NewSubscriptions(pool)
	invoices := billingadapters.NewInvoices(pool)
	usage := billingadapters.NewUsage(pool)
	audit := billingadapters.NewAuditLog(pool)

	gateway := ov.billingGateway
	if gateway == nil {
		gateway = billingadapters.NewMockGateway()
	}

	dunning := billingdomain.NewDunningPolicy(cfg.DunningMaxAttempts, cfg.DunningRetryInterval)
	charge := billingcommand.NewChargeInvoiceHandler(subscriptions, invoices, plans, gateway, dunning)
	subscribe := billingcommand.NewSubscribeHandler(plans, subscriptions, invoices, charge, audit)
	cancel := billingcommand.NewCancelSubscriptionHandler(subscriptions, audit)
	settle := billingcommand.NewSettleInvoiceHandler(charge, audit)

	return billingapp.Application{
		Commands: billingapp.Commands{
			Subscribe: decorator.ApplyResultCommandDecorators(subscribe, "Subscribe", logger),
			CancelSubscription: decorator.ApplyCommandDecorators(
				cancel, "CancelSubscription", logger),
			SettleInvoice: decorator.ApplyResultCommandDecorators(
				settle, "SettleInvoice", logger),
		},
		Queries: billingapp.Queries{
			ListPlans: decorator.ApplyQueryDecorators(
				billingquery.NewListPlansHandler(plans), "ListPlans", logger),
			GetSubscription: decorator.ApplyQueryDecorators(
				billingquery.NewGetSubscriptionHandler(subscriptions, plans, usage),
				"GetSubscription", logger),
			ListInvoices: decorator.ApplyQueryDecorators(
				billingquery.NewListInvoicesHandler(invoices), "ListInvoices", logger),
			GetInvoice: decorator.ApplyQueryDecorators(
				billingquery.NewGetInvoiceHandler(invoices), "GetInvoice", logger),
		},
	}
}

// buildDeliverability wires the deliverability context — inbound webhook
// ingestion, suppression, and analytics — with logging decorators applied. The
// feedback.process and analytics.refresh workers are wired separately in
// cmd/worker; the scheduler tick in cmd/scheduler.
func buildDeliverability(pool *pgxpool.Pool, cfg config.Config,
	logger *slog.Logger) deliverabilityapp.Application {

	events := deliverabilityadapters.NewEvents(pool)
	parser := deliverabilityadapters.NewNotificationParser()
	suppressions := deliverabilityadapters.NewSuppressions(pool)
	settings := deliverabilityadapters.NewSettings(pool)
	analytics := deliverabilityadapters.NewAnalytics(pool)
	audit := deliverabilityadapters.NewAuditLog(pool)

	riverClient, err := jobs.NewInsertOnlyClient(pool)
	if err != nil {
		panic("building river client: " + err.Error())
	}
	enqueuer := jobs.NewSendEnqueuer(riverClient, cfg.WorkerSendQueue)

	return deliverabilityapp.Application{
		Commands: deliverabilityapp.Commands{
			IngestNotification: decorator.ApplyCommandDecorators(
				deliverabilitycommand.NewIngestNotificationHandler(parser, events, enqueuer),
				"IngestNotification", logger),
			AddSuppression: decorator.ApplyCommandDecorators(
				deliverabilitycommand.NewAddSuppressionHandler(suppressions, audit),
				"AddSuppression", logger),
			RemoveSuppression: decorator.ApplyCommandDecorators(
				deliverabilitycommand.NewRemoveSuppressionHandler(suppressions, audit),
				"RemoveSuppression", logger),
			UpdateBounceSettings: decorator.ApplyCommandDecorators(
				deliverabilitycommand.NewUpdateBounceSettingsHandler(settings, audit),
				"UpdateBounceSettings", logger),
		},
		Queries: deliverabilityapp.Queries{
			ListSuppressions: decorator.ApplyQueryDecorators(
				deliverabilityquery.NewListSuppressionsHandler(suppressions),
				"ListSuppressions", logger),
			GetBounceSettings: decorator.ApplyQueryDecorators(
				deliverabilityquery.NewGetBounceSettingsHandler(settings),
				"GetBounceSettings", logger),
			GetCampaignAnalytics: decorator.ApplyQueryDecorators(
				deliverabilityquery.NewGetCampaignAnalyticsHandler(analytics),
				"GetCampaignAnalytics", logger),
			GetDashboard: decorator.ApplyQueryDecorators(
				deliverabilityquery.NewGetDashboardHandler(analytics),
				"GetDashboard", logger),
		},
	}
}

// fieldsProvider adapts the audience FieldRepository plus the package-level
// built-in pseudo-rows to the consumer-owned FieldsProvider interface the
// save_visual_{campaign,template} commands depend on.
type fieldsProvider struct {
	repo audiencedomain.FieldRepository
}

func (p fieldsProvider) AllSlugs(ctx context.Context, tenantID string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, b := range audiencedomain.BuiltinFields() {
		out[b.Slug()] = true
	}
	custom, err := p.repo.All(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	for _, f := range custom {
		out[f.Slug()] = true
	}
	return out, nil
}

// tenantMediaRefValidator answers the FR-021 media-ref check by prefix
// match against the platform's media base URL. The save command builds one
// per request from the tenant id so each tenant's images must live under
// /tenants/<their tenant id>/.
type tenantMediaRefValidator struct{ prefix string }

func (v tenantMediaRefValidator) IsTenantMediaRef(ref string) bool {
	if v.prefix == "" || ref == "" {
		return false
	}
	return strings.HasPrefix(ref, v.prefix)
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
	txMessages := campaignadapters.NewTransactionalMessages(pool)
	lookup := NewSendingDomainLookup(sendingadapters.NewSendingDomains(pool))

	// Visual-editor save dependencies — the BFF posts pre-rendered HTML/text
	// to PUT /campaigns/{id}/visual and the save command revalidates the doc
	// against the merged subscriber-field slug set before sanitizing + persisting.
	fields := fieldsProvider{repo: audienceadapters.NewFields(pool)}
	mediaRefs := tenantMediaRefValidator{prefix: cfg.ObjectStoragePublicBaseURL}

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

	// The billing gates meter and quota-check every transactional send.
	usageRecorder := billingadapters.NewUsageRecorder(
		billingadapters.NewSubscriptions(pool), billingadapters.NewUsage(pool))
	quotaGate := billingadapters.NewQuotaGate(billingadapters.NewSubscriptions(pool),
		billingadapters.NewPlans(pool), billingadapters.NewUsage(pool))

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
				campaigncommand.NewSendTransactionalHandler(templates, lookup, messenger, limiter,
					txMessages, deliverabilityadapters.NewSuppressionChecker(pool), usageRecorder,
					quotaGate, perTenant),
				"SendTransactional", logger),
			SetArchiveVisibility: decorator.ApplyCommandDecorators(
				campaigncommand.NewSetArchiveVisibilityHandler(campaigns),
				"SetArchiveVisibility", logger),
			SaveVisualCampaign: decorator.ApplyResultCommandDecorators(
				campaigncommand.NewSaveVisualCampaignHandler(campaigns, fields, mediaRefs),
				"SaveVisualCampaign", logger),
			SaveVisualTemplate: decorator.ApplyResultCommandDecorators(
				campaigncommand.NewSaveVisualTemplateHandler(templates, fields, mediaRefs),
				"SaveVisualTemplate", logger),
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
			ListArchive: decorator.ApplyQueryDecorators(
				campaignquery.NewListArchiveHandler(campaigns), "ListArchive", logger),
			GetArchivedCampaign: decorator.ApplyQueryDecorators(
				campaignquery.NewGetArchivedCampaignHandler(campaigns), "GetArchivedCampaign", logger),
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

// optinThrottleMax and optinThrottleWindow bound how often a public
// subscription form may be submitted for one address or source, so the form
// cannot be used to flood an inbox with confirmation mail.
const (
	optinThrottleMax    = 5
	optinThrottleWindow = 10 * time.Minute
)

// buildAudience wires the audience context — lists, subscribers, memberships,
// import/export jobs, and the Phase 6 public subscription pages — with logging
// decorators applied to every handler.
func buildAudience(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger) audienceapp.Application {
	lists := audienceadapters.NewLists(pool)
	subscribers := audienceadapters.NewSubscribers(pool)
	memberships := audienceadapters.NewMemberships(pool)
	jobRepo := audienceadapters.NewJobs(pool)
	subscriptionPages := audienceadapters.NewSubscriptionPages(pool)
	pendingSubscriptions := audienceadapters.NewPendingSubscriptions(pool)
	fields := audienceadapters.NewFields(pool)

	// The API service only enqueues jobs; the worker service consumes them.
	riverClient, err := jobs.NewInsertOnlyClient(pool)
	if err != nil {
		panic("building river client: " + err.Error())
	}
	enqueuer := jobs.NewEnqueuer(riverClient, cfg.WorkerQueue)
	// Double-opt-in confirmation emails ride the sending queue.
	optinEnqueuer := jobs.NewSendEnqueuer(riverClient, cfg.WorkerSendQueue)

	// The submission throttle needs Redis. Production config always supplies a
	// Redis DSN (config.Validate requires it); a config without one — only
	// constructed directly in tests — gets a permissive no-op throttle.
	var throttle audiencedomain.SubmissionThrottle = allowAllThrottle{}
	if cfg.RedisURL != "" {
		redisThrottle, err := audienceadapters.NewSubmissionThrottle(cfg.RedisURL,
			optinThrottleMax, optinThrottleWindow)
		if err != nil {
			panic("building submission throttle: " + err.Error())
		}
		throttle = redisThrottle
	}
	sendingDomains := sendingadapters.NewSendingDomains(pool)
	domainCheck := NewSendingDomainOwnership(sendingDomains)
	suppression := deliverabilityadapters.NewSuppressionChecker(pool)

	// Production config always supplies a confirmation TTL (config.Validate
	// requires a positive value); a directly-constructed test config may not.
	optinTTL := cfg.OptinConfirmationTTL
	if optinTTL <= 0 {
		optinTTL = 168 * time.Hour
	}

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
			SaveSubscriptionPage: decorator.ApplyResultCommandDecorators(
				audiencecommand.NewSaveSubscriptionPageHandler(subscriptionPages, lists, domainCheck),
				"SaveSubscriptionPage", logger),
			SubmitPublicSubscription: decorator.ApplyCommandDecorators(
				audiencecommand.NewSubmitPublicSubscriptionHandler(subscriptionPages,
					pendingSubscriptions, throttle, optinEnqueuer, optinTTL),
				"SubmitPublicSubscription", logger),
			ConfirmSubscription: decorator.ApplyResultCommandDecorators(
				audiencecommand.NewConfirmSubscriptionHandler(pendingSubscriptions, subscribers,
					memberships, suppression),
				"ConfirmSubscription", logger),
			ResendConfirmation: decorator.ApplyCommandDecorators(
				audiencecommand.NewResendConfirmationHandler(pendingSubscriptions, optinEnqueuer,
					optinTTL),
				"ResendConfirmation", logger),
			UpdatePreferences: decorator.ApplyCommandDecorators(
				audiencecommand.NewUpdatePreferencesHandler(subscribers, memberships),
				"UpdatePreferences", logger),
			PublicUnsubscribe: decorator.ApplyCommandDecorators(
				audiencecommand.NewPublicUnsubscribeHandler(memberships),
				"PublicUnsubscribe", logger),
			CreateField: decorator.ApplyResultCommandDecorators(
				audiencecommand.NewCreateFieldHandler(fields), "CreateField", logger),
			UpdateField: decorator.ApplyCommandDecorators(
				audiencecommand.NewUpdateFieldHandler(fields), "UpdateField", logger),
			DeleteField: decorator.ApplyCommandDecorators(
				audiencecommand.NewDeleteFieldHandler(fields), "DeleteField", logger),
			ReorderFields: decorator.ApplyCommandDecorators(
				audiencecommand.NewReorderFieldsHandler(fields), "ReorderFields", logger),
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
			GetSubscriptionPage: decorator.ApplyQueryDecorators(
				audiencequery.NewGetSubscriptionPageHandler(subscriptionPages),
				"GetSubscriptionPage", logger),
			ListSubscriptionPages: decorator.ApplyQueryDecorators(
				audiencequery.NewListSubscriptionPagesHandler(subscriptionPages),
				"ListSubscriptionPages", logger),
			GetPendingByToken: decorator.ApplyQueryDecorators(
				audiencequery.NewGetPendingByTokenHandler(pendingSubscriptions),
				"GetPendingByToken", logger),
			GetPreferences: decorator.ApplyQueryDecorators(
				audiencequery.NewGetPreferencesHandler(subscribers, memberships, lists),
				"GetPreferences", logger),
			ListFields: decorator.ApplyQueryDecorators(
				audiencequery.NewListFieldsHandler(fields), "ListFields", logger),
			GetField: decorator.ApplyQueryDecorators(
				audiencequery.NewGetFieldHandler(fields), "GetField", logger),
		},
	}
}
