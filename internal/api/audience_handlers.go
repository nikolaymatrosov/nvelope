package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// pageFromRequest reads limit/offset query parameters into a domain.Page.
func pageFromRequest(r *http.Request) domain.Page {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	return domain.Page{Limit: limit, Offset: offset}
}

func (s *Server) handleCreateList(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermListsManage); !ok {
		return
	}
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Visibility  string   `json:"visibility"`
		OptIn       string   `json:"optin"`
		Tags        []string `json:"tags"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if req.Visibility == "" {
		req.Visibility = string(domain.VisibilityPrivate)
	}
	if req.OptIn == "" {
		req.OptIn = string(domain.OptInSingle)
	}
	res, err := s.audience.Commands.CreateList.Handle(r.Context(), audiencecommand.CreateList{
		TenantID: ws.ID, Name: req.Name, Description: req.Description,
		Visibility: req.Visibility, OptIn: req.OptIn, Tags: req.Tags,
	})
	if err != nil {
		s.fail(w, "create list", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": res.ListID})
}

func (s *Server) handleListLists(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermListsGet); !ok {
		return
	}
	page, err := s.audience.Queries.ListLists.Handle(r.Context(), audiencequery.ListLists{
		TenantID: ws.ID, Page: pageFromRequest(r),
	})
	if err != nil {
		s.fail(w, "list lists", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lists": page.Lists, "total": page.Total})
}

func (s *Server) handleGetList(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	listID := chi.URLParam(r, "id")
	if _, ok := s.requireListPermission(w, r, iamdomain.PermListsGet, listID); !ok {
		return
	}
	view, err := s.audience.Queries.GetList.Handle(r.Context(), audiencequery.GetList{
		TenantID: ws.ID, ListID: listID,
	})
	if err != nil {
		s.fail(w, "get list", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"list": view})
}

func (s *Server) handleUpdateList(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	listID := chi.URLParam(r, "id")
	if _, ok := s.requireListPermission(w, r, iamdomain.PermListsManage, listID); !ok {
		return
	}
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Visibility  string   `json:"visibility"`
		OptIn       string   `json:"optin"`
		Tags        []string `json:"tags"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if req.Visibility == "" {
		req.Visibility = string(domain.VisibilityPrivate)
	}
	if req.OptIn == "" {
		req.OptIn = string(domain.OptInSingle)
	}
	if err := s.audience.Commands.UpdateList.Handle(r.Context(), audiencecommand.UpdateList{
		TenantID: ws.ID, ListID: listID, Name: req.Name,
		Description: req.Description, Visibility: req.Visibility, OptIn: req.OptIn, Tags: req.Tags,
	}); err != nil {
		s.fail(w, "update list", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteList(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	listID := chi.URLParam(r, "id")
	if _, ok := s.requireListPermission(w, r, iamdomain.PermListsManage, listID); !ok {
		return
	}
	if err := s.audience.Commands.DeleteList.Handle(r.Context(), audiencecommand.DeleteList{
		TenantID: ws.ID, ListID: listID,
	}); err != nil {
		s.fail(w, "delete list", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateSubscriber(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscribersManage); !ok {
		return
	}
	var req struct {
		Email      string         `json:"email"`
		Name       string         `json:"name"`
		Attributes map[string]any `json:"attributes"`
		ListIDs    []string       `json:"list_ids"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	res, err := s.audience.Commands.CreateSubscriber.Handle(r.Context(), audiencecommand.CreateSubscriber{
		TenantID: ws.ID, Email: req.Email, Name: req.Name,
		Attributes: req.Attributes, ListIDs: req.ListIDs,
	})
	if err != nil {
		s.fail(w, "create subscriber", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": res.SubscriberID})
}

func (s *Server) handleSearchSubscribers(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscribersGet); !ok {
		return
	}
	page, err := s.audience.Queries.SearchSubscribers.Handle(r.Context(), audiencequery.SearchSubscribers{
		TenantID: ws.ID, Query: r.URL.Query().Get("q"), Page: pageFromRequest(r),
	})
	if err != nil {
		s.fail(w, "search subscribers", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subscribers": page.Subscribers, "total": page.Total,
	})
}

func (s *Server) handleQuerySubscribers(w http.ResponseWriter, r *http.Request) {
	s.runSegmentQuery(w, r, false)
}

func (s *Server) handleCountSubscribers(w http.ResponseWriter, r *http.Request) {
	s.runSegmentQuery(w, r, true)
}

// runSegmentQuery decodes a segment query, validates it, and evaluates it —
// returning either a page of matching subscribers or just the total count.
func (s *Server) runSegmentQuery(w http.ResponseWriter, r *http.Request, countOnly bool) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscribersGet); !ok {
		return
	}
	var req struct {
		Segment domain.Node `json:"segment"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	segment, err := domain.NewSegment(req.Segment)
	if err != nil {
		s.fail(w, "query subscribers", err)
		return
	}
	page, err := s.audience.Queries.RunSegment.Handle(r.Context(), audiencequery.RunSegment{
		TenantID: ws.ID, Segment: *segment, Page: pageFromRequest(r), CountOnly: countOnly,
	})
	if err != nil {
		s.fail(w, "query subscribers", err)
		return
	}
	if countOnly {
		writeJSON(w, http.StatusOK, map[string]any{"total": page.Total})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subscribers": page.Subscribers, "total": page.Total,
	})
}

func (s *Server) handleGetSubscriber(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscribersGet); !ok {
		return
	}
	view, err := s.audience.Queries.GetSubscriber.Handle(r.Context(), audiencequery.GetSubscriber{
		TenantID: ws.ID, SubscriberID: chi.URLParam(r, "id"),
	})
	if err != nil {
		s.fail(w, "get subscriber", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscriber": view})
}

func (s *Server) handleUpdateSubscriber(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscribersManage); !ok {
		return
	}
	var req struct {
		Name       string         `json:"name"`
		Attributes map[string]any `json:"attributes"`
		State      string         `json:"state"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if req.State == "" {
		req.State = string(domain.StateEnabled)
	}
	if err := s.audience.Commands.UpdateSubscriber.Handle(r.Context(), audiencecommand.UpdateSubscriber{
		TenantID: ws.ID, SubscriberID: chi.URLParam(r, "id"), Name: req.Name,
		Attributes: req.Attributes, State: req.State,
	}); err != nil {
		s.fail(w, "update subscriber", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteSubscriber(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscribersManage); !ok {
		return
	}
	if err := s.audience.Commands.DeleteSubscriber.Handle(r.Context(), audiencecommand.DeleteSubscriber{
		TenantID: ws.ID, SubscriberID: chi.URLParam(r, "id"),
	}); err != nil {
		s.fail(w, "delete subscriber", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddToList(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	var req struct {
		ListID string `json:"list_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if _, ok := s.requireListPermission(w, r, iamdomain.PermSubscribersManage, req.ListID); !ok {
		return
	}
	if err := s.audience.Commands.AddToList.Handle(r.Context(), audiencecommand.AddToList{
		TenantID: ws.ID, SubscriberID: chi.URLParam(r, "id"), ListID: req.ListID,
	}); err != nil {
		s.fail(w, "add to list", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveFromList(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	listID := chi.URLParam(r, "listId")
	if _, ok := s.requireListPermission(w, r, iamdomain.PermSubscribersManage, listID); !ok {
		return
	}
	if err := s.audience.Commands.RemoveFromList.Handle(r.Context(), audiencecommand.RemoveFromList{
		TenantID: ws.ID, SubscriberID: chi.URLParam(r, "id"), ListID: listID,
	}); err != nil {
		s.fail(w, "remove from list", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChangeSubscription(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	listID := chi.URLParam(r, "listId")
	if _, ok := s.requireListPermission(w, r, iamdomain.PermSubscribersManage, listID); !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if err := s.audience.Commands.ChangeSubscriptionState.Handle(r.Context(),
		audiencecommand.ChangeSubscriptionState{
			TenantID: ws.ID, SubscriberID: chi.URLParam(r, "id"),
			ListID: listID, Status: req.Status,
		}); err != nil {
		s.fail(w, "change subscription", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
