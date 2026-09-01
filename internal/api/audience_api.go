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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/nikolaymatrosov/nvelope/internal/api/openapitenant"
	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	audiencedomain "github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// maxUploadBytes bounds an import upload — a CSV of 50,000 subscribers is a
// few megabytes (research.md Decision 2).
const maxUploadBytes = 32 << 20

// uploadBodySlack is added to maxUploadBytes when bounding the *whole*
// POST /import request body (see limitImportUploadSize in openapi_wiring.go).
// It exists only to cover multipart framing — the boundary lines, the
// file part's Content-Disposition/Content-Type headers, and a handful of
// listIds parts — around the file bytes themselves; it is not a second
// truncation boundary, since StartImport independently rejects a file part
// over maxUploadBytes on its own (see the "file" case below) regardless of
// where the whole-body cap sits. 4096 bytes is generous headroom for that
// framing — a boundary line plus headers for one part is on the order of a
// few hundred bytes even with a long filename — while staying small enough
// that a request just over the file-size limit is still caught by
// limitImportUploadSize rather than reaching the handler at all. Mirrors the
// same +4096 handleUploadMedia applies for the same reason.
const uploadBodySlack = 4096

// errUploadTooLarge is returned when StartImport's own read of the file part
// finds it over maxUploadBytes — the same slug and status
// limitImportUploadSize's http.MaxBytesReader produces for an oversize whole
// body (see statusForSlug in errmap.go), so a caller gets the same 413
// answer regardless of which of the two guards happens to catch them.
var errUploadTooLarge = apperr.NewIncorrectInput("payload_too_large",
	fmt.Sprintf("the uploaded file exceeds the %d byte limit", maxUploadBytes))

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
	// The fallbacks are the schema's own defaults; see deref's doc comment for
	// why they are re-applied here rather than in the domain.
	res, err := s.audience.Commands.CreateList.Handle(ctx, audiencecommand.CreateList{
		TenantID:    ws.ID,
		Name:        req.Body.Name,
		Description: deref(req.Body.Description, ""),
		Visibility:  string(deref(req.Body.Visibility, openapitenant.Private)),
		OptIn:       string(deref(req.Body.OptIn, openapitenant.Single)),
		Tags:        deref(req.Body.Tags, nil),
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
		Description: deref(req.Body.Description, ""),
		Visibility:  string(deref(req.Body.Visibility, openapitenant.Private)),
		OptIn:       string(deref(req.Body.OptIn, openapitenant.Single)),
		Tags:        deref(req.Body.Tags, nil),
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

// CreateSubscriber serves POST /subscribers.
func (s *Server) CreateSubscriber(ctx context.Context,
	req openapitenant.CreateSubscriberRequestObject) (openapitenant.CreateSubscriberResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermSubscribersManage); err != nil {
		return nil, err
	}
	res, err := s.audience.Commands.CreateSubscriber.Handle(ctx, audiencecommand.CreateSubscriber{
		TenantID:   ws.ID,
		Email:      req.Body.Email,
		Name:       deref(req.Body.Name, ""),
		Attributes: deref(req.Body.Attributes, nil),
		ListIDs:    deref(req.Body.ListIds, nil),
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.CreateSubscriber201JSONResponse{Id: res.SubscriberID}, nil
}

// SearchSubscribers serves GET /subscribers. An absent `q` is an empty
// search, which the query handler treats as "all subscribers".
func (s *Server) SearchSubscribers(ctx context.Context,
	req openapitenant.SearchSubscribersRequestObject) (openapitenant.SearchSubscribersResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermSubscribersGet); err != nil {
		return nil, err
	}
	page, err := s.audience.Queries.SearchSubscribers.Handle(ctx, audiencequery.SearchSubscribers{
		TenantID: ws.ID,
		Query:    deref(req.Params.Q, ""),
		Page:     pageFrom(req.Params.Limit, req.Params.Offset),
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.SearchSubscribers200JSONResponse(subscriberPageToAPI(page)), nil
}

// QuerySubscribers serves POST /subscribers/query. NewSegment validates the
// tree and returns a typed apperr on a malformed one, so the error is
// returned unwrapped and rendered as a 422 problem by the strict handler's
// error routing.
func (s *Server) QuerySubscribers(ctx context.Context,
	req openapitenant.QuerySubscribersRequestObject) (openapitenant.QuerySubscribersResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermSubscribersGet); err != nil {
		return nil, err
	}
	segment, err := audiencedomain.NewSegment(segmentNodeFromAPI(req.Body.Segment))
	if err != nil {
		return nil, err
	}
	page, err := s.audience.Queries.RunSegment.Handle(ctx, audiencequery.RunSegment{
		TenantID: ws.ID,
		Segment:  *segment,
		Page:     pageFrom(req.Params.Limit, req.Params.Offset),
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.QuerySubscribers200JSONResponse(subscriberPageToAPI(page)), nil
}

// CountSubscribers serves POST /subscribers/query/count — the same segment
// evaluation as QuerySubscribers, but with CountOnly set and no paging.
func (s *Server) CountSubscribers(ctx context.Context,
	req openapitenant.CountSubscribersRequestObject) (openapitenant.CountSubscribersResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermSubscribersGet); err != nil {
		return nil, err
	}
	segment, err := audiencedomain.NewSegment(segmentNodeFromAPI(req.Body.Segment))
	if err != nil {
		return nil, err
	}
	page, err := s.audience.Queries.RunSegment.Handle(ctx, audiencequery.RunSegment{
		TenantID:  ws.ID,
		Segment:   *segment,
		CountOnly: true,
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.CountSubscribers200JSONResponse{Total: page.Total}, nil
}

// GetSubscriber serves GET /subscribers/{id}.
func (s *Server) GetSubscriber(ctx context.Context,
	req openapitenant.GetSubscriberRequestObject) (openapitenant.GetSubscriberResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermSubscribersGet); err != nil {
		return nil, err
	}
	view, err := s.audience.Queries.GetSubscriber.Handle(ctx, audiencequery.GetSubscriber{
		TenantID: ws.ID, SubscriberID: req.Id,
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.GetSubscriber200JSONResponse(subscriberViewToAPI(view)), nil
}

// UpdateSubscriber serves PUT /subscribers/{id}. req.Body.State is a pointer,
// so an absent state defaults to enabled — the schema's own default.
func (s *Server) UpdateSubscriber(ctx context.Context,
	req openapitenant.UpdateSubscriberRequestObject) (openapitenant.UpdateSubscriberResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermSubscribersManage); err != nil {
		return nil, err
	}
	if err := s.audience.Commands.UpdateSubscriber.Handle(ctx, audiencecommand.UpdateSubscriber{
		TenantID:     ws.ID,
		SubscriberID: req.Id,
		Name:         deref(req.Body.Name, ""),
		Attributes:   deref(req.Body.Attributes, nil),
		State:        string(deref(req.Body.State, openapitenant.Enabled)),
	}); err != nil {
		return nil, err
	}
	return openapitenant.UpdateSubscriber204Response{}, nil
}

// DeleteSubscriber serves DELETE /subscribers/{id}.
func (s *Server) DeleteSubscriber(ctx context.Context,
	req openapitenant.DeleteSubscriberRequestObject) (openapitenant.DeleteSubscriberResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermSubscribersManage); err != nil {
		return nil, err
	}
	if err := s.audience.Commands.DeleteSubscriber.Handle(ctx, audiencecommand.DeleteSubscriber{
		TenantID: ws.ID, SubscriberID: req.Id,
	}); err != nil {
		return nil, err
	}
	return openapitenant.DeleteSubscriber204Response{}, nil
}

// AddSubscriberToList serves POST /subscribers/{id}/lists. The list id comes
// from the request body, not the path, so the per-list permission check reads
// req.Body.ListId — mirroring the handler it replaces. Checking the path's id
// instead would authorize against the wrong list (the path only carries the
// subscriber id here).
func (s *Server) AddSubscriberToList(ctx context.Context,
	req openapitenant.AddSubscriberToListRequestObject) (openapitenant.AddSubscriberToListResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requireListPerm(ctx, iamdomain.PermSubscribersManage, req.Body.ListId); err != nil {
		return nil, err
	}
	if err := s.audience.Commands.AddToList.Handle(ctx, audiencecommand.AddToList{
		TenantID: ws.ID, SubscriberID: req.Id, ListID: req.Body.ListId,
	}); err != nil {
		return nil, err
	}
	return openapitenant.AddSubscriberToList204Response{}, nil
}

// RemoveSubscriberFromList serves DELETE /subscribers/{id}/lists/{listId}.
// Here, unlike AddSubscriberToList, the list id is a path parameter.
func (s *Server) RemoveSubscriberFromList(ctx context.Context,
	req openapitenant.RemoveSubscriberFromListRequestObject) (openapitenant.RemoveSubscriberFromListResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requireListPerm(ctx, iamdomain.PermSubscribersManage, req.ListId); err != nil {
		return nil, err
	}
	if err := s.audience.Commands.RemoveFromList.Handle(ctx, audiencecommand.RemoveFromList{
		TenantID: ws.ID, SubscriberID: req.Id, ListID: req.ListId,
	}); err != nil {
		return nil, err
	}
	return openapitenant.RemoveSubscriberFromList204Response{}, nil
}

// ChangeSubscription serves PUT /subscribers/{id}/lists/{listId}. Like
// RemoveSubscriberFromList, the list id is a path parameter.
func (s *Server) ChangeSubscription(ctx context.Context,
	req openapitenant.ChangeSubscriptionRequestObject) (openapitenant.ChangeSubscriptionResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requireListPerm(ctx, iamdomain.PermSubscribersManage, req.ListId); err != nil {
		return nil, err
	}
	if err := s.audience.Commands.ChangeSubscriptionState.Handle(ctx, audiencecommand.ChangeSubscriptionState{
		TenantID: ws.ID, SubscriberID: req.Id, ListID: req.ListId,
		Status: string(req.Body.Status),
	}); err != nil {
		return nil, err
	}
	return openapitenant.ChangeSubscription204Response{}, nil
}

// StartImport serves POST /import. oapi-codegen models a multipart/form-data
// body as a *multipart.Reader rather than decoding it into a struct, so the
// file and the repeated listIds parts are read here directly instead of
// through r.ParseMultipartForm. Each part is bounded the same way the
// hand-written handler bounded r.FormFile's reader — the 32 MiB cap on the
// file part, a much smaller one on each listIds value — except that the file
// part is read one byte past its limit so an oversize file can be told apart
// from one that exactly fills it: io.LimitReader alone stops silently at the
// limit with err == nil, which would otherwise import a truncated file
// without any indication rows were dropped. limitImportUploadSize
// (openapi_wiring.go) catches most oversize uploads earlier, ahead of the
// permission check, but its whole-body cap carries a few KiB of multipart
// framing slack (uploadBodySlack) that a file just over maxUploadBytes can
// still land inside — this check is what closes that gap.
func (s *Server) StartImport(ctx context.Context,
	req openapitenant.StartImportRequestObject) (openapitenant.StartImportResponseObject, error) {
	ws := tenantFromContext(ctx)
	principal, err := s.requirePerm(ctx, iamdomain.PermSubscribersImport)
	if err != nil {
		return nil, err
	}

	var (
		fileName string
		fileData []byte
		listIDs  []string
	)
	for {
		part, perr := req.Body.NextPart()
		if errors.Is(perr, io.EOF) {
			break
		}
		if perr != nil {
			return nil, apperr.NewIncorrectInput("invalid_upload", "expected a multipart/form-data upload")
		}
		switch part.FormName() {
		case "file":
			fileName = part.FileName()
			fileData, err = io.ReadAll(io.LimitReader(part, maxUploadBytes+1))
			if err == nil && len(fileData) > maxUploadBytes {
				fileData = nil
				err = errUploadTooLarge
			}
		case "listIds":
			var value []byte
			value, err = io.ReadAll(io.LimitReader(part, 1<<12))
			if err == nil {
				listIDs = append(listIDs, string(value))
			}
		}
		closeErr := part.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	if len(fileData) == 0 {
		return nil, apperr.NewIncorrectInput("missing_file", "a CSV or ZIP file is required")
	}

	res, err := s.audience.Commands.StartImport.Handle(ctx, audiencecommand.StartImport{
		TenantID: ws.ID, RequestedBy: principal.ActorID(), FileName: fileName,
		FileBytes: fileData, TargetListIDs: listIDs,
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.StartImport202JSONResponse{JobId: res.JobID}, nil
}

// StartExport serves POST /export.
func (s *Server) StartExport(ctx context.Context,
	req openapitenant.StartExportRequestObject) (openapitenant.StartExportResponseObject, error) {
	ws := tenantFromContext(ctx)
	principal, err := s.requirePerm(ctx, iamdomain.PermSubscribersExport)
	if err != nil {
		return nil, err
	}
	var segment *audiencedomain.Segment
	if req.Body.Segment != nil {
		seg, serr := audiencedomain.NewSegment(segmentNodeFromAPI(*req.Body.Segment))
		if serr != nil {
			return nil, serr
		}
		segment = seg
	}
	res, err := s.audience.Commands.StartExport.Handle(ctx, audiencecommand.StartExport{
		TenantID: ws.ID, RequestedBy: principal.ActorID(),
		Selection: string(req.Body.Selection), ListID: deref(req.Body.ListId, ""), Segment: segment,
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.StartExport202JSONResponse{JobId: res.JobID}, nil
}

// GetJobStatus serves GET /jobs/{id}. It is readable by anyone who may import
// or export — the job list is shared between both flows — which is an OR that
// requirePerm cannot express, so the check is inlined here rather than routed
// through it, mirroring what the hand-written handler did.
func (s *Server) GetJobStatus(ctx context.Context,
	req openapitenant.GetJobStatusRequestObject) (openapitenant.GetJobStatusResponseObject, error) {
	ws := tenantFromContext(ctx)
	p, ok := principalFromContext(ctx)
	if !ok {
		return nil, apperr.NewAuthorization("unauthenticated", "a workspace session is required")
	}
	if !p.Can(iamdomain.PermSubscribersImport) && !p.Can(iamdomain.PermSubscribersExport) {
		return nil, iamdomain.Forbidden(iamdomain.PermSubscribersImport)
	}
	view, err := s.audience.Queries.GetJobStatus.Handle(ctx, audiencequery.GetJobStatus{
		TenantID: ws.ID, JobID: req.Id,
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.GetJobStatus200JSONResponse(jobStatusToAPI(view)), nil
}

// DownloadExport serves GET /jobs/{id}/download. The generated response
// object carries the body reader, its length, and the Content-Disposition
// header directly, so the strict server writes the non-JSON text/csv response
// without ever touching an http.ResponseWriter.
func (s *Server) DownloadExport(ctx context.Context,
	req openapitenant.DownloadExportRequestObject) (openapitenant.DownloadExportResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermSubscribersExport); err != nil {
		return nil, err
	}
	res, err := s.audience.Queries.ExportFile.Handle(ctx, audiencequery.ExportFile{
		TenantID: ws.ID, JobID: req.Id,
	})
	if err != nil {
		return nil, err
	}
	disposition := `attachment; filename="` + res.FileName + `"`
	return openapitenant.DownloadExport200TextcsvResponse{
		Body:          bytes.NewReader(res.Data),
		ContentLength: int64(len(res.Data)),
		Headers:       openapitenant.DownloadExport200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}
