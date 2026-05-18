package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	sendingcommand "github.com/nikolaymatrosov/nvelope/internal/sending/app/command"
	sendingquery "github.com/nikolaymatrosov/nvelope/internal/sending/app/query"
)

func (s *Server) handleAddSendingDomain(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSendingManage); !ok {
		return
	}
	var req struct {
		Domain string `json:"domain"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	res, err := s.sending.Commands.AddDomain.Handle(r.Context(), sendingcommand.AddDomain{
		TenantID: ws.ID, Domain: req.Domain,
	})
	if err != nil {
		s.fail(w, "add sending domain", err)
		return
	}
	view, err := s.sending.Queries.GetDomain.Handle(r.Context(), sendingquery.GetDomain{
		TenantID: ws.ID, DomainID: res.DomainID,
	})
	if err != nil {
		s.fail(w, "add sending domain", err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *Server) handleListSendingDomains(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSendingGet); !ok {
		return
	}
	views, err := s.sending.Queries.ListDomains.Handle(r.Context(), sendingquery.ListDomains{
		TenantID: ws.ID,
	})
	if err != nil {
		s.fail(w, "list sending domains", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": views})
}

func (s *Server) handleGetSendingDomain(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSendingGet); !ok {
		return
	}
	view, err := s.sending.Queries.GetDomain.Handle(r.Context(), sendingquery.GetDomain{
		TenantID: ws.ID, DomainID: chi.URLParam(r, "id"),
	})
	if err != nil {
		s.fail(w, "get sending domain", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleRecheckSendingDomain(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSendingManage); !ok {
		return
	}
	if err := s.sending.Commands.RecheckDomain.Handle(r.Context(), sendingcommand.RecheckDomain{
		TenantID: ws.ID, DomainID: chi.URLParam(r, "id"),
	}); err != nil {
		s.fail(w, "recheck sending domain", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "pending"})
}
