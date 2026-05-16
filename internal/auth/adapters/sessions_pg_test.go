package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func persistUser(t *testing.T, users *adapters.Users) string {
	t.Helper()
	u, _ := newUser(t, "Session Owner")
	created, err := users.Create(context.Background(), u, "hash")
	require.NoError(t, err)
	return created.ID()
}

func TestSessionsIssueAndResolve(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	users := adapters.NewUsers(pool)
	sessions := adapters.NewSessions(pool)
	ctx := context.Background()

	userID := persistUser(t, users)
	s, err := domain.NewSession(userID, time.Hour)
	require.NoError(t, err)

	tokenHash := dbtest.RandString()
	require.NoError(t, sessions.Issue(ctx, s, tokenHash))

	resolved, err := sessions.ResolveByTokenHash(ctx, tokenHash)
	require.NoError(t, err)
	require.Equal(t, userID, resolved.UserID())
	require.True(t, resolved.IsLive(time.Now()))
}

func TestSessionsResolveRejectsExpired(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	users := adapters.NewUsers(pool)
	sessions := adapters.NewSessions(pool)
	ctx := context.Background()

	userID := persistUser(t, users)
	expired, err := domain.NewSession(userID, -time.Hour)
	require.NoError(t, err)

	tokenHash := dbtest.RandString()
	require.NoError(t, sessions.Issue(ctx, expired, tokenHash))

	_, err = sessions.ResolveByTokenHash(ctx, tokenHash)
	require.ErrorIs(t, err, domain.ErrSessionInvalid, "an expired session does not resolve")
}

func TestSessionsRevoke(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	users := adapters.NewUsers(pool)
	sessions := adapters.NewSessions(pool)
	ctx := context.Background()

	userID := persistUser(t, users)
	s, err := domain.NewSession(userID, time.Hour)
	require.NoError(t, err)

	tokenHash := dbtest.RandString()
	require.NoError(t, sessions.Issue(ctx, s, tokenHash))
	require.NoError(t, sessions.RevokeByTokenHash(ctx, tokenHash))

	_, err = sessions.ResolveByTokenHash(ctx, tokenHash)
	require.ErrorIs(t, err, domain.ErrSessionInvalid, "a revoked session does not resolve")

	// Revoking an unknown token is a harmless no-op.
	require.NoError(t, sessions.RevokeByTokenHash(ctx, dbtest.RandString()))
}
