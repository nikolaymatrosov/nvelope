package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	campaigncommand "github.com/nikolaymatrosov/nvelope/internal/campaign/app/command"
	campaignquery "github.com/nikolaymatrosov/nvelope/internal/campaign/app/query"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
	"github.com/nikolaymatrosov/nvelope/internal/platform/metrics"
)

// saveVisualErrorResult maps a save-command error to the metric label used
// for the "result" dimension of VisualSavesTotal. Unknown errors fall back
// to "error" so the dashboard can flag them without losing the data point.
func saveVisualErrorResult(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, campaigndomain.ErrStaleRow) {
		return "stale_row"
	}
	if typed, ok := apperr.As(err); ok {
		switch typed.Slug() {
		case "unknown_placeholder":
			return "unknown_placeholder"
		case "invalid_media_ref":
			return "invalid_media_ref"
		case "invalid_doc", "validation_failed":
			return "invalid_doc"
		case "campaign_forbidden", "forbidden":
			return "forbidden"
		}
	}
	return "error"
}

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

// handleSaveVisualCampaign persists a visually-authored campaign. This
// endpoint is the Go tail of the BFF-hosted PUT /campaigns/{id}/visual route
// (see specs/014-visual-email-editor/contracts/tenant-api.md). The BFF has
// already rendered the structured document to HTML and plain text via
// @react-email/components; this handler validates the doc (defense in
// depth), sanitizes the rendered HTML, enforces the FR-009 optimistic-
// concurrency gate, and persists all three pieces atomically.
//
// The Go-internal body requires bodyHtml, bodyText, and ifUnmodifiedSince;
// any missing piece is rejected with 400 invalid_body before the command
// runs (the BFF is the only legitimate caller and must always supply them).
func (s *Server) handleSaveVisualCampaign(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	// Wire shape: BodyDoc + Theme arrive as raw JSON the BFF rendered; the
	// typed forms are decoded separately for save-time validation while the
	// raw bytes flow through to persistence so the editor reloads losslessly.
	var req struct {
		Subject           string          `json:"subject"`
		BodyDocRaw        json.RawMessage `json:"bodyDoc"`
		BodyHTML          string          `json:"bodyHtml"`
		BodyText          string          `json:"bodyText"`
		ThemeRaw          json.RawMessage `json:"theme"`
		IfUnmodifiedSince time.Time       `json:"ifUnmodifiedSince"`
	}
	if err := decodeJSON(r, &req); err != nil {
		metrics.VisualSavesTotal.WithLabelValues("campaign", "invalid_body", "false").Inc()
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	doc, theme, ok := s.decodeVisualPayload(w, req.BodyDocRaw, req.ThemeRaw)
	if !ok {
		metrics.VisualSavesTotal.WithLabelValues("campaign", "invalid_doc", "false").Inc()
		return
	}
	if req.BodyHTML == "" || req.BodyText == "" {
		metrics.VisualSavesTotal.WithLabelValues("campaign", "invalid_body", "false").Inc()
		writeError(w, http.StatusBadRequest, "invalid_body", "bodyHtml and bodyText are required")
		return
	}
	if req.IfUnmodifiedSince.IsZero() {
		metrics.VisualSavesTotal.WithLabelValues("campaign", "invalid_body", "false").Inc()
		writeError(w, http.StatusBadRequest, "invalid_body", "ifUnmodifiedSince is required")
		return
	}
	campaignID := chi.URLParam(r, "id")
	res, err := s.campaign.Commands.SaveVisualCampaign.Handle(r.Context(), campaigncommand.SaveVisualCampaign{
		TenantID: ws.ID, CampaignID: campaignID,
		Subject:           req.Subject,
		Doc:               doc,
		BodyHTML:          req.BodyHTML,
		BodyText:          req.BodyText,
		PinnedTheme:       theme,
		DocJSON:           req.BodyDocRaw,
		ThemeJSON:         req.ThemeRaw,
		IfUnmodifiedSince: req.IfUnmodifiedSince,
	})
	if err != nil {
		if errors.Is(err, campaigndomain.ErrStaleRow) {
			metrics.VisualSavesTotal.WithLabelValues("campaign", "stale_row", "false").Inc()
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":            "stale_row",
				"kind":             "stale_row",
				"currentUpdatedAt": res.CurrentUpdatedAt,
			})
			return
		}
		metrics.VisualSavesTotal.WithLabelValues("campaign", saveVisualErrorResult(err), "false").Inc()
		s.fail(w, "save visual campaign", err)
		return
	}
	view, err := s.campaign.Queries.GetCampaign.Handle(r.Context(), campaignquery.GetCampaign{
		TenantID: ws.ID, CampaignID: campaignID,
	})
	if err != nil {
		metrics.VisualSavesTotal.WithLabelValues("campaign", "error", "false").Inc()
		s.fail(w, "save visual campaign", err)
		return
	}
	warningsPresent := "false"
	if len(res.Warnings) > 0 {
		warningsPresent = "true"
	}
	metrics.VisualSavesTotal.WithLabelValues("campaign", "ok", warningsPresent).Inc()
	s.recordAudit(r.Context(), "campaign.save_visual", campaignID, map[string]any{
		"warnings_count": len(res.Warnings),
	})
	s.logEvent(r.Context(), "campaign.save_visual",
		slog.String("campaign_id", campaignID),
		slog.Int("warnings_count", len(res.Warnings)),
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"campaign":  view,
		"warnings":  res.Warnings,
		"updatedAt": res.UpdatedAt,
	})
}

