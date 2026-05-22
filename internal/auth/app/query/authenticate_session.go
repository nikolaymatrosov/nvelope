package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// AuthenticateSession is the request to resolve a raw session token to a user.
type AuthenticateSession struct {
	RawToken string
}

// AuthenticatedUser is the flat read model for an authenticated caller.
type AuthenticatedUser struct {
	ID    string
	Email string
	Name  string
	// Locale is the user's chosen interface language, empty when the user has
	// never explicitly chosen one.
	Locale string
}

// AuthenticateSessionHandler handles the AuthenticateSession query.
type AuthenticateSessionHandler struct {
	sessions domain.SessionRepository
	users    domain.UserRepository
}

// NewAuthenticateSessionHandler builds the handler, failing fast on nil
// dependencies.
func NewAuthenticateSessionHandler(sessions domain.SessionRepository,
	users domain.UserRepository) AuthenticateSessionHandler {
	if sessions == nil {
		panic("nil sessions repository")
	}
	if users == nil {
		panic("nil users repository")
	}
	return AuthenticateSessionHandler{sessions: sessions, users: users}
}

// Handle resolves the token to its live session and owning user. It returns
// domain.ErrSessionInvalid when the token does not resolve to a usable session.
func (h AuthenticateSessionHandler) Handle(ctx context.Context, q AuthenticateSession) (AuthenticatedUser, error) {
	if q.RawToken == "" {
		return AuthenticatedUser{}, domain.ErrSessionInvalid
	}
	session, err := h.sessions.ResolveByTokenHash(ctx, token.Hash(q.RawToken))
	if err != nil {
		return AuthenticatedUser{}, err
	}
	user, err := h.users.GetByID(ctx, session.UserID())
	if err != nil {
		return AuthenticatedUser{}, err
	}
	return AuthenticatedUser{
		ID:     user.ID(),
		Email:  user.Email().String(),
		Name:   user.Name(),
		Locale: user.Locale().String(),
	}, nil
}
