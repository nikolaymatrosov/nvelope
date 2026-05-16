package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	authapp "github.com/nikolaymatrosov/nvelope/internal/auth/app"
	"github.com/nikolaymatrosov/nvelope/internal/config"
	tenantapp "github.com/nikolaymatrosov/nvelope/internal/tenant/app"
)

// Server wires the nvelope HTTP API: the wired Application, configuration, and
// the route table. It holds no database handle — every request flows through
// the command and query handlers.
type Server struct {
	auth   authapp.Application
	tenant tenantapp.Application
	cfg    config.Config
	logger *slog.Logger
	health http.Handler
}

// New returns a Server. The auth and tenant applications are built by the
// composition root; the health handler is supplied by the caller so it can
// also toggle readiness during startup and graceful shutdown.
func New(auth authapp.Application, tenant tenantapp.Application, cfg config.Config,
	logger *slog.Logger, health http.Handler) *Server {
	return &Server{auth: auth, tenant: tenant, cfg: cfg, logger: logger, health: health}
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
	})

	return r
}
