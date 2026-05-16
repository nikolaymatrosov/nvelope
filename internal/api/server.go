package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvelope/nvelope/internal/auth"
	"github.com/nvelope/nvelope/internal/config"
	"github.com/nvelope/nvelope/internal/tenant"
)

// Server wires the nvelope HTTP API: its dependencies and route table.
type Server struct {
	pool   *pgxpool.Pool
	cfg    config.Config
	logger *slog.Logger
	health http.Handler
}

// New returns a Server. The health handler is supplied by the caller so it
// can also toggle readiness during startup and graceful shutdown.
func New(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger, health http.Handler) *Server {
	return &Server{pool: pool, cfg: cfg, logger: logger, health: health}
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
			r.Use(auth.RequireUser(s.pool))
			r.Post("/logout", s.handleLogout)
			r.Get("/me", s.handleMe)
			r.Post("/tenants", s.handleCreateTenant)
			r.Get("/tenants", s.handleListTenants)
		})
	})

	// Tenant-scoped API — every route passes through authentication and the
	// tenant-resolution + membership cross-check middleware.
	r.Route("/t/{slug}/api", func(r chi.Router) {
		r.Use(auth.RequireUser(s.pool))
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

// serverError logs an unexpected error and returns a generic 500.
func (s *Server) serverError(w http.ResponseWriter, op string, err error) {
	s.logger.Error("request failed", "op", op, "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "something went wrong")
}

// fail is the single place a domain error is mapped to an HTTP response
// (PATTERNS.md #5: transport-agnostic error mapping). Handlers return their
// domain errors here rather than each switching on status codes. An
// unrecognised error is treated as an internal error and logged.
func (s *Server) fail(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, auth.ErrEmailTaken):
		writeError(w, http.StatusConflict, "email_taken", "that email is already registered")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
	case errors.Is(err, tenant.ErrSlugTaken):
		writeError(w, http.StatusConflict, "slug_taken", "that workspace address is already in use")
	case errors.Is(err, tenant.ErrInvitationExists):
		writeError(w, http.StatusConflict, "invitation_exists",
			"a pending invitation for that email already exists")
	case errors.Is(err, tenant.ErrInvitationNotFound):
		writeError(w, http.StatusNotFound, "invitation_not_found", "this invitation is not valid")
	case errors.Is(err, tenant.ErrTenantNotFound), errors.Is(err, tenant.ErrNotMember):
		writeError(w, http.StatusNotFound, "tenant_not_found", "no such tenant")
	default:
		if msg, ok := validationMessage(err); ok {
			writeError(w, http.StatusUnprocessableEntity, "validation_failed", msg)
			return
		}
		s.serverError(w, op, err)
	}
}

// validationMessage extracts a user-safe message from a validation error
// raised by either the auth or tenant package.
func validationMessage(err error) (string, bool) {
	var av auth.ValidationError
	if errors.As(err, &av) {
		return av.Message, true
	}
	var tv tenant.ValidationError
	if errors.As(err, &tv) {
		return tv.Message, true
	}
	return "", false
}
