package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// LogOut is the request to end the session identified by a raw token.
type LogOut struct {
	RawToken string
}

// LogOutHandler handles the LogOut command.
type LogOutHandler struct {
	sessions domain.SessionRepository
}

// NewLogOutHandler builds a LogOutHandler, failing fast on a nil dependency.
func NewLogOutHandler(sessions domain.SessionRepository) LogOutHandler {
	if sessions == nil {
		panic("nil sessions repository")
	}
	return LogOutHandler{sessions: sessions}
}

// Handle revokes the session. Logging out with no token, or with an unknown or
// already-revoked token, is a harmless no-op.
func (h LogOutHandler) Handle(ctx context.Context, cmd LogOut) error {
	if cmd.RawToken == "" {
		return nil
	}
	return h.sessions.RevokeByTokenHash(ctx, token.Hash(cmd.RawToken))
}
