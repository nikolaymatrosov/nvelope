// Generated-contract handlers for the audience context. Every method here
// implements one operation from api/openapi/tenant.yaml. Compared with the
// hand-written handlers they replace: the request body arrives already decoded
// and validated, permission failures are returned rather than written, and the
// response type is checked against the spec at compile time.
//
// The request validator's declared security is deliberately inert (see
// openapi_wiring.go), so authorization is NOT structural here: every method
// must call requirePerm or requireListPerm itself, exactly as the hand-written
// handlers did.
package api

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/api/openapitenant"
	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// ListLists serves GET /lists.
func (s *Server) ListLists(ctx context.Context,
	req openapitenant.ListListsRequestObject) (openapitenant.ListListsResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermListsGet); err != nil {
		return nil, err
	}
	page, err := s.audience.Queries.ListLists.Handle(ctx, audiencequery.ListLists{
		TenantID: ws.ID, Page: pageFrom(req.Params.Limit, req.Params.Offset),
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.ListLists200JSONResponse(listPageToAPI(page)), nil
}

// CreateList serves POST /lists.
func (s *Server) CreateList(ctx context.Context,
	req openapitenant.CreateListRequestObject) (openapitenant.CreateListResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermListsManage); err != nil {
		return nil, err
	}
	res, err := s.audience.Commands.CreateList.Handle(ctx, audiencecommand.CreateList{
		TenantID:    ws.ID,
		Name:        req.Body.Name,
		Description: derefString(req.Body.Description),
		Visibility:  string(derefVisibility(req.Body.Visibility)),
		OptIn:       string(derefOptIn(req.Body.OptIn)),
		Tags:        derefStrings(req.Body.Tags),
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.CreateList201JSONResponse{Id: res.ListID}, nil
}

// GetList serves GET /lists/{id}.
func (s *Server) GetList(ctx context.Context,
	req openapitenant.GetListRequestObject) (openapitenant.GetListResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requireListPerm(ctx, iamdomain.PermListsGet, req.Id); err != nil {
		return nil, err
	}
	view, err := s.audience.Queries.GetList.Handle(ctx, audiencequery.GetList{
		TenantID: ws.ID, ListID: req.Id,
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.GetList200JSONResponse(listViewToAPI(view)), nil
}

// UpdateList serves PUT /lists/{id}.
func (s *Server) UpdateList(ctx context.Context,
	req openapitenant.UpdateListRequestObject) (openapitenant.UpdateListResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requireListPerm(ctx, iamdomain.PermListsManage, req.Id); err != nil {
		return nil, err
	}
	if err := s.audience.Commands.UpdateList.Handle(ctx, audiencecommand.UpdateList{
		TenantID:    ws.ID,
		ListID:      req.Id,
		Name:        req.Body.Name,
		Description: derefString(req.Body.Description),
		Visibility:  string(derefVisibility(req.Body.Visibility)),
		OptIn:       string(derefOptIn(req.Body.OptIn)),
		Tags:        derefStrings(req.Body.Tags),
	}); err != nil {
		return nil, err
	}
	return openapitenant.UpdateList204Response{}, nil
}

// DeleteList serves DELETE /lists/{id}.
func (s *Server) DeleteList(ctx context.Context,
	req openapitenant.DeleteListRequestObject) (openapitenant.DeleteListResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requireListPerm(ctx, iamdomain.PermListsManage, req.Id); err != nil {
		return nil, err
	}
	if err := s.audience.Commands.DeleteList.Handle(ctx, audiencecommand.DeleteList{
		TenantID: ws.ID, ListID: req.Id,
	}); err != nil {
		return nil, err
	}
	return openapitenant.DeleteList204Response{}, nil
}
