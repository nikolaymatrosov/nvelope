package api

import (
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
	sendingapp "github.com/nikolaymatrosov/nvelope/internal/sending/app"
	tenantapp "github.com/nikolaymatrosov/nvelope/internal/tenant/app"
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
	tracking       campaigndomain.TrackingRepository
	cfg            config.Config
	logger         *slog.Logger
	health         http.Handler
}

// New returns a Server. The context applications are built by the composition
// root; tracking is the campaign context's tracking repository, used directly
// by the public open/click endpoints. The health handler is supplied by the
// caller so it can also toggle readiness during startup and graceful shutdown.
func New(auth authapp.Application, tenant tenantapp.Application, audience audienceapp.Application,
	iam iamapp.Application, sending sendingapp.Application, campaign campaignapp.Application,
	deliverability deliverabilityapp.Application, billing billingapp.Application,
	tracking campaigndomain.TrackingRepository,
	cfg config.Config, logger *slog.Logger, health http.Handler) *Server {
	return &Server{
		auth: auth, tenant: tenant, audience: audience, iam: iam, sending: sending,
		campaign: campaign, deliverability: deliverability, billing: billing,
		tracking: tracking, cfg: cfg, logger: logger, health: health,
	}
}

// Handler builds the chi router with every route mounted.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Method(http.MethodGet, "/healthz", s.health)

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
			r.Delete("/templates/{id}", s.handleDeleteTemplate)
			r.Post("/campaigns", s.handleCreateCampaign)
			r.Get("/campaigns", s.handleListCampaigns)
			r.Get("/campaigns/{id}", s.handleGetCampaign)
			r.Put("/campaigns/{id}", s.handleUpdateCampaign)
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

	return r
}
