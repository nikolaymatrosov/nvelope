package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"

	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// requestIDHeader is the canonical HTTP header used end-to-end (BFF → Go) so
// one request can be correlated across both tiers.
const requestIDHeader = "X-Request-Id"

// requestIDCtxKey holds the resolved request id in the request context.
const requestIDCtxKey ctxKey = 200

// requestID is middleware that resolves X-Request-Id from the incoming request
// (the BFF generates one if absent and forwards it; an external caller may
// omit it, in which case a fresh id is minted). The id is echoed on the
// response and stored in context for per-request structured logs.
func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDCtxKey, id)))
	})
}

// newRequestID returns a 128-bit hex-encoded id. crypto/rand cannot fail on
// the supported platforms; an extremely improbable read error yields an
// empty id, which the logging layer renders as "" and the response header
// suppresses.
func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(buf[:])
}

// requestIDFromContext returns the request id stored by the requestID
// middleware. Empty when no middleware ran (e.g. on /healthz).
func requestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDCtxKey).(string)
	return id
}

// requestAttrs builds the standard slog.Attr set every Phase 7 handler emits
// on success or failure: tenant_id, actor_id, request_id. Callers append
// handler-specific fields on top. Empty fields are dropped so logs stay tidy
// for unauthenticated routes.
func requestAttrs(ctx context.Context) []slog.Attr {
	out := make([]slog.Attr, 0, 3)
	if ws := tenantFromContext(ctx); ws.ID != "" {
		out = append(out, slog.String("tenant_id", ws.ID))
	}
	if p, ok := principalFromContext(ctx); ok {
		out = append(out, slog.String("actor_id", p.ActorID()))
		out = append(out, slog.String("actor_kind", string(p.Kind())))
	} else if _, ok := userFromContext(ctx); ok {
		// Pre-authz routes (e.g. /tenant settings) only have an authenticated
		// user, not a Principal. The user id is logged as the actor.
		u, _ := userFromContext(ctx)
		out = append(out, slog.String("actor_id", u.ID))
		out = append(out, slog.String("actor_kind", "user"))
	}
	if id := requestIDFromContext(ctx); id != "" {
		out = append(out, slog.String("request_id", id))
	}
	return out
}

// logEvent emits a per-request slog record at info level with the standard
// fields plus handler-specific extras. The logger is allowed to be nil so
// directly-constructed test servers don't panic; the call becomes a no-op.
func (s *Server) logEvent(ctx context.Context, event string, extras ...slog.Attr) {
	if s.logger == nil {
		return
	}
	attrs := requestAttrs(ctx)
	attrs = append(attrs, extras...)
	s.logger.LogAttrs(ctx, slog.LevelInfo, event, attrs...)
}

// actorIDFromContext returns the resolved actor id, or empty. Convenience
// wrapper for handlers that need just the id to thread into a command.
func actorIDFromContext(ctx context.Context) string {
	if p, ok := principalFromContext(ctx); ok {
		return p.ActorID()
	}
	return ""
}

// actorKindFromContext returns the resolved actor's kind, defaulting to
// PrincipalSession when no principal is present (the most common case for
// non-API-key callers).
func actorKindFromContext(ctx context.Context) iamdomain.PrincipalKind {
	if p, ok := principalFromContext(ctx); ok {
		return p.Kind()
	}
	return iamdomain.PrincipalSession
}
