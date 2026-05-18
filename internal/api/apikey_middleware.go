package api

import (
	"context"
	"net/http"

	iamquery "github.com/nikolaymatrosov/nvelope/internal/iam/app/query"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// apiKeyAuth is middleware for the API-key-authenticated transactional route.
// It reads an `Authorization: Bearer <key>` header, resolves it against the
// resolved tenant via the Phase 2 AuthenticateAPIKey query, and confirms the
// key carries the transactional-send scope. A request with no valid, scoped
// key is rejected before the handler runs.
func (s *Server) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws := tenantFromContext(r.Context())

		key, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized",
				"an API key is required")
			return
		}
		principal, err := s.iam.Queries.AuthenticateAPIKey.Handle(r.Context(),
			iamquery.AuthenticateAPIKey{TenantID: ws.ID, RawKey: key})
		if err != nil {
			s.fail(w, "api key auth", err)
			return
		}
		if !principal.Can(iamdomain.PermTransactionalSend) {
			s.fail(w, "api key auth", iamdomain.Forbidden(iamdomain.PermTransactionalSend))
			return
		}
		next.ServeHTTP(w, r.WithContext(
			context.WithValue(r.Context(), principalCtxKey, principal)))
	})
}
