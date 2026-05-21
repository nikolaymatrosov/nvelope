package api

import (
	"encoding/hex"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	audienceapp "github.com/nikolaymatrosov/nvelope/internal/audience/app"
	authapp "github.com/nikolaymatrosov/nvelope/internal/auth/app"
	billingapp "github.com/nikolaymatrosov/nvelope/internal/billing/app"
	campaignapp "github.com/nikolaymatrosov/nvelope/internal/campaign/app"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/config"
	deliverabilityapp "github.com/nikolaymatrosov/nvelope/internal/deliverability/app"
	iamapp "github.com/nikolaymatrosov/nvelope/internal/iam/app"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	mediaapp "github.com/nikolaymatrosov/nvelope/internal/media/app"
	"github.com/nikolaymatrosov/nvelope/internal/platform/metrics"
	sendingapp "github.com/nikolaymatrosov/nvelope/internal/sending/app"
	tenantapp "github.com/nikolaymatrosov/nvelope/internal/tenant/app"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// Server wires the nvelope HTTP API: the wired Application, configuration, and
// the route table. It holds no database handle — every request flows through
// the command and query handlers.
type Server struct {
	auth           authapp.Application
	tenant         tenantapp.Application
	audience       audienceapp.Application
	iam            iamapp.Application
	sending        sendingapp.Application
	campaign       campaignapp.Application
	deliverability deliverabilityapp.Application
	billing        billingapp.Application
	media          mediaapp.Application
	tracking       campaigndomain.TrackingRepository
	audit          iamdomain.AuditRepository
	cfg            config.Config
	logger         *slog.Logger
	health         http.Handler
	// prefSigner verifies the stateless preference / one-click-unsubscribe
	// tokens minted by the campaign send path.
	prefSigner token.Signer
}

// New returns a Server. The context applications are built by the composition
// root; tracking is the campaign context's tracking repository, used directly
// by the public open/click endpoints. The health handler is supplied by the
// caller so it can also toggle readiness during startup and graceful shutdown.
func New(auth authapp.Application, tenant tenantapp.Application, audience audienceapp.Application,
	iam iamapp.Application, sending sendingapp.Application, campaign campaignapp.Application,
	deliverability deliverabilityapp.Application, billing billingapp.Application,
	media mediaapp.Application, tracking campaigndomain.TrackingRepository,
	audit iamdomain.AuditRepository,
	cfg config.Config, logger *slog.Logger, health http.Handler) *Server {
	// The preference-token signer shares the TOTP encryption key, derived to a
	// distinct purpose. A malformed key cannot occur in production (config
	// validation rejects it) and yields a signer that verifies nothing.
	signKey, _ := hex.DecodeString(cfg.TOTPEncryptionKey)
	return &Server{
		auth: auth, tenant: tenant, audience: audience, iam: iam, sending: sending,
		campaign: campaign, deliverability: deliverability, billing: billing,
		media: media, tracking: tracking, audit: audit,
		cfg: cfg, logger: logger, health: health,
		prefSigner: token.NewSigner(signKey),
	}
}

// Handler builds the chi router with every route mounted.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(s.requestID)

	r.Method(http.MethodGet, "/healthz", s.health)
	r.Method(http.MethodGet, "/metrics", metrics.Handler())

	// Platform API — not scoped to a tenant.
	r.Route("/api/platform", func(r chi.Router) {
		r.Post("/signup", s.handleSignup)
		r.Post("/login", s.handleLogin)
		r.Get("/invitations/{token}", s.handleGetInvitation)
		r.Post("/invitations/{token}/accept", s.handleAcceptInvitation)

		r.Group(func(r chi.Router) {
			r.Use(s.requireUser)
			r.Post("/logout", s.handleLogout)
			r.Get("/me", s.handleMe)
			r.Post("/tenants", s.handleCreateTenant)
			r.Get("/tenants", s.handleListTenants)
		})
	})

	// Tenant-scoped API — every route passes through authentication and the
	// tenant-resolution + membership cross-check middleware.
	r.Route("/t/{slug}/api", func(r chi.Router) {
		r.Use(s.requireUser)
		r.Use(s.resolveTenant)

		r.Get("/tenant", s.handleTenantInfo)
		r.Get("/settings", s.handleGetSettings)
		r.Put("/settings", s.handleUpdateSettings)
		r.Post("/invitations", s.handleCreateInvitation)
		r.Get("/invitations", s.handleListInvitations)
		r.Delete("/invitations/{id}", s.handleRevokeInvitation)

		// Workspace session — opens/closes the tenant-plane session; no
		// Principal is required since these establish one.
		r.Post("/session", s.handleOpenSession)
		r.Delete("/session", s.handleCloseSession)
		// The TOTP challenge lives here too: a totp-pending session resolves
		// to no Principal, so it cannot pass the authz middleware.
		r.Post("/session/totp", s.handleVerifyTOTP)

		// Guarded routes — the authz middleware resolves the request's
		// Principal; handlers then enforce permissions.
		r.Group(func(r chi.Router) {
			r.Use(s.authz)

			// Audience — lists and subscribers (US1).
			r.Post("/lists", s.handleCreateList)
			r.Get("/lists", s.handleListLists)
			r.Get("/lists/{id}", s.handleGetList)
			r.Put("/lists/{id}", s.handleUpdateList)
			r.Delete("/lists/{id}", s.handleDeleteList)

			r.Post("/subscribers", s.handleCreateSubscriber)
			r.Get("/subscribers", s.handleSearchSubscribers)
			r.Post("/subscribers/query", s.handleQuerySubscribers)
			r.Post("/subscribers/query/count", s.handleCountSubscribers)
			r.Get("/subscribers/{id}", s.handleGetSubscriber)
			r.Put("/subscribers/{id}", s.handleUpdateSubscriber)
			r.Delete("/subscribers/{id}", s.handleDeleteSubscriber)
			r.Post("/subscribers/{id}/lists", s.handleAddToList)
			r.Delete("/subscribers/{id}/lists/{listId}", s.handleRemoveFromList)
			r.Put("/subscribers/{id}/lists/{listId}", s.handleChangeSubscription)

			// Import & export (US3).
			r.Post("/import", s.handleStartImport)
			r.Post("/export", s.handleStartExport)
			r.Get("/jobs/{id}", s.handleJobStatus)
			r.Get("/jobs/{id}/download", s.handleDownloadExport)

			// RBAC — role management (US2).
			r.Post("/roles", s.handleCreateRole)
			r.Get("/roles", s.handleListRoles)
			r.Put("/roles/{id}", s.handleUpdateRole)
			r.Delete("/roles/{id}", s.handleDeleteRole)
			r.Put("/users/{userId}/role", s.handleAssignRole)
			r.Put("/users/{userId}/lists/{listId}/role", s.handleAssignListRole)
			r.Delete("/users/{userId}/lists/{listId}/role", s.handleRemoveListRole)

			// Scoped API keys (US5).
			r.Post("/api-keys", s.handleIssueAPIKey)
			r.Get("/api-keys", s.handleListAPIKeys)
			r.Delete("/api-keys/{id}", s.handleRevokeAPIKey)

			// TOTP two-factor enrolment (US5) — requires an active session.
			r.Post("/me/totp", s.handleEnableTOTP)
			r.Post("/me/totp/confirm", s.handleConfirmTOTP)
			r.Delete("/me/totp", s.handleDisableTOTP)

			// Audit trail.
			r.Get("/audit", s.handleAuditTrail)

			// Sending domains (Phase 3 US1).
			r.Post("/sending-domains", s.handleAddSendingDomain)
			r.Get("/sending-domains", s.handleListSendingDomains)
			r.Get("/sending-domains/{id}", s.handleGetSendingDomain)
			r.Post("/sending-domains/{id}/recheck", s.handleRecheckSendingDomain)

			// Templates & campaigns (Phase 3 US2).
			r.Post("/templates", s.handleCreateTemplate)
			r.Get("/templates", s.handleListTemplates)
			r.Get("/templates/{id}", s.handleGetTemplate)
			r.Put("/templates/{id}", s.handleUpdateTemplate)
			r.Put("/templates/{id}/visual", s.handleSaveVisualTemplate)
			r.Post("/templates/{id}/convert-to-visual", s.handleConvertTemplateToVisual)
			r.Post("/templates/{id}/opt-out-visual", s.handleOptOutTemplateVisual)
			r.Delete("/templates/{id}", s.handleDeleteTemplate)
			r.Post("/campaigns", s.handleCreateCampaign)
			r.Get("/campaigns", s.handleListCampaigns)
			r.Get("/campaigns/{id}", s.handleGetCampaign)
			r.Put("/campaigns/{id}", s.handleUpdateCampaign)
			r.Put("/campaigns/{id}/visual", s.handleSaveVisualCampaign)
			r.Post("/campaigns/{id}/convert-to-visual", s.handleConvertCampaignToVisual)
			r.Post("/campaigns/{id}/opt-out-visual", s.handleOptOutCampaignVisual)
			r.Post("/campaigns/{id}/start", s.handleStartCampaign)
			r.Post("/campaigns/{id}/pause", s.handlePauseCampaign)
			r.Post("/campaigns/{id}/resume", s.handleResumeCampaign)
			r.Post("/campaigns/{id}/cancel", s.handleCancelCampaign)

			// Suppression list & bounce settings (Phase 4 US2).
			r.Get("/suppressions", s.handleListSuppressions)
			r.Post("/suppressions", s.handleAddSuppression)
			r.Delete("/suppressions/{email}", s.handleRemoveSuppression)
			r.Get("/bounce-settings", s.handleGetBounceSettings)
			r.Put("/bounce-settings", s.handleUpdateBounceSettings)

			// Campaign analytics & workspace dashboard (Phase 4 US3).
			r.Get("/campaigns/{id}/analytics", s.handleCampaignAnalytics)
			r.Get("/dashboard", s.handleDashboard)

			// Public subscription pages — admin configuration (Phase 6 US1).
			r.Get("/subscription-pages", s.handleListSubscriptionPages)
			r.Post("/subscription-pages", s.handleCreateSubscriptionPage)
			r.Put("/subscription-pages/{id}", s.handleUpdateSubscriptionPage)

			// Tenant branding & campaign archive toggle (Phase 6 US3).
			r.Get("/branding", s.handleGetBranding)
			r.Put("/branding", s.handleSaveBranding)
			r.Post("/campaigns/{id}/archive", s.handleSetCampaignArchive)

			// Tenant media library (Phase 6 US4).
			r.Get("/media", s.handleListMedia)
			r.Post("/media", s.handleUploadMedia)
			r.Delete("/media/{id}", s.handleDeleteMedia)

			// Visual-editor subscriber-field registry + merge-tag picker
			// (Phase 7 US1). The /order route is registered before the
			// /{id} route so chi's radix tree treats "order" as a literal
			// segment rather than a path parameter.
			r.Get("/subscriber-fields", s.handleListSubscriberFields)
			r.Post("/subscriber-fields", s.handleCreateSubscriberField)
			r.Patch("/subscriber-fields/order", s.handleReorderSubscriberFields)
			r.Patch("/subscriber-fields/{id}", s.handleUpdateSubscriberField)
			r.Delete("/subscriber-fields/{id}", s.handleDeleteSubscriberField)
			r.Get("/merge-tags", s.handleListMergeTags)

			// BFF→Go helper for sample-data placeholder substitution
			// (Phase 7 US1). Reached only by the BFF's render-preview
			// route; routes through the canonical send-pipeline
			// substituter so preview matches inbox.
			r.Post("/substitute-sample", s.handleSubstituteSample)

			// Billing — plans, subscription, invoices (Phase 5).
			r.Get("/plans", s.handleListPlans)
			r.Post("/subscription", s.handleSubscribe)
			r.Get("/subscription", s.handleGetSubscription)
			r.Delete("/subscription", s.handleCancelSubscription)
			r.Get("/invoices", s.handleListInvoices)
			r.Get("/invoices/{id}", s.handleGetInvoice)
			r.Post("/invoices/{id}/settle", s.handleSettleInvoice)
		})

		// API-key-authenticated transactional send (Phase 3 US3) — a sibling
		// group off the session/authz path, since callers are server-to-server.
		r.Group(func(r chi.Router) {
			r.Use(s.apiKeyAuth)
			r.Post("/tx", s.handleTransactionalSend)
		})
	})

	// Public, unauthenticated tracking endpoints (Phase 3 US2). The tenant is
	// resolved from the link/campaign UUID, not the path.
	r.Get("/o/{campaignId}", s.handleTrackOpen)
	r.Get("/l/{linkId}", s.handleTrackClick)

	// Public, unauthenticated subscriber-facing pages (Phase 6). The slug-
	// scoped subtree resolves the tenant from the path; token-addressed pages
	// resolve it from the signed token's payload.
	s.mountPublicRoutes(r)

	// Subscriber preference page and one-click unsubscribe (Phase 6 US2). The
	// signed token carries the tenant, so these routes are not slug-scoped.
	r.Get("/p/{token}", s.handlePreferencesForm)
	r.Post("/p/{token}", s.handlePreferencesSubmit)
	r.Get("/u/{token}", s.handleUnsubscribe)
	r.Post("/u/{token}", s.handleUnsubscribe)

	return r
}

// mountPublicRoutes registers the Phase 6 server-rendered public pages — the
// subscription form, double-opt-in confirmation, preference management,
// campaign archive, and RSS feed — none of which require a session.
func (s *Server) mountPublicRoutes(r chi.Router) {
	r.Route("/t/{slug}", func(r chi.Router) {
		r.Use(s.resolvePublicTenant)

		// Public subscription + double opt-in (Phase 6 US1).
		r.Get("/subscribe/{pageSlug}", s.handlePublicSubscribeForm)
		r.Post("/subscribe/{pageSlug}", s.handlePublicSubscribeSubmit)
		r.Get("/confirm/{token}", s.handleConfirm)
		r.Post("/confirm/{token}/resend", s.handleResendConfirmation)

		// Campaign archive + RSS feed (Phase 6 US3).
		r.Get("/archive", s.handleArchiveIndex)
		r.Get("/archive/{campaignId}", s.handleArchiveCampaign)
		r.Get("/feed.xml", s.handleRSSFeed)
	})
}
