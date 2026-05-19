package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	audiencedomain "github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// subscriptionPageRequest is the JSON body for creating or updating a public
// subscription page.
type subscriptionPageRequest struct {
	Slug            string                     `json:"slug"`
	Title           string                     `json:"title"`
	TargetListIDs   []string                   `json:"target_list_ids"`
	Fields          []audiencedomain.FormField `json:"fields"`
	SendingDomainID string                     `json:"sending_domain_id"`
	FromName        string                     `json:"from_name"`
	FromLocalPart   string                     `json:"from_local_part"`
	Active          bool                       `json:"active"`
}

// handleListSubscriptionPages returns every subscription page of the tenant.
func (s *Server) handleListSubscriptionPages(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscriptionPagesManage); !ok {
		return
	}
	views, err := s.audience.Queries.ListSubscriptionPages.Handle(r.Context(),
		audiencequery.ListSubscriptionPages{TenantID: ws.ID})
	if err != nil {
		s.fail(w, "list subscription pages", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscription_pages": views})
}

// handleCreateSubscriptionPage creates a new subscription page.
func (s *Server) handleCreateSubscriptionPage(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscriptionPagesManage); !ok {
		return
	}
	var req subscriptionPageRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	res, err := s.audience.Commands.SaveSubscriptionPage.Handle(r.Context(),
		audiencecommand.SaveSubscriptionPage{
			TenantID:        ws.ID,
			Slug:            req.Slug,
			Title:           req.Title,
			TargetListIDs:   req.TargetListIDs,
			Fields:          req.Fields,
			SendingDomainID: req.SendingDomainID,
			FromName:        req.FromName,
			FromLocalPart:   req.FromLocalPart,
			Active:          true,
		})
	if err != nil {
		s.fail(w, "create subscription page", err)
		return
	}
	view, err := s.audience.Queries.GetSubscriptionPage.Handle(r.Context(),
		audiencequery.GetSubscriptionPage{TenantID: ws.ID, Slug: req.Slug})
	if err != nil {
		writeJSON(w, http.StatusCreated, map[string]string{"id": res.PageID})
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

// handleUpdateSubscriptionPage updates an existing subscription page.
func (s *Server) handleUpdateSubscriptionPage(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscriptionPagesManage); !ok {
		return
	}
	pageID := chi.URLParam(r, "id")
	var req subscriptionPageRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if _, err := s.audience.Commands.SaveSubscriptionPage.Handle(r.Context(),
		audiencecommand.SaveSubscriptionPage{
			TenantID:        ws.ID,
			PageID:          pageID,
			Slug:            req.Slug,
			Title:           req.Title,
			TargetListIDs:   req.TargetListIDs,
			Fields:          req.Fields,
			SendingDomainID: req.SendingDomainID,
			FromName:        req.FromName,
			FromLocalPart:   req.FromLocalPart,
			Active:          req.Active,
		}); err != nil {
		s.fail(w, "update subscription page", err)
		return
	}
	view, err := s.audience.Queries.GetSubscriptionPage.Handle(r.Context(),
		audiencequery.GetSubscriptionPage{TenantID: ws.ID, Slug: req.Slug})
	if err != nil {
		s.fail(w, "update subscription page", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
