package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	authquery "github.com/nikolaymatrosov/nvelope/internal/auth/app/query"
	tenantquery "github.com/nikolaymatrosov/nvelope/internal/tenant/app/query"
)

// sessionCookie is the name of the platform session cookie.
const sessionCookie = "nv_session"

type ctxKey int

const (
	userCtxKey ctxKey = iota
	tenantCtxKey
)

// requireUser is middleware that resolves the session cookie to a user and
// stores it in the request context. Requests with no valid session are
// rejected with 401 before the handler runs.
func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.authenticate(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "a valid session is required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userCtxKey, user)))
	})
}

// authenticate resolves the session cookie to a user without rejecting the
// request. It is used both by requireUser and by routes that accept anonymous
// callers, such as accepting an invitation.
func (s *Server) authenticate(r *http.Request) (authquery.AuthenticatedUser, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return authquery.AuthenticatedUser{}, false
	}
	user, err := s.auth.Queries.AuthenticateSession.Handle(r.Context(),
		authquery.AuthenticateSession{RawToken: c.Value})
	if err != nil {
		return authquery.AuthenticatedUser{}, false
	}
	return user, true
}

// userFromContext returns the authenticated user stored by requireUser.
func userFromContext(ctx context.Context) (authquery.AuthenticatedUser, bool) {
	u, ok := ctx.Value(userCtxKey).(authquery.AuthenticatedUser)
	return u, ok
}

// resolveTenant is middleware for /t/{slug}/... routes. It resolves the slug to
// a workspace and confirms the authenticated user is a member. An unknown slug
// and a non-member both yield an identical opaque 404, so a non-member cannot
// learn whether a workspace exists. On success the workspace is placed in the
// request context.
func (s *Server) resolveTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := userFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "a valid session is required")
			return
		}
		ws, err := s.tenant.Queries.ResolveWorkspace.Handle(r.Context(),
			tenantquery.ResolveWorkspace{Slug: chi.URLParam(r, "slug"), UserID: user.ID})
		if err != nil {
			s.fail(w, "resolve tenant", err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tenantCtxKey, ws)))
	})
}

// tenantFromContext returns the workspace stored by resolveTenant.
func tenantFromContext(ctx context.Context) tenantquery.ResolvedWorkspace {
	ws, _ := ctx.Value(tenantCtxKey).(tenantquery.ResolvedWorkspace)
	return ws
}

// setSessionCookie writes the session cookie to the response.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
	})
}

// clearSessionCookie expires the session cookie on the client.
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
