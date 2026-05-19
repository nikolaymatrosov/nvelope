package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	tenantquery "github.com/nikolaymatrosov/nvelope/internal/tenant/app/query"
)

// resolvePublicTenant is middleware for the unauthenticated public-page routes
// under /t/{slug}/.... It resolves the slug to a workspace without a session
// or membership cross-check — public visitors have no credential — and places
// the workspace on the request context so downstream handlers run inside the
// correct tenant scope. An unknown slug renders the branded "not available"
// page rather than an error.
func (s *Server) resolvePublicTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		ws, err := s.tenant.Queries.LocateWorkspace.Handle(r.Context(),
			tenantquery.LocateWorkspace{Slug: slug})
		if err != nil {
			s.renderPublicNotFound(w, r.Context())
			return
		}
		ctx := context.WithValue(r.Context(), tenantCtxKey, ws)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
