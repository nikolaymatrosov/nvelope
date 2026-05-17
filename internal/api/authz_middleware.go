package api

import (
	"context"
	"net/http"
	"strings"

	iamquery "github.com/nikolaymatrosov/nvelope/internal/iam/app/query"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// workspaceCookie is the name of the tenant-plane workspace session cookie. It
// is distinct from the control-plane session cookie and is path-scoped to one
// tenant.
const workspaceCookie = "nv_workspace"

// principalCtxKey holds the resolved Principal in the request context.
const principalCtxKey ctxKey = 100

// authz is middleware for guarded tenant-scoped routes. It resolves the
// request's credential — the workspace session cookie or an API-key bearer
// token — into a Principal and attaches it to the request context. A request
// with no valid credential is rejected before the handler runs.
func (s *Server) authz(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws := tenantFromContext(r.Context())
		principal, err := s.resolvePrincipal(r, ws.ID)
		if err != nil {
			s.fail(w, "authorize", err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalCtxKey, principal)))
	})
}

// resolvePrincipal resolves the request's credential into a Principal. An
// `Authorization: Bearer <api-key>` header is tried first; otherwise the
// workspace session cookie is used. A totp-pending session resolves to no
// principal (AuthenticatePrincipal returns ErrTOTPRequired), so a guarded route
// stays closed until the two-factor challenge is met.
func (s *Server) resolvePrincipal(r *http.Request, tenantID string) (iamdomain.Principal, error) {
	if key, ok := bearerToken(r); ok {
		return s.iam.Queries.AuthenticateAPIKey.Handle(r.Context(),
			iamquery.AuthenticateAPIKey{TenantID: tenantID, RawKey: key})
	}
	c, err := r.Cookie(workspaceCookie)
	if err != nil || c.Value == "" {
		return iamdomain.Principal{}, iamdomain.ErrUnauthenticated
	}
	return s.iam.Queries.AuthenticatePrincipal.Handle(r.Context(),
		iamquery.AuthenticatePrincipal{TenantID: tenantID, Token: c.Value})
}

// bearerToken extracts an API key from an `Authorization: Bearer <token>`
// header, reporting whether one was present.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	return token, token != ""
}

// principalFromContext returns the Principal resolved by the authz middleware.
func principalFromContext(ctx context.Context) (iamdomain.Principal, bool) {
	p, ok := ctx.Value(principalCtxKey).(iamdomain.Principal)
	return p, ok
}

// requirePermission resolves the request's Principal and checks a tenant-level
// permission. It writes a 403 and returns false when the permission is absent.
func (s *Server) requirePermission(w http.ResponseWriter, r *http.Request,
	required iamdomain.Permission) (iamdomain.Principal, bool) {
	p, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "a workspace session is required")
		return iamdomain.Principal{}, false
	}
	if !p.Can(required) {
		s.fail(w, "authorize", iamdomain.Forbidden(required))
		return iamdomain.Principal{}, false
	}
	return p, true
}

// requireListPermission checks a permission that targets a specific list,
// honouring per-list role grants.
func (s *Server) requireListPermission(w http.ResponseWriter, r *http.Request,
	required iamdomain.Permission, listID string) (iamdomain.Principal, bool) {
	p, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "a workspace session is required")
		return iamdomain.Principal{}, false
	}
	if !p.CanOnList(required, listID) {
		s.fail(w, "authorize", iamdomain.Forbidden(required))
		return iamdomain.Principal{}, false
	}
	return p, true
}