// handleSaveVisualTemplate persists a visually-authored template (T073).
// This endpoint is the Go tail of the BFF-hosted PUT /templates/{id}/visual
// route (see specs/014-visual-email-editor/contracts/tenant-api.md). The
// BFF has already rendered the structured document to HTML and plain text
// via @react-email/components; this handler validates the doc (defense in
// depth), sanitizes the rendered HTML, enforces the FR-009 optimistic-
// concurrency gate, and persists all three pieces atomically.
//
// The Go-internal body requires bodyHtml, bodyText, and ifUnmodifiedSince;
// any missing piece is rejected with 400 invalid_body before the command
// runs (the BFF is the only legitimate caller and must always supply them).
func (s *Server) handleSaveVisualTemplate(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	// Template name + kind appear on the browser-facing wire shape (see the
	// contracts doc) but are immutable post-create — the handler decodes
	// only the editable pieces.
	var req struct {
		Subject           string          `json:"subject"`
		BodyDocRaw        json.RawMessage `json:"bodyDoc"`
		BodyHTML          string          `json:"bodyHtml"`
		BodyText          string          `json:"bodyText"`
		ThemeRaw          json.RawMessage `json:"theme"`
		IfUnmodifiedSince time.Time       `json:"ifUnmodifiedSince"`
	}
	if err := decodeJSON(r, &req); err != nil {
		metrics.VisualSavesTotal.WithLabelValues("template", "invalid_body", "false").Inc()
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	doc, theme, ok := s.decodeVisualPayload(w, req.BodyDocRaw, req.ThemeRaw)
	if !ok {
		metrics.VisualSavesTotal.WithLabelValues("template", "invalid_doc", "false").Inc()
		return
	}
	if req.BodyHTML == "" || req.BodyText == "" {
		metrics.VisualSavesTotal.WithLabelValues("template", "invalid_body", "false").Inc()
		writeError(w, http.StatusBadRequest, "invalid_body", "bodyHtml and bodyText are required")
		return
	}
	if req.IfUnmodifiedSince.IsZero() {
		metrics.VisualSavesTotal.WithLabelValues("template", "invalid_body", "false").Inc()
		writeError(w, http.StatusBadRequest, "invalid_body", "ifUnmodifiedSince is required")
		return
	}
	templateID := chi.URLParam(r, "id")
	res, err := s.campaign.Commands.SaveVisualTemplate.Handle(r.Context(), campaigncommand.SaveVisualTemplate{
		TenantID: ws.ID, TemplateID: templateID,
		Subject:           req.Subject,
		Doc:               doc,
		BodyHTML:          req.BodyHTML,
		BodyText:          req.BodyText,
		PinnedTheme:       theme,
		DocJSON:           req.BodyDocRaw,
		ThemeJSON:         req.ThemeRaw,
		IfUnmodifiedSince: req.IfUnmodifiedSince,
	})
	if err != nil {
		if errors.Is(err, campaigndomain.ErrStaleRow) {
			metrics.VisualSavesTotal.WithLabelValues("template", "stale_row", "false").Inc()
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":            "stale_row",
				"kind":             "stale_row",
				"currentUpdatedAt": res.CurrentUpdatedAt,
			})
			return
		}
		metrics.VisualSavesTotal.WithLabelValues("template", saveVisualErrorResult(err), "false").Inc()
		s.fail(w, "save visual template", err)
		return
	}
	view, err := s.campaign.Queries.GetTemplate.Handle(r.Context(), campaignquery.GetTemplate{
		TenantID: ws.ID, TemplateID: templateID,
	})
	if err != nil {
		metrics.VisualSavesTotal.WithLabelValues("template", "error", "false").Inc()
		s.fail(w, "save visual template", err)
		return
	}
	warningsPresent := "false"
	if len(res.Warnings) > 0 {
		warningsPresent = "true"
	}
	metrics.VisualSavesTotal.WithLabelValues("template", "ok", warningsPresent).Inc()
	s.recordAudit(r.Context(), "template.save_visual", templateID, map[string]any{
		"warnings_count": len(res.Warnings),
	})
	s.logEvent(r.Context(), "template.save_visual",
		slog.String("template_id", templateID),
		slog.Int("warnings_count", len(res.Warnings)),
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"template":  view,
		"warnings":  res.Warnings,
		"updatedAt": res.UpdatedAt,
	})
}

