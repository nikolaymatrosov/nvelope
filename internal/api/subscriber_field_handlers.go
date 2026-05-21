package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
	"github.com/nikolaymatrosov/nvelope/internal/platform/metrics"
)

// subscriberFieldErrorResult maps a command error to the metric label used
// for the "result" dimension of SubscriberFieldMutationsTotal.
func subscriberFieldErrorResult(err error) string {
	if err == nil {
		return "ok"
	}
	if typed, ok := apperr.As(err); ok {
		switch typed.Category() {
		case apperr.Conflict:
			return "conflict"
		case apperr.IncorrectInput:
			return "invalid"
		case apperr.Forbidden, apperr.Authorization:
			return "forbidden"
		}
	}
	if errors.Is(err, iamdomain.ErrUnauthenticated) {
		return "forbidden"
	}
	return "error"
}

// handleListSubscriberFields returns the tenant's subscriber-field registry
// merged with the platform's built-in pseudo-rows. Any tenant member can read
// this list — the merge-tag picker and the Phase 6 subscription-page editor
// both consume it.
func (s *Server) handleListSubscriberFields(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := principalFromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "a workspace session is required")
		return
	}
	views, err := s.audience.Queries.ListFields.Handle(r.Context(),
		audiencequery.ListFields{TenantID: ws.ID})
	if err != nil {
		s.fail(w, "list subscriber fields", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fields": views})
}

func (s *Server) handleCreateSubscriberField(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscriberFieldsManage); !ok {
		return
	}
	var req struct {
		Slug         string `json:"slug"`
		DisplayName  string `json:"displayName"`
		Type         string `json:"type"`
		DefaultValue string `json:"defaultValue"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	res, err := s.audience.Commands.CreateField.Handle(r.Context(), audiencecommand.CreateField{
		TenantID: ws.ID, Slug: req.Slug, DisplayName: req.DisplayName,
		Type: req.Type, DefaultValue: req.DefaultValue,
		// New fields land at the end of the list; operators reorder via the
		// dedicated /order endpoint.
		Position: s.nextSubscriberFieldPosition(r.Context(), ws.ID),
	})
	if err != nil {
		metrics.SubscriberFieldMutationsTotal.WithLabelValues("create", subscriberFieldErrorResult(err)).Inc()
		s.fail(w, "create subscriber field", err)
		return
	}
	metrics.SubscriberFieldMutationsTotal.WithLabelValues("create", "ok").Inc()
	s.recordAudit(r.Context(), "subscriber_field.create", res.FieldID, map[string]any{
		"slug":         req.Slug,
		"display_name": req.DisplayName,
		"type":         req.Type,
	})
	s.logEvent(r.Context(), "subscriber_field.create",
		slog.String("field_id", res.FieldID),
		slog.String("slug", req.Slug),
	)
	s.respondSubscriberField(w, r, ws.ID, res.FieldID, http.StatusCreated)
}

// nextSubscriberFieldPosition returns the position a new tenant field should
// land at. It scans the existing custom (non-built-in) rows and appends after
// the highest position, so newly-created fields land at the end of the picker.
// Errors fall back to position 0 — the row will still persist, just at the
// top, which is benign.
func (s *Server) nextSubscriberFieldPosition(ctx context.Context, tenantID string) int {
	views, err := s.audience.Queries.ListFields.Handle(ctx,
		audiencequery.ListFields{TenantID: tenantID})
	if err != nil {
		return 0
	}
	next := 0
	for _, v := range views {
		if v.BuiltIn {
			continue
		}
		if v.Position+1 > next {
			next = v.Position + 1
		}
	}
	return next
}

func (s *Server) handleUpdateSubscriberField(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscriberFieldsManage); !ok {
		return
	}
	fieldID := chi.URLParam(r, "id")
	// Fetch the current row so unspecified PATCH fields preserve their values
	// and the immutable position is carried through (operators reorder via the
	// dedicated /order endpoint, not via PATCH on a single field).
	current, err := s.audience.Queries.GetField.Handle(r.Context(),
		audiencequery.GetField{TenantID: ws.ID, FieldID: fieldID})
	if err != nil {
		s.fail(w, "update subscriber field", err)
		return
	}
	var req struct {
		DisplayName  *string `json:"displayName"`
		Type         *string `json:"type"`
		DefaultValue *string `json:"defaultValue"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	merged := audiencecommand.UpdateField{
		TenantID: ws.ID, FieldID: fieldID,
		DisplayName:  current.DisplayName,
		Type:         current.Type,
		DefaultValue: current.DefaultValue,
		Position:     current.Position,
	}
	if req.DisplayName != nil {
		merged.DisplayName = *req.DisplayName
	}
	if req.Type != nil {
		merged.Type = *req.Type
	}
	if req.DefaultValue != nil {
		merged.DefaultValue = *req.DefaultValue
	}
	if err := s.audience.Commands.UpdateField.Handle(r.Context(), merged); err != nil {
		metrics.SubscriberFieldMutationsTotal.WithLabelValues("update", subscriberFieldErrorResult(err)).Inc()
		s.fail(w, "update subscriber field", err)
		return
	}
	metrics.SubscriberFieldMutationsTotal.WithLabelValues("update", "ok").Inc()
	changed := map[string]any{}
	if req.DisplayName != nil {
		changed["display_name"] = *req.DisplayName
	}
	if req.Type != nil {
		changed["type"] = *req.Type
	}
	if req.DefaultValue != nil {
		changed["default_value"] = *req.DefaultValue
	}
	s.recordAudit(r.Context(), "subscriber_field.update", fieldID, map[string]any{
		"changed": changed,
	})
	s.logEvent(r.Context(), "subscriber_field.update",
		slog.String("field_id", fieldID),
		slog.Int("changed_count", len(changed)),
	)
	s.respondSubscriberField(w, r, ws.ID, fieldID, http.StatusOK)
}

func (s *Server) handleDeleteSubscriberField(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscriberFieldsManage); !ok {
		return
	}
	fieldID := chi.URLParam(r, "id")
	if err := s.audience.Commands.DeleteField.Handle(r.Context(), audiencecommand.DeleteField{
		TenantID: ws.ID, FieldID: fieldID,
	}); err != nil {
		metrics.SubscriberFieldMutationsTotal.WithLabelValues("delete", subscriberFieldErrorResult(err)).Inc()
		s.fail(w, "delete subscriber field", err)
		return
	}
	metrics.SubscriberFieldMutationsTotal.WithLabelValues("delete", "ok").Inc()
	s.recordAudit(r.Context(), "subscriber_field.delete", fieldID, nil)
	s.logEvent(r.Context(), "subscriber_field.delete",
		slog.String("field_id", fieldID),
	)
	w.WriteHeader(http.StatusNoContent)
}

