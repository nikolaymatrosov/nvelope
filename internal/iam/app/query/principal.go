// Package query holds the iam context's read-only handlers.
package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// AuthenticatePrincipal is the request to resolve a session token into the
// request's Principal.
type AuthenticatePrincipal struct {
	TenantID string
	Token    string
}

// AuthenticatePrincipalHandler handles the AuthenticatePrincipal query. It
// resolves a workspace session token into a Principal carrying the holder's
// effective permissions, loaded fresh so a role change takes effect on the
// next request.
type AuthenticatePrincipalHandler struct {
	sessions domain.SessionRepository
	roles    domain.RoleRepository
}

// NewAuthenticatePrincipalHandler builds the handler, failing fast on a nil
// dependency.
func NewAuthenticatePrincipalHandler(sessions domain.SessionRepository,
	roles domain.RoleRepository) AuthenticatePrincipalHandler {
	if sessions == nil || roles == nil {
		panic("nil dependency")
	}
	return AuthenticatePrincipalHandler{sessions: sessions, roles: roles}
}

// Handle resolves the session token. It returns ErrUnauthenticated for an
// unknown, expired, or revoked session, and ErrTOTPRequired for a session
// still awaiting its two-factor challenge.
func (h AuthenticatePrincipalHandler) Handle(ctx context.Context,
	q AuthenticatePrincipal) (domain.Principal, error) {

	session, err := h.sessions.ByTokenHash(ctx, q.TenantID, token.Hash(q.Token))
	if err != nil {
		return domain.Principal{}, domain.ErrUnauthenticated
	}
	if session.State() == domain.SessionTOTPPending {
		return domain.Principal{}, domain.ErrTOTPRequired
	}
	if !session.IsActive(time.Now()) {
		return domain.Principal{}, domain.ErrUnauthenticated
	}
	tenantPerms, listPerms, err := h.roles.EffectiveFor(ctx, q.TenantID, session.UserID())
	if err != nil {
		return domain.Principal{}, err
	}
	return domain.NewPrincipal(domain.PrincipalSession, q.TenantID, session.UserID(),
		tenantPerms, listPerms), nil
}