// handleConvertCampaignToVisual is the Go tail of the
// POST /campaigns/{id}/convert-to-visual endpoint (per
// specs/014-visual-email-editor/contracts/tenant-api.md). It runs the
// best-effort raw-HTML → VisualDoc converter (research.md § R6) over the
// campaign's persisted body_html and returns the candidate doc plus any
// rawhtml-fallback warnings the operator should review. The endpoint does
// not persist — the operator opens the candidate doc in the visual editor
// and saves through the regular PUT /campaigns/{id}/visual.
func (s *Server) handleConvertCampaignToVisual(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	res, err := s.campaign.Commands.ConvertCampaignToVisual.Handle(r.Context(),
		campaigncommand.ConvertCampaignToVisual{
			TenantID:   ws.ID,
			CampaignID: chi.URLParam(r, "id"),
		})
	if err != nil {
		s.fail(w, "convert campaign to visual", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bodyDoc":  res.BodyDoc,
		"warnings": res.Warnings,
	})
}

// handleOptOutCampaignVisual is the Go tail of the
// POST /campaigns/{id}/opt-out-visual endpoint. Clears the campaign's
// body_doc and theme override so the row reverts to a code-only campaign
// per FR-029; body_html / body_text stay intact so the campaign remains
// sendable.
func (s *Server) handleOptOutCampaignVisual(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	if err := s.campaign.Commands.OptOutVisualCampaign.Handle(r.Context(),
		campaigncommand.OptOutVisualCampaign{
			TenantID:   ws.ID,
			CampaignID: chi.URLParam(r, "id"),
		}); err != nil {
		s.fail(w, "opt out of visual campaign", err)
		return
	}
	s.respondCampaign(w, r, ws.ID, chi.URLParam(r, "id"), http.StatusOK)
}

// handleConvertTemplateToVisual mirrors handleConvertCampaignToVisual for
// templates.
func (s *Server) handleConvertTemplateToVisual(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	res, err := s.campaign.Commands.ConvertTemplateToVisual.Handle(r.Context(),
		campaigncommand.ConvertTemplateToVisual{
			TenantID:   ws.ID,
			TemplateID: chi.URLParam(r, "id"),
		})
	if err != nil {
		s.fail(w, "convert template to visual", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bodyDoc":  res.BodyDoc,
		"warnings": res.Warnings,
	})
}

// handleOptOutTemplateVisual mirrors handleOptOutCampaignVisual for templates.
func (s *Server) handleOptOutTemplateVisual(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermCampaignsManage); !ok {
		return
	}
	if err := s.campaign.Commands.OptOutVisualTemplate.Handle(r.Context(),
		campaigncommand.OptOutVisualTemplate{
			TenantID:   ws.ID,
			TemplateID: chi.URLParam(r, "id"),
		}); err != nil {
		s.fail(w, "opt out of visual template", err)
		return
	}
	view, err := s.campaign.Queries.GetTemplate.Handle(r.Context(), campaignquery.GetTemplate{
		TenantID: ws.ID, TemplateID: chi.URLParam(r, "id"),
	})
	if err != nil {
		s.fail(w, "opt out of visual template", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
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
