package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	tenantcommand "github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	tenantquery "github.com/nikolaymatrosov/nvelope/internal/tenant/app/query"
)

func (s *Server) handleTenantInfo(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	members, err := s.tenant.Queries.WorkspaceMembers.Handle(r.Context(),
		tenantquery.WorkspaceMembers{TenantID: ws.ID})
	if err != nil {
		s.fail(w, r, "tenant info", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant":  tenantPayload(ws.ID, ws.Slug, ws.Name, ws.Status),
		"members": members,
	})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	settings, err := s.tenant.Queries.GetSettings.Handle(r.Context(),
		tenantquery.GetSettings{TenantID: ws.ID})
	if err != nil {
		s.fail(w, r, "get settings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSettingsManage); !ok {
		return
	}
	var req struct {
		DisplayName string `json:"display_name"`
		Timezone    string `json:"timezone"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	result, err := s.tenant.Commands.UpdateSettings.Handle(r.Context(), tenantcommand.UpdateSettings{
		TenantID: ws.ID, DisplayName: req.DisplayName, Timezone: req.Timezone,
	})
	if err != nil {
		s.fail(w, r, "update settings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": map[string]string{
			"display_name": result.DisplayName,
			"timezone":     result.Timezone,
		},
	})
}

// handleSetRawHTMLSanitization changes how the workspace's raw campaign and
// template HTML is sanitized on save. Turning it off requires accept_risk;
// every change is audited.
func (s *Server) handleSetRawHTMLSanitization(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSettingsManage); !ok {
		return
	}
	var req struct {
		Level      string `json:"level"`
		AcceptRisk bool   `json:"accept_risk"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	res, err := s.tenant.Commands.SetRawHTMLSanitization.Handle(r.Context(),
		tenantcommand.SetRawHTMLSanitization{TenantID: ws.ID, Level: req.Level, AcceptRisk: req.AcceptRisk})
	if err != nil {
		s.fail(w, r, "set raw html sanitization", err)
		return
	}
	if res.Previous != res.Current {
		s.recordAudit(r.Context(), "settings.raw_html_sanitization", ws.ID,
			map[string]any{"from": res.Previous, "to": res.Current})
	}
	writeJSON(w, http.StatusOK, map[string]any{"raw_html_sanitization": res.Current})
}

func (s *Server) handleCreateInvitation(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	inviter, _ := userFromContext(r.Context())

	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}

	result, err := s.tenant.Commands.InviteTeammate.Handle(r.Context(), tenantcommand.InviteTeammate{
		TenantID: ws.ID, InviterID: inviter.ID, Email: req.Email,
	})
	if err != nil {
		s.fail(w, r, "create invitation", err)
		return
	}
	if result.AlreadyMember {
		writeJSON(w, http.StatusOK, map[string]any{
			"code":    "already_member",
			"message": "that person is already a member of this tenant",
		})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"invitation": map[string]any{
			"id":         result.InvitationID,
			"email":      result.Email,
			"status":     result.Status,
			"created_at": result.CreatedAt,
			"expires_at": result.ExpiresAt,
		},
		"accept_url": s.cfg.BaseURL + "/invite/" + result.Token,
	})
}

func (s *Server) handleListInvitations(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	invitations, err := s.tenant.Queries.PendingInvitations.Handle(r.Context(),
		tenantquery.PendingInvitations{TenantID: ws.ID})
	if err != nil {
		s.fail(w, r, "list invitations", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invitations": invitations})
}

func (s *Server) handleRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if err := s.tenant.Commands.RevokeInvitation.Handle(r.Context(), tenantcommand.RevokeInvitation{
		TenantID: ws.ID, InvitationID: chi.URLParam(r, "id"),
	}); err != nil {
		s.fail(w, r, "revoke invitation", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
