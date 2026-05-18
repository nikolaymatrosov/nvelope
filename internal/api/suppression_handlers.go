package api

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	deliverabilitycommand "github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	deliverabilityquery "github.com/nikolaymatrosov/nvelope/internal/deliverability/app/query"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// handleListSuppressions returns a page of the tenant's suppression list.
func (s *Server) handleListSuppressions(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSendingGet); !ok {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	page, err := s.deliverability.Queries.ListSuppressions.Handle(r.Context(),
		deliverabilityquery.ListSuppressions{
			TenantID:  ws.ID,
			Cursor:    r.URL.Query().Get("cursor"),
			Limit:     limit,
			Reason:    r.URL.Query().Get("reason"),
			EmailLike: r.URL.Query().Get("email"),
		})
	if err != nil {
		s.fail(w, "list suppressions", err)
		return
	}
	items := make([]map[string]any, 0, len(page.Items))
	for _, it := range page.Items {
		items = append(items, map[string]any{
			"email":        it.Email,
			"reason":       it.Reason,
			"suppressedAt": it.SuppressedAt,
			"note":         it.Note,
		})
	}
	var nextCursor any
	if page.NextCursor != "" {
		nextCursor = page.NextCursor
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": nextCursor})
}

// handleAddSuppression manually adds an address to the suppression list.
func (s *Server) handleAddSuppression(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermSendingManage)
	if !ok {
		return
	}
	var req struct {
		Email string `json:"email"`
		Note  string `json:"note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.deliverability.Commands.AddSuppression.Handle(r.Context(),
		deliverabilitycommand.AddSuppression{
			TenantID: ws.ID, ActorID: principal.ActorID(), Email: req.Email, Note: req.Note,
		}); err != nil {
		s.fail(w, "add suppression", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"email": req.Email, "reason": "manual", "note": req.Note,
	})
}

// handleRemoveSuppression removes an address from the suppression list.
func (s *Server) handleRemoveSuppression(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermSendingManage)
	if !ok {
		return
	}
	email, err := url.PathUnescape(chi.URLParam(r, "email"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "the email path segment is malformed")
		return
	}
	if err := s.deliverability.Commands.RemoveSuppression.Handle(r.Context(),
		deliverabilitycommand.RemoveSuppression{
			TenantID: ws.ID, ActorID: principal.ActorID(), Email: email,
		}); err != nil {
		s.fail(w, "remove suppression", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetBounceSettings returns the tenant's bounce-action configuration.
func (s *Server) handleGetBounceSettings(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSendingGet); !ok {
		return
	}
	view, err := s.deliverability.Queries.GetBounceSettings.Handle(r.Context(),
		deliverabilityquery.GetBounceSettings{TenantID: ws.ID})
	if err != nil {
		s.fail(w, "get bounce settings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"suppressOnHardBounce": view.SuppressOnHardBounce,
		"suppressOnComplaint":  view.SuppressOnComplaint,
	})
}

// handleUpdateBounceSettings updates the tenant's bounce-action configuration.
func (s *Server) handleUpdateBounceSettings(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermSendingManage)
	if !ok {
		return
	}
	var req struct {
		SuppressOnHardBounce bool `json:"suppressOnHardBounce"`
		SuppressOnComplaint  bool `json:"suppressOnComplaint"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.deliverability.Commands.UpdateBounceSettings.Handle(r.Context(),
		deliverabilitycommand.UpdateBounceSettings{
			TenantID: ws.ID, ActorID: principal.ActorID(),
			SuppressHardBounce: req.SuppressOnHardBounce,
			SuppressComplaint:  req.SuppressOnComplaint,
		}); err != nil {
		s.fail(w, "update bounce settings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"suppressOnHardBounce": req.SuppressOnHardBounce,
		"suppressOnComplaint":  req.SuppressOnComplaint,
	})
}
