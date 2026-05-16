package command_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

func TestLogOutRevokesSession(t *testing.T) {
	t.Parallel()
	sessions := newFakeSessions()
	raw := "raw-token"
	s, err := domain.NewSession("user-1", 0)
	require.NoError(t, err)
	require.NoError(t, sessions.Issue(context.Background(), s, token.Hash(raw)))

	h := command.NewLogOutHandler(sessions)
	require.NoError(t, h.Handle(context.Background(), command.LogOut{RawToken: raw}))

	_, err = sessions.ResolveByTokenHash(context.Background(), token.Hash(raw))
	require.ErrorIs(t, err, domain.ErrSessionInvalid, "the session is revoked")
}

func TestLogOutWithoutTokenIsNoOp(t *testing.T) {
	t.Parallel()
	h := command.NewLogOutHandler(newFakeSessions())
	require.NoError(t, h.Handle(context.Background(), command.LogOut{RawToken: ""}))
}
