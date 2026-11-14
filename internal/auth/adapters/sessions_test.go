package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func TestSessionsIssueAndResolve(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		sessions := b.sessions(t)
		ctx := context.Background()

		userID := b.newUserID(t)
		s, err := domain.NewSession(userID, time.Hour)
		require.NoError(t, err)

		tokenHash := dbtest.RandString()
		require.NoError(t, sessions.Issue(ctx, s, tokenHash))

		resolved, err := sessions.ResolveByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		require.NotEmpty(t, resolved.ID(), "the adapter assigns an id")
		require.Equal(t, userID, resolved.UserID())
		require.WithinDuration(t, s.ExpiresAt(), resolved.ExpiresAt(), time.Second)
		require.True(t, resolved.IsLive(time.Now()))
	})
}

func TestSessionsResolveRejectsExpired(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		sessions := b.sessions(t)
		ctx := context.Background()

		expired, err := domain.NewSession(b.newUserID(t), -time.Hour)
		require.NoError(t, err)

		tokenHash := dbtest.RandString()
		require.NoError(t, sessions.Issue(ctx, expired, tokenHash))

		_, err = sessions.ResolveByTokenHash(ctx, tokenHash)
		require.ErrorIs(t, err, domain.ErrSessionInvalid, "an expired session does not resolve")
	})
}

func TestSessionsResolveUnknownToken(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		_, err := b.sessions(t).ResolveByTokenHash(context.Background(), dbtest.RandString())
		require.ErrorIs(t, err, domain.ErrSessionInvalid)
	})
}

func TestSessionsRevoke(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		sessions := b.sessions(t)
		ctx := context.Background()

		s, err := domain.NewSession(b.newUserID(t), time.Hour)
		require.NoError(t, err)

		tokenHash := dbtest.RandString()
		require.NoError(t, sessions.Issue(ctx, s, tokenHash))
		require.NoError(t, sessions.RevokeByTokenHash(ctx, tokenHash))

		_, err = sessions.ResolveByTokenHash(ctx, tokenHash)
		require.ErrorIs(t, err, domain.ErrSessionInvalid, "a revoked session does not resolve")

		// Revoking again, or revoking an unknown token, is a harmless no-op.
		require.NoError(t, sessions.RevokeByTokenHash(ctx, tokenHash))
		require.NoError(t, sessions.RevokeByTokenHash(ctx, dbtest.RandString()))
	})
}
