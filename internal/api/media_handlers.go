package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	mediacommand "github.com/nikolaymatrosov/nvelope/internal/media/app/command"
	mediaquery "github.com/nikolaymatrosov/nvelope/internal/media/app/query"
	mediadomain "github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// handleListMedia returns the tenant's media library, newest first.
func (s *Server) handleListMedia(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermMediaGet); !ok {
		return
	}
	items, err := s.media.Queries.ListAssets.Handle(r.Context(),
		mediaquery.ListAssets{TenantID: ws.ID})
	if err != nil {
		s.fail(w, "list media", err)
		return
	}
	if items == nil {
		items = []mediaquery.AssetView{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleUploadMedia accepts one multipart-encoded file and persists it.
func (s *Server) handleUploadMedia(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	principal, ok := s.requirePermission(w, r, iamdomain.PermMediaManage)
	if !ok {
		return
	}
	// Cap the request body at the configured size + a small multipart slack so
	// an oversized upload is rejected at the transport boundary too, not just
	// in the domain constructor.
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MediaMaxBytes+4096)
	if err := r.ParseMultipartForm(s.cfg.MediaMaxBytes + 4096); err != nil {
		s.fail(w, "upload media", mediadomain.ErrMediaTooLarge)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.fail(w, "upload media", mediadomain.ErrEmptyUpload)
		return
	}
	defer func() { _ = file.Close() }()

	contentType := header.Header.Get("Content-Type")
	result, err := s.media.Commands.UploadAsset.Handle(r.Context(), mediacommand.UploadAsset{
		TenantID:    ws.ID,
		Filename:    header.Filename,
		ContentType: contentType,
		SizeBytes:   header.Size,
		Body:        file,
		UploadedBy:  principal.ActorID(),
	})
	if err != nil {
		s.fail(w, "upload media", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         result.AssetID,
		"public_url": result.PublicURL,
		"filename":   header.Filename,
	})
}

// handleDeleteMedia removes an asset's metadata row and its bytes.
func (s *Server) handleDeleteMedia(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermMediaManage); !ok {
		return
	}
	if err := s.media.Commands.DeleteAsset.Handle(r.Context(), mediacommand.DeleteAsset{
		TenantID: ws.ID,
		AssetID:  chi.URLParam(r, "id"),
	}); err != nil {
		s.fail(w, "delete media", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
