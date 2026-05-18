package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	campaigncommand "github.com/nikolaymatrosov/nvelope/internal/campaign/app/command"
	campaignquery "github.com/nikolaymatrosov/nvelope/internal/campaign/app/query"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// campaignPageFromRequest reads limit/offset query parameters into a
// campaign-domain Page.
func campaignPageFromRequest(r *http.Request) campaigndomain.Page {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	return campaigndomain.Page{Limit: limit, Offset: offset}
}

func (s *Server) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	var req struct {
		Name     string `json:"name"`
		Kind     string `json:"kind"`
		Subject  string `json:"subject"`
		BodyHTML string `json:"body_html"`
		BodyText string `json:"body_text"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	res, err := s.campaign.Commands.CreateTemplate.Handle(r.Context(), campaigncommand.CreateTemplate{
		TenantID: ws.ID, Name: req.Name, Kind: req.Kind, Subject: req.Subject,
		BodyHTML: req.BodyHTML, BodyText: req.BodyText,
	})
	if err != nil {
		s.fail(w, "create template", err)
		return
	}
	s.respondTemplate(w, r, ws.ID, res.TemplateID, http.StatusCreated)
}

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsGet); !ok {
		return
	}
	page, err := s.campaign.Queries.ListTemplates.Handle(r.Context(), campaignquery.ListTemplates{
		TenantID: ws.ID, Page: campaignPageFromRequest(r),
	})
	if err != nil {
		s.fail(w, "list templates", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": page.Templates, "total": page.Total})
}

func (s *Server) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsGet); !ok {
		return
	}
	s.respondTemplate(w, r, ws.ID, chi.URLParam(r, "id"), http.StatusOK)
}

func (s *Server) handleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	var req struct {
		Name     string `json:"name"`
		Subject  string `json:"subject"`
		BodyHTML string `json:"body_html"`
		BodyText string `json:"body_text"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.campaign.Commands.UpdateTemplate.Handle(r.Context(), campaigncommand.UpdateTemplate{
		TenantID: ws.ID, TemplateID: chi.URLParam(r, "id"), Name: req.Name,
		Subject: req.Subject, BodyHTML: req.BodyHTML, BodyText: req.BodyText,
	}); err != nil {
		s.fail(w, "update template", err)
		return
	}
	s.respondTemplate(w, r, ws.ID, chi.URLParam(r, "id"), http.StatusOK)
}

