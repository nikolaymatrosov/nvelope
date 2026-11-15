package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	authcommand "github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	authdomain "github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	tenantcommand "github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	tenantquery "github.com/nikolaymatrosov/nvelope/internal/tenant/app/query"
	tenantdomain "github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	// A single-tenant install has no open registration: the first account
	// comes from first-run setup, everyone else from an invitation.
	if s.cfg.SingleTenant() {
		s.handleSetupSignup(w, r)
		return
	}
	var req credentialsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	result, err := s.auth.Commands.SignUp.Handle(r.Context(), authcommand.SignUp{
		Email: req.Email, Password: req.Password, Name: req.Name,
	})
	if err != nil {
		s.fail(w, r, "signup", err)
		return
	}
	// No session is issued — the account must verify its email address first.
	// The request's locale preference is still adopted onto the new account so
	// the verification email can be localized.
	s.resolveAuthLocale(w, r, result.UserID, "")
	writeJSON(w, http.StatusCreated, map[string]any{
		"verification": map[string]any{"required": true, "email": result.UserEmail},
	})
}

// handleSetupSignup is first-run setup of a single-tenant install. With the
// setup token, and only while the install's workspace does not exist yet, it
// creates the owner's account (verified and signed in) and the workspace.
func (s *Server) handleSetupSignup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		credentialsRequest
		SetupToken string `json:"setup_token"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	done, err := s.setupDone(r.Context())
	if err != nil {
		s.fail(w, r, "setup", err)
		return
	}
	if done {
		writeError(w, r, http.StatusForbidden, "setup_complete",
			"this install is already set up; ask a workspace admin for an invitation")
		return
	}
	// ponytail: two concurrent setups with the right token both pass the check
	// above; the second CreateWorkspace then fails on the slug and leaves a
	// verified account with no workspace. Only the token holder can race, so
	// it is not guarded; the upgrade path is one transaction across auth and
	// tenant, as onboarding does.
	account, err := s.auth.Commands.SetUp.Handle(r.Context(), authcommand.SetUp{
		Token: req.SetupToken, Email: req.Email, Password: req.Password, Name: req.Name,
	})
	if err != nil {
		s.fail(w, r, "setup", err)
		return
	}
	// The session is set before the workspace is created: if that step fails,
	// the owner is still signed in and creates the workspace from the landing
	// page (handleCreateTenant pins the slug).
	s.setSessionCookie(w, account.SessionToken)
	locale := s.resolveAuthLocale(w, r, account.UserID, "")
	ws, err := s.tenant.Commands.CreateWorkspace.Handle(r.Context(), tenantcommand.CreateWorkspace{
		OwnerID: account.UserID, Name: s.cfg.TenantSlug, Slug: s.cfg.TenantSlug,
	})
	if err != nil {
		s.fail(w, r, "setup workspace", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"user":   userPayload(account.UserID, account.UserEmail, account.UserName, locale),
		"tenant": tenantPayload(ws.TenantID, ws.Slug, ws.Name, ws.Status),
	})
}

// setupDone reports whether first-run setup has created the single-tenant
// install's workspace.
func (s *Server) setupDone(ctx context.Context) (bool, error) {
	_, err := s.tenant.Queries.LocateWorkspace.Handle(ctx,
		tenantquery.LocateWorkspace{Slug: s.cfg.TenantSlug})
	if errors.Is(err, tenantdomain.ErrTenantNotFound) {
		return false, nil
	}
	return err == nil, err
}

// handlePlatformConfig tells the SPA which runtime it talks to. It is public:
// the signup page reads setup_required before anyone has signed in.
func (s *Server) handlePlatformConfig(w http.ResponseWriter, r *http.Request) {
	single := s.cfg.SingleTenant()
	setupRequired := false
	if single {
		done, err := s.setupDone(r.Context())
		if err != nil {
			s.fail(w, r, "platform config", err)
			return
		}
		setupRequired = !done
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"features":       map[string]bool{"billing": !single, "multi_tenant": !single},
		"setup_required": setupRequired,
	})
}

// handleVerifyEmail completes email verification when the recipient opens the
// link from their verification email. It is public — the account has no
// session until it is verified.
func (s *Server) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	result, err := s.auth.Commands.VerifyEmail.Handle(r.Context(),
		authcommand.VerifyEmail{Token: req.Token})
	if err != nil {
		s.fail(w, r, "verify email", err)
		return
	}
	status := "verified"
	if result.AlreadyVerified {
		status = "already_verified"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"verification": map[string]any{"status": status},
	})
}

// handleResendVerification re-sends the verification email for an unverified
// account. It is public and enumeration-safe: the response is identical
// whether or not the address belongs to an account.
func (s *Server) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.auth.Commands.ResendEmailVerification.Handle(r.Context(),
		authcommand.ResendEmailVerification{Email: req.Email}); err != nil {
		s.fail(w, r, "resend verification", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"verification": map[string]any{"resent": true},
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
		writeError(w, r, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	result, err := s.auth.Commands.LogIn.Handle(r.Context(), authcommand.LogIn{
		Email: req.Email, Password: req.Password,
	})
	if err != nil {
		s.fail(w, r, "login", err)
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
		s.fail(w, r, "logout", err)
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
		s.fail(w, r, "me", err)
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
		writeError(w, r, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.auth.Commands.SetLocale.Handle(r.Context(), authcommand.SetLocale{
		UserID: user.ID, Locale: req.Locale,
	}); err != nil {
		s.fail(w, r, "set locale", err)
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
		writeError(w, r, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	// A single-tenant install has exactly one workspace, at the configured
	// slug, so once it exists this answers 409 slug_taken.
	if s.cfg.SingleTenant() {
		req.Slug = s.cfg.TenantSlug
	}
	result, err := s.tenant.Commands.CreateWorkspace.Handle(r.Context(), tenantcommand.CreateWorkspace{
		OwnerID: user.ID, Name: req.Name, Slug: req.Slug,
	})
	if err != nil {
		s.fail(w, r, "create tenant", err)
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
		s.fail(w, r, "list tenants", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": memberships})
}

func (s *Server) handleGetInvitation(w http.ResponseWriter, r *http.Request) {
	lookup, err := s.tenant.Queries.LookUpInvitation.Handle(r.Context(),
		tenantquery.LookUpInvitation{Token: chi.URLParam(r, "token")})
	if err != nil {
		s.fail(w, r, "get invitation", err)
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
			writeError(w, r, http.StatusBadRequest, "invalid_body",
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
		writeError(w, r, http.StatusConflict, "email_taken",
			"an account with this email already exists — log in, then accept the invitation")
		return
	}
	if err != nil {
		s.fail(w, r, "accept invitation", err)
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
