package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	authcommand "github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	authdomain "github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	tenantcommand "github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	tenantquery "github.com/nikolaymatrosov/nvelope/internal/tenant/app/query"
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
	result, err := s.auth.Commands.SignUp.Handle(r.Context(), authcommand.SignUp{
		Email: req.Email, Password: req.Password, Name: req.Name,
	})
	if err != nil {
		s.fail(w, "signup", err)
		return
	}
	s.setSessionCookie(w, result.Token)
	locale := s.resolveAuthLocale(w, r, result.UserID, "")
	writeJSON(w, http.StatusCreated, map[string]any{
		"user": userPayload(result.UserID, result.UserEmail, result.UserName, locale),
	})
}

// resolveAuthLocale picks the user's effective interface language right after
// authentication and mirrors it into the nv_locale cookie. An existing stored
// preference wins; otherwise, when the request carries a supported nv_locale
// cookie, that choice is adopted as the account preference (FR-008). An
// unsupported or absent cookie leaves the locale unset.
func (s *Server) resolveAuthLocale(w http.ResponseWriter, r *http.Request, userID, stored string) string {
	effective := stored
	if effective == "" {
		if c, err := r.Cookie(localeCookie); err == nil && c.Value != "" {
			// SetLocale validates the value; adoption is silently skipped when
			// the cookie carries an unsupported locale.
			if err := s.auth.Commands.SetLocale.Handle(r.Context(),
				authcommand.SetLocale{UserID: userID, Locale: c.Value}); err == nil {
				effective = c.Value
			}
		}
	}
	if effective != "" {
		s.setLocaleCookie(w, effective)
	}
	return effective
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	result, err := s.auth.Commands.LogIn.Handle(r.Context(), authcommand.LogIn{
		Email: req.Email, Password: req.Password,
	})
	if err != nil {
		s.fail(w, "login", err)
		return
	}
	s.setSessionCookie(w, result.Token)
	locale := s.resolveAuthLocale(w, r, result.UserID, result.UserLocale)
	writeJSON(w, http.StatusOK, map[string]any{
		"user": userPayload(result.UserID, result.UserEmail, result.UserName, locale),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var raw string
	if c, err := r.Cookie(sessionCookie); err == nil {
		raw = c.Value
	}
	if err := s.auth.Commands.LogOut.Handle(r.Context(), authcommand.LogOut{RawToken: raw}); err != nil {
		s.fail(w, "logout", err)
		return
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	memberships, err := s.tenant.Queries.ListWorkspaces.Handle(r.Context(),
		tenantquery.ListWorkspaces{UserID: user.ID})
	if err != nil {
		s.fail(w, "me", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":    userPayload(user.ID, user.Email, user.Name, user.Locale),
		"tenants": memberships,
	})
}

// handleUpdateMe changes the authenticated user's interface-language
// preference. The target user is always the caller resolved from the session,
// so one user can never write another's locale.
func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var req struct {
		Locale string `json:"locale"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.auth.Commands.SetLocale.Handle(r.Context(), authcommand.SetLocale{
		UserID: user.ID, Locale: req.Locale,
	}); err != nil {
		s.fail(w, "set locale", err)
		return
	}
	// Mirror the chosen locale into the cookie so the next server render is in
	// the new language.
	s.setLocaleCookie(w, req.Locale)
	writeJSON(w, http.StatusOK, map[string]any{
		"user": userPayload(user.ID, user.Email, user.Name, req.Locale),
	})
}

func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	result, err := s.tenant.Commands.CreateWorkspace.Handle(r.Context(), tenantcommand.CreateWorkspace{
		OwnerID: user.ID, Name: req.Name, Slug: req.Slug,
	})
	if err != nil {
		s.fail(w, "create tenant", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"tenant": tenantPayload(result.TenantID, result.Slug, result.Name, result.Status),
	})
}

func (s *Server) handleListTenants(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	memberships, err := s.tenant.Queries.ListWorkspaces.Handle(r.Context(),
		tenantquery.ListWorkspaces{UserID: user.ID})
	if err != nil {
		s.fail(w, "list tenants", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": memberships})
}

func (s *Server) handleGetInvitation(w http.ResponseWriter, r *http.Request) {
	lookup, err := s.tenant.Queries.LookUpInvitation.Handle(r.Context(),
		tenantquery.LookUpInvitation{Token: chi.URLParam(r, "token")})
	if err != nil {
		s.fail(w, "get invitation", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant": map[string]string{"slug": lookup.TenantSlug, "name": lookup.TenantName},
		"email":  lookup.Email,
	})
}

func (s *Server) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	cmd := tenantcommand.AcceptInvitation{Token: chi.URLParam(r, "token")}

	currentUser, hasSession := s.authenticate(r)
	if hasSession {
		cmd.CurrentUserID = currentUser.ID
	} else {
		var body struct {
			Password string `json:"password"`
			Name     string `json:"name"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body",
				"request body must supply a password and name to accept this invitation")
			return
		}
		cmd.Password = body.Password
		cmd.Name = body.Name
	}

	result, err := s.tenant.Commands.AcceptInvitation.Handle(r.Context(), cmd)
	if errors.Is(err, authdomain.ErrEmailTaken) {
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

	userID, userEmail, userName := currentUser.ID, currentUser.Email, currentUser.Name
	stored := currentUser.Locale
	if result.NewUser != nil {
		s.setSessionCookie(w, result.NewUser.SessionToken)
		userID = result.NewUser.ID
		userEmail = result.NewUser.Email
		userName = result.NewUser.Name
		stored = ""
	}
	locale := s.resolveAuthLocale(w, r, userID, stored)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":   userPayload(userID, userEmail, userName, locale),
		"tenant": tenantPayload(result.TenantID, result.TenantSlug, result.TenantName, result.TenantStatus),
	})
}
