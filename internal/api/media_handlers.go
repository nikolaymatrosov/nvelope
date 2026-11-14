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
		mediaquery.ListAssets{TenantID: ws.ID, TenantSlug: ws.Slug})
	if err != nil {
		s.fail(w, r, "list media", err)
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
		s.fail(w, r, "upload media", mediadomain.ErrMediaTooLarge)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.fail(w, r, "upload media", mediadomain.ErrEmptyUpload)
		return
	}
	defer func() { _ = file.Close() }()

	result, err := s.media.Commands.UploadAsset.Handle(r.Context(), mediacommand.UploadAsset{
		TenantID:   ws.ID,
		TenantSlug: ws.Slug,
		Filename:   header.Filename,
		Body:       file,
		UploadedBy: principal.ActorID(),
	})
	if err != nil {
		s.fail(w, r, "upload media", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       result.AssetID,
		"url":      result.URL,
		"filename": result.Filename,
		"width":    result.Width,
		"height":   result.Height,
	})
}

// handleMediaContent redirects a member to a short-lived presigned read of
// one library asset. Library URLs in drafts point here, so the editor
// canvas, the media picker, and a browser tab show private assets without
// making them public.
func (s *Server) handleMediaContent(w http.ResponseWriter, r *http.Request) {
	ws := tenantFromContext(r.Context())
	if _, ok := s.requirePermission(w, r, iamdomain.PermMediaGet); !ok {
		return
	}
	target, err := s.media.Queries.AssetContentURL.Handle(r.Context(), mediaquery.AssetContentURL{
		TenantID: ws.ID, AssetID: chi.URLParam(r, "id"),
	})
	if err != nil {
		s.fail(w, r, "media content", err)
		return
	}
	// Cached for 10 minutes, under the 15-minute signature (ContentURLTTL).
	w.Header().Set("Cache-Control", "private, max-age=600")
	http.Redirect(w, r, target, http.StatusFound)
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
		s.fail(w, r, "delete media", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
