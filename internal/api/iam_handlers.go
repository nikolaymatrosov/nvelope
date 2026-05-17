package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	iamcommand "github.com/nikolaymatrosov/nvelope/internal/iam/app/command"
	iamquery "github.com/nikolaymatrosov/nvelope/internal/iam/app/query"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// setWorkspaceCookie writes the tenant-plane workspace session cookie,
// path-scoped to the tenant so each tenant carries its own session.
func (s *Server) setWorkspaceCookie(w http.ResponseWriter, slug, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     workspaceCookie,
		Value:    token,
		Path:     "/t/" + slug,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
	})
}

// clearWorkspaceCookie expires the workspace session cookie.
func (s *Server) clearWorkspaceCookie(w http.ResponseWriter, slug string) {
	http.SetCookie(w, &http.Cookie{
		Name:     workspaceCookie,
		Value:    "",
		Path:     "/t/" + slug,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (s *Server) handleOpenSession(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	user, _ := userFromContext(r.Context())

	res, err := s.iam.Commands.OpenWorkspaceSession.Handle(r.Context(), iamcommand.OpenWorkspaceSession{
		TenantID: ws.ID, PlatformUserID: user.ID, Email: user.Email, Name: user.Name,
	})
	if err != nil {
		s.fail(w, "open session", err)
		return
	}
	s.setWorkspaceCookie(w, chi.URLParam(r, "slug"), res.Token)
	writeJSON(w, http.StatusCreated, map[string]any{"state": res.State})
}

func (s *Server) handleCloseSession(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if c, err := r.Cookie(workspaceCookie); err == nil {
		if err := s.iam.Commands.CloseSession.Handle(r.Context(), iamcommand.CloseSession{
			TenantID: ws.ID, Token: c.Value,
		}); err != nil {
			s.fail(w, "close session", err)
			return
		}
	}
	s.clearWorkspaceCookie(w, chi.URLParam(r, "slug"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermRolesManage)
	if !ok {
		return
	}
	var req struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	res, err := s.iam.Commands.CreateRole.Handle(r.Context(), iamcommand.CreateRole{
		TenantID: ws.ID, ActorID: principal.ActorID(), Name: req.Name, Permissions: req.Permissions,
	})
	if err != nil {
		s.fail(w, "create role", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": res.RoleID})
}

func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermRolesGet); !ok {
		return
	}
	roles, err := s.iam.Queries.ListRoles.Handle(r.Context(), iamquery.ListRoles{TenantID: ws.ID})
	if err != nil {
		s.fail(w, "list roles", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

func (s *Server) handleUpdateRole(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermRolesManage)
	if !ok {
		return
	}
	var req struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.iam.Commands.UpdateRole.Handle(r.Context(), iamcommand.UpdateRole{
		TenantID: ws.ID, ActorID: principal.ActorID(), RoleID: chi.URLParam(r, "id"),
		Name: req.Name, Permissions: req.Permissions,
	}); err != nil {
		s.fail(w, "update role", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermRolesManage)
	if !ok {
		return
	}
	if err := s.iam.Commands.DeleteRole.Handle(r.Context(), iamcommand.DeleteRole{
		TenantID: ws.ID, ActorID: principal.ActorID(), RoleID: chi.URLParam(r, "id"),
	}); err != nil {
		s.fail(w, "delete role", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAssignRole(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermRolesManage)
	if !ok {
		return
	}
	var req struct {
		RoleID string `json:"role_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.iam.Commands.AssignRole.Handle(r.Context(), iamcommand.AssignRole{
		TenantID: ws.ID, ActorID: principal.ActorID(),
		UserID: chi.URLParam(r, "userId"), RoleID: req.RoleID,
	}); err != nil {
		s.fail(w, "assign role", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAssignListRole(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermRolesManage)
	if !ok {
		return
	}
	var req struct {
		RoleID string `json:"role_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.iam.Commands.AssignListRole.Handle(r.Context(), iamcommand.AssignListRole{
		TenantID: ws.ID, ActorID: principal.ActorID(),
		UserID: chi.URLParam(r, "userId"), ListID: chi.URLParam(r, "listId"), RoleID: req.RoleID,
	}); err != nil {
		s.fail(w, "assign list role", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveListRole(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermRolesManage)
	if !ok {
		return
	}
	if err := s.iam.Commands.RevokeRole.Handle(r.Context(), iamcommand.RevokeRole{
		TenantID: ws.ID, ActorID: principal.ActorID(),
		UserID: chi.URLParam(r, "userId"), ListID: chi.URLParam(r, "listId"),
	}); err != nil {
		s.fail(w, "remove list role", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleIssueAPIKey(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermAPIKeysManage)
	if !ok {
		return
	}
	var req struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	res, err := s.iam.Commands.IssueAPIKey.Handle(r.Context(), iamcommand.IssueAPIKey{
		TenantID: ws.ID, ActorID: principal.ActorID(), Name: req.Name, Permissions: req.Permissions,
	})
	if err != nil {
		s.fail(w, "issue API key", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": res.KeyID, "token": res.Token})
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermAPIKeysGet); !ok {
		return
	}
	keys, err := s.iam.Queries.ListAPIKeys.Handle(r.Context(), iamquery.ListAPIKeys{TenantID: ws.ID})
	if err != nil {
		s.fail(w, "list API keys", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": keys})
}

func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermAPIKeysManage)
	if !ok {
		return
	}
	if err := s.iam.Commands.RevokeAPIKey.Handle(r.Context(), iamcommand.RevokeAPIKey{
		TenantID: ws.ID, ActorID: principal.ActorID(), KeyID: chi.URLParam(r, "id"),
	}); err != nil {
		s.fail(w, "revoke API key", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleVerifyTOTP meets a totp-pending session's two-factor challenge. It runs
// outside the authz middleware because a totp-pending session resolves to no
// principal.
func (s *Server) handleVerifyTOTP(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	c, err := r.Cookie(workspaceCookie)
	if err != nil || c.Value == "" {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "a workspace session is required")
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.iam.Commands.VerifyTOTPChallenge.Handle(r.Context(), iamcommand.VerifyTOTPChallenge{
		TenantID: ws.ID, Token: c.Value, Code: req.Code,
	}); err != nil {
		s.fail(w, "verify TOTP", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": string(iamdomain.SessionActive)})
}

// requireSessionPrincipal returns the request's Principal when it was resolved
// from a workspace session — TOTP enrolment is per-user and cannot be driven by
// an API key.
func (s *Server) requireSessionPrincipal(w http.ResponseWriter, r *http.Request) (iamdomain.Principal, bool) {
	p, ok := principalFromContext(r.Context())
	if !ok || p.Kind() != iamdomain.PrincipalSession {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "a workspace session is required")
		return iamdomain.Principal{}, false
	}
	return p, true
}

func (s *Server) handleEnableTOTP(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requireSessionPrincipal(w, r)
	if !ok {
		return
	}
	user, _ := userFromContext(r.Context())
	res, err := s.iam.Commands.EnableTOTP.Handle(r.Context(), iamcommand.EnableTOTP{
		TenantID: ws.ID, UserID: principal.ActorID(), AccountName: user.Email,
	})
	if err != nil {
		s.fail(w, "enable TOTP", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secret": res.Secret, "uri": res.URI})
}

func (s *Server) handleConfirmTOTP(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requireSessionPrincipal(w, r)
	if !ok {
		return
	}
	var req struct {
		Secret string `json:"secret"`
		Code   string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	res, err := s.iam.Commands.ConfirmTOTP.Handle(r.Context(), iamcommand.ConfirmTOTP{
		TenantID: ws.ID, UserID: principal.ActorID(), Secret: req.Secret, Code: req.Code,
	})
	if err != nil {
		s.fail(w, "confirm TOTP", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": res.RecoveryCodes})
}

func (s *Server) handleAuditTrail(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermAuditGet); !ok {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	res, err := s.iam.Queries.AuditTrail.Handle(r.Context(), iamquery.AuditTrail{
		TenantID: ws.ID, Page: iamdomain.Page{Limit: limit, Offset: offset},
	})
	if err != nil {
		s.fail(w, "audit trail", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": res.Records, "total": res.Total})
}

func (s *Server) handleDisableTOTP(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requireSessionPrincipal(w, r)
	if !ok {
		return
	}
	if err := s.iam.Commands.DisableTOTP.Handle(r.Context(), iamcommand.DisableTOTP{
		TenantID: ws.ID, UserID: principal.ActorID(),
	}); err != nil {
		s.fail(w, "disable TOTP", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
