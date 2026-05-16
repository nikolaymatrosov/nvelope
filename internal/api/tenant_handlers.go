package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/nvelope/nvelope/internal/auth"
	"github.com/nvelope/nvelope/internal/tenant"
)

type tenantCtxKey int

const tenantCtxKeyValue tenantCtxKey = 0

// resolveTenant is middleware for /t/{slug}/... routes. It resolves the slug
// to a tenant and confirms the authenticated user is a member. An unknown slug
// and a non-member both yield an identical opaque 404 (mapped centrally by
// fail), so a non-member cannot learn whether a tenant exists. On success the
// tenant is placed in the request context.
func (s *Server) resolveTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.UserFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "a valid session is required")
			return
		}
		t, _, err := tenant.Resolve(r.Context(), s.pool, chi.URLParam(r, "slug"), user.ID)
		if err != nil {
			s.fail(w, "resolve tenant", err)
			return
		}
		ctx := context.WithValue(r.Context(), tenantCtxKeyValue, t)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func tenantFromContext(ctx context.Context) tenant.Tenant {
	t, _ := ctx.Value(tenantCtxKeyValue).(tenant.Tenant)
	return t
}

func (s *Server) handleTenantInfo(w http.ResponseWriter, r *http.Request) {
	t := tenantFromContext(r.Context())
	members, err := tenant.ListMembers(r.Context(), s.pool, t.ID)
	if err != nil {
		s.serverError(w, "tenant info", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenant": t, "members": members})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	t := tenantFromContext(r.Context())
	var settings tenant.Settings
	err := tenant.WithTenant(r.Context(), s.pool, t.ID, func(ctx context.Context, tx pgx.Tx) error {
		var gerr error
		settings, gerr = tenant.GetSettings(ctx, tx)
		return gerr
	})
	if err != nil {
		s.serverError(w, "get settings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	t := tenantFromContext(r.Context())
	var settings tenant.Settings
	if err := decodeJSON(r, &settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	err := tenant.WithTenant(r.Context(), s.pool, t.ID, func(ctx context.Context, tx pgx.Tx) error {
		return tenant.UpdateSettings(ctx, tx, settings)
	})
	if err != nil {
		s.fail(w, "update settings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (s *Server) handleCreateInvitation(w http.ResponseWriter, r *http.Request) {
	t := tenantFromContext(r.Context())
	inviter, _ := auth.UserFromContext(r.Context())

	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if !auth.ValidEmail(req.Email) {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed",
			"a valid email address is required")
		return
	}

	// If the email already belongs to a member, do not create an invitation.
	if existing, err := auth.LookupUserByEmail(r.Context(), s.pool, req.Email); err == nil {
		if _, err := tenant.GetMembershipRole(r.Context(), s.pool, existing.ID, t.ID); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"error":   "already_member",
				"message": "that person is already a member of this tenant",
			})
			return
		}
	}

	inv, rawToken, err := tenant.CreateInvitation(
		r.Context(), s.pool, t.ID, req.Email, inviter.ID, s.cfg.InviteTTL)
	if err != nil {
		s.fail(w, "create invitation", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"invitation": inv,
		"accept_url": s.cfg.BaseURL + "/invite/" + rawToken,
	})
}

func (s *Server) handleListInvitations(w http.ResponseWriter, r *http.Request) {
	t := tenantFromContext(r.Context())
	invitations, err := tenant.ListPendingInvitations(r.Context(), s.pool, t.ID)
	if err != nil {
		s.serverError(w, "list invitations", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invitations": invitations})
}

func (s *Server) handleRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	t := tenantFromContext(r.Context())
	if err := tenant.RevokeInvitation(r.Context(), s.pool, t.ID, chi.URLParam(r, "id")); err != nil {
		s.fail(w, "revoke invitation", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