// handleReorderSubscriberFields applies a new display ordering across every
// tenant-defined field. The supplied id list MUST cover every custom field
// exactly once; built-in pseudo-rows are not included.
func (s *Server) handleReorderSubscriberFields(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscriberFieldsManage); !ok {
		return
	}
	var req struct {
		Order []string `json:"order"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.audience.Commands.ReorderFields.Handle(r.Context(), audiencecommand.ReorderFields{
		TenantID: ws.ID, IDs: req.Order,
	}); err != nil {
		metrics.SubscriberFieldMutationsTotal.WithLabelValues("reorder", subscriberFieldErrorResult(err)).Inc()
		s.fail(w, "reorder subscriber fields", err)
		return
	}
	metrics.SubscriberFieldMutationsTotal.WithLabelValues("reorder", "ok").Inc()
	s.recordAudit(r.Context(), "subscriber_field.reorder", "", map[string]any{
		"order": req.Order,
	})
	s.logEvent(r.Context(), "subscriber_field.reorder",
		slog.Int("order_len", len(req.Order)),
	)
	views, err := s.audience.Queries.ListFields.Handle(r.Context(),
		audiencequery.ListFields{TenantID: ws.ID})
	if err != nil {
		s.fail(w, "reorder subscriber fields", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fields": views})
}

// respondSubscriberField fetches and writes a single field view. Used by the
// POST and PATCH responses per the contract (return the persisted row).
func (s *Server) respondSubscriberField(w http.ResponseWriter, r *http.Request, tenantID, id string, status int) {
	view, err := s.audience.Queries.GetField.Handle(r.Context(),
		audiencequery.GetField{TenantID: tenantID, FieldID: id})
	if err != nil {
		s.fail(w, "get subscriber field", err)
		return
	}
	writeJSON(w, status, view)
}
