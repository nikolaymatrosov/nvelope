package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/nvelope/nvelope/internal/auth"
	"github.com/nvelope/nvelope/internal/tenant"
)

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	user, sessionToken, err := auth.Signup(
		r.Context(), s.pool, s.cfg.SessionTTL, req.Email, req.Password, req.Name)
	if err != nil {
		s.fail(w, "signup", err)
		return
	}
	auth.SetSessionCookie(w, sessionToken, s.cfg.SessionTTL)
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	user, sessionToken, err := auth.Login(
		r.Context(), s.pool, s.cfg.SessionTTL, req.Email, req.Password)
	if err != nil {
		s.fail(w, "login", err)
		return
	}
	auth.SetSessionCookie(w, sessionToken, s.cfg.SessionTTL)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookie); err == nil && c.Value != "" {
		if err := auth.Logout(r.Context(), s.pool, c.Value); err != nil {
			s.serverError(w, "logout", err)
			return
		}
	}
	auth.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	memberships, err := tenant.ListMembershipsForUser(r.Context(), s.pool, user.ID)
	if err != nil {
		s.serverError(w, "me", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "tenants": memberships})
}

func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	t, err := tenant.CreateTenant(r.Context(), s.pool, user.ID, req.Name, req.Slug)
	if err != nil {
		s.fail(w, "create tenant", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"tenant": t})
}

func (s *Server) handleListTenants(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	memberships, err := tenant.ListMembershipsForUser(r.Context(), s.pool, user.ID)
	if err != nil {
		s.serverError(w, "list tenants", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": memberships})
}

func (s *Server) handleGetInvitation(w http.ResponseWriter, r *http.Request) {
	inv, err := tenant.GetPendingInvitationByToken(r.Context(), s.pool, chi.URLParam(r, "token"))
	if err != nil {
		s.fail(w, "get invitation", err)
		return
	}
	t, err := tenant.GetTenantByID(r.Context(), s.pool, inv.TenantID)
	if err != nil {
		s.serverError(w, "get invitation tenant", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant": map[string]string{"slug": t.Slug, "name": t.Name},
		"email":  inv.Email,
	})
}

func (s *Server) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	inv, err := tenant.GetPendingInvitationByToken(r.Context(), s.pool, chi.URLParam(r, "token"))
	if err != nil {
		s.fail(w, "accept invitation", err)
		return
	}

	currentUser, hasSession := auth.CurrentUser(r, s.pool)
	var body struct {
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if !hasSession {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body",
				"request body must supply a password and name to accept this invitation")
			return
		}
	}

	var (
		user         auth.User
		sessionToken string
	)
	err = pgx.BeginFunc(r.Context(), s.pool, func(tx pgx.Tx) error {
		if hasSession {
			user = currentUser
		} else {
			u, cerr := auth.CreateAccount(r.Context(), tx, inv.Email, body.Password, body.Name)
			if cerr != nil {
				return cerr
			}
			user = u
			t, terr := auth.IssueSession(r.Context(), tx, user.ID, s.cfg.SessionTTL)
			if terr != nil {
				return terr
			}
			sessionToken = t
		}
		accepted, merr := tenant.MarkInvitationAccepted(r.Context(), tx, inv.ID, user.ID)
		if merr != nil {
			return merr
		}
		if !accepted {
			return tenant.ErrInvitationNotFound
		}
		return tenant.AddMembership(r.Context(), tx, user.ID, inv.TenantID, "admin")
	})
	if errors.Is(err, auth.ErrEmailTaken) {
		// More helpful than the generic message: the invitee already has an
		// account and should sign in before accepting.
		writeError(w, http.StatusConflict, "email_taken",
			"an account with this email already exists — log in, then accept the invitation")
		return
	}
	if err != nil {
		s.fail(w, "accept invitation", err)
		return
	}

	t, err := tenant.GetTenantByID(r.Context(), s.pool, inv.TenantID)
	if err != nil {
		s.serverError(w, "accept invitation tenant", err)
		return
	}
	if sessionToken != "" {
		auth.SetSessionCookie(w, sessionToken, s.cfg.SessionTTL)
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "tenant": t})
}