func (s *Server) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	if err := s.campaign.Commands.DeleteTemplate.Handle(r.Context(), campaigncommand.DeleteTemplate{
		TenantID: ws.ID, TemplateID: chi.URLParam(r, "id"),
	}); err != nil {
		s.fail(w, "delete template", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// respondTemplate fetches and writes a template view.
func (s *Server) respondTemplate(w http.ResponseWriter, r *http.Request, tenantID, id string, status int) {
	view, err := s.campaign.Queries.GetTemplate.Handle(r.Context(), campaignquery.GetTemplate{
		TenantID: tenantID, TemplateID: id,
	})
	if err != nil {
		s.fail(w, "get template", err)
		return
	}
	writeJSON(w, status, view)
}

func (s *Server) handleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	var req struct {
		Name            string            `json:"name"`
		TemplateID      string            `json:"template_id"`
		Subject         string            `json:"subject"`
		BodyHTML        string            `json:"body_html"`
		BodyText        string            `json:"body_text"`
		FromName        string            `json:"from_name"`
		FromLocalPart   string            `json:"from_local_part"`
		SendingDomainID string            `json:"sending_domain_id"`
		ListIDs         []string          `json:"list_ids"`
		Segments        []json.RawMessage `json:"segments"`
		MaxSendErrors   int               `json:"max_send_errors"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	res, err := s.campaign.Commands.CreateCampaign.Handle(r.Context(), campaigncommand.CreateCampaign{
		TenantID: ws.ID, Name: req.Name, TemplateID: req.TemplateID, Subject: req.Subject,
		BodyHTML: req.BodyHTML, BodyText: req.BodyText, FromName: req.FromName,
		FromLocalPart: req.FromLocalPart, SendingDomainID: req.SendingDomainID,
		ListIDs: req.ListIDs, Segments: rawSegments(req.Segments), MaxSendErrors: req.MaxSendErrors,
	})
	if err != nil {
		s.fail(w, "create campaign", err)
		return
	}
	s.respondCampaign(w, r, ws.ID, res.CampaignID, http.StatusCreated)
}

func (s *Server) handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsGet); !ok {
		return
	}
	page, err := s.campaign.Queries.ListCampaigns.Handle(r.Context(), campaignquery.ListCampaigns{
		TenantID: ws.ID, Page: campaignPageFromRequest(r),
	})
	if err != nil {
		s.fail(w, "list campaigns", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"campaigns": page.Campaigns, "total": page.Total})
}

func (s *Server) handleGetCampaign(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsGet); !ok {
		return
	}
	s.respondCampaign(w, r, ws.ID, chi.URLParam(r, "id"), http.StatusOK)
}

func (s *Server) handleUpdateCampaign(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	var req struct {
		Name            string            `json:"name"`
		Subject         string            `json:"subject"`
		BodyHTML        string            `json:"body_html"`
		BodyText        string            `json:"body_text"`
		FromName        string            `json:"from_name"`
		FromLocalPart   string            `json:"from_local_part"`
		SendingDomainID string            `json:"sending_domain_id"`
		ListIDs         []string          `json:"list_ids"`
		Segments        []json.RawMessage `json:"segments"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.campaign.Commands.UpdateCampaign.Handle(r.Context(), campaigncommand.UpdateCampaign{
		TenantID: ws.ID, CampaignID: chi.URLParam(r, "id"), Name: req.Name, Subject: req.Subject,
		BodyHTML: req.BodyHTML, BodyText: req.BodyText, FromName: req.FromName,
		FromLocalPart: req.FromLocalPart, SendingDomainID: req.SendingDomainID,
		ListIDs: req.ListIDs, Segments: rawSegments(req.Segments),
	}); err != nil {
		s.fail(w, "update campaign", err)
		return
	}
	s.respondCampaign(w, r, ws.ID, chi.URLParam(r, "id"), http.StatusOK)
}

func (s *Server) handleStartCampaign(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	if err := s.campaign.Commands.StartCampaign.Handle(r.Context(), campaigncommand.StartCampaign{
		TenantID: ws.ID, CampaignID: chi.URLParam(r, "id"),
	}); err != nil {
		s.fail(w, "start campaign", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "running"})
}

func (s *Server) handlePauseCampaign(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	if err := s.campaign.Commands.PauseCampaign.Handle(r.Context(), campaigncommand.PauseCampaign{
		TenantID: ws.ID, CampaignID: chi.URLParam(r, "id"),
	}); err != nil {
		s.fail(w, "pause campaign", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "paused"})
}

func (s *Server) handleResumeCampaign(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	if err := s.campaign.Commands.ResumeCampaign.Handle(r.Context(), campaigncommand.ResumeCampaign{
		TenantID: ws.ID, CampaignID: chi.URLParam(r, "id"),
	}); err != nil {
		s.fail(w, "resume campaign", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "running"})
}

func (s *Server) handleCancelCampaign(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	if err := s.campaign.Commands.CancelCampaign.Handle(r.Context(), campaigncommand.CancelCampaign{
		TenantID: ws.ID, CampaignID: chi.URLParam(r, "id"),
	}); err != nil {
		s.fail(w, "cancel campaign", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}

// respondCampaign fetches and writes a campaign view.
func (s *Server) respondCampaign(w http.ResponseWriter, r *http.Request, tenantID, id string, status int) {
	view, err := s.campaign.Queries.GetCampaign.Handle(r.Context(), campaignquery.GetCampaign{
		TenantID: tenantID, CampaignID: id,
	})
	if err != nil {
		s.fail(w, "get campaign", err)
		return
	}
	writeJSON(w, status, view)
}

// rawSegments projects JSON segment payloads onto the [][]byte the command
// layer expects.
func rawSegments(segments []json.RawMessage) [][]byte {
	if len(segments) == 0 {
		return nil
	}
	out := make([][]byte, 0, len(segments))
	for _, seg := range segments {
		out = append(out, []byte(seg))
	}
	return out
}
