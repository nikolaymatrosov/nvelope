package api

import (
	"net/http"

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
