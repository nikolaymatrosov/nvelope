package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	campaigncommand "github.com/nikolaymatrosov/nvelope/internal/campaign/app/command"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	tenantcommand "github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	tenantquery "github.com/nikolaymatrosov/nvelope/internal/tenant/app/query"
)

// brandingRequest is the JSON body for configuring a tenant's branding.
type brandingRequest struct {
	LogoURL      string `json:"logo_url"`
	PrimaryColor string `json:"primary_color"`
	CustomCSS    string `json:"custom_css"`
}

// handleGetBranding returns the tenant's branding.
func (s *Server) handleGetBranding(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermBrandingManage); !ok {
		return
	}
	view, err := s.tenant.Queries.GetBranding.Handle(r.Context(),
		tenantquery.GetBranding{TenantID: ws.ID})
	if err != nil {
		s.fail(w, "get branding", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// handleSaveBranding configures the tenant's branding.
func (s *Server) handleSaveBranding(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermBrandingManage); !ok {
		return
	}
	var req brandingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.tenant.Commands.SaveBranding.Handle(r.Context(), tenantcommand.SaveBranding{
		TenantID:     ws.ID,
		LogoURL:      req.LogoURL,
		PrimaryColor: req.PrimaryColor,
		CustomCSS:    req.CustomCSS,
	}); err != nil {
		s.fail(w, "save branding", err)
		return
	}
	view, err := s.tenant.Queries.GetBranding.Handle(r.Context(),
		tenantquery.GetBranding{TenantID: ws.ID})
	if err != nil {
		s.fail(w, "save branding", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// archiveRequest is the JSON body for toggling a campaign's archive visibility.
type archiveRequest struct {
	Visible bool `json:"visible"`
}

// handleSetCampaignArchive toggles a campaign's appearance on the public
// archive and RSS feed.
func (s *Server) handleSetCampaignArchive(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	var req archiveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.campaign.Commands.SetArchiveVisibility.Handle(r.Context(),
		campaigncommand.SetArchiveVisibility{
			TenantID: ws.ID, CampaignID: chi.URLParam(r, "id"), Visible: req.Visible,
		}); err != nil {
		s.fail(w, "set campaign archive", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"visible": req.Visible})
}
