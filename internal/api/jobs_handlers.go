package api

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	audiencedomain "github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// maxUploadBytes bounds an import upload — a CSV of 50,000 subscribers is a
// few megabytes (research.md Decision 2).
const maxUploadBytes = 32 << 20

func (s *Server) handleStartImport(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermSubscribersImport)
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload", "expected a multipart/form-data upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_file", "a CSV or ZIP file is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes))
	if err != nil {
		s.fail(w, "read upload", err)
		return
	}
	res, err := s.audience.Commands.StartImport.Handle(r.Context(), audiencecommand.StartImport{
		TenantID: ws.ID, RequestedBy: principal.ActorID(), FileName: header.Filename,
		FileBytes: data, TargetListIDs: r.MultipartForm.Value["list_ids"],
	})
	if err != nil {
		s.fail(w, "start import", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": res.JobID})
}

func (s *Server) handleStartExport(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermSubscribersExport)
	if !ok {
		return
	}
	var req struct {
		Selection string               `json:"selection"`
		ListID    string               `json:"list_id"`
		Segment   *audiencedomain.Node `json:"segment"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	var segment *audiencedomain.Segment
	if req.Segment != nil {
		seg, err := audiencedomain.NewSegment(*req.Segment)
		if err != nil {
			s.fail(w, "start export", err)
			return
		}
		segment = seg
	}
	res, err := s.audience.Commands.StartExport.Handle(r.Context(), audiencecommand.StartExport{
		TenantID: ws.ID, RequestedBy: principal.ActorID(),
		Selection: req.Selection, ListID: req.ListID, Segment: segment,
	})
	if err != nil {
		s.fail(w, "start export", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": res.JobID})
}

func (s *Server) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	p, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "a workspace session is required")
		return
	}
	if !p.Can(iamdomain.PermSubscribersImport) && !p.Can(iamdomain.PermSubscribersExport) {
		s.fail(w, "authorize", iamdomain.Forbidden(iamdomain.PermSubscribersImport))
		return
	}
	view, err := s.audience.Queries.GetJobStatus.Handle(r.Context(), audiencequery.GetJobStatus{
		TenantID: ws.ID, JobID: chi.URLParam(r, "id"),
	})
	if err != nil {
		s.fail(w, "job status", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": view})
}

func (s *Server) handleDownloadExport(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermSubscribersExport); !ok {
		return
	}
	res, err := s.audience.Queries.ExportFile.Handle(r.Context(), audiencequery.ExportFile{
		TenantID: ws.ID, JobID: chi.URLParam(r, "id"),
	})
	if err != nil {
		s.fail(w, "download export", err)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="`+res.FileName+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(res.Data)
}
