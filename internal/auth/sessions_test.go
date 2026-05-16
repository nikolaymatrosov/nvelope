package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nvelope/nvelope/internal/dbtest"
)

func TestIssueResolveRevokeSession(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	user := insertTestUser(t, pool)

	rawToken, err := IssueSession(ctx, pool, user.ID, time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, rawToken)

	session, err := ResolveSession(ctx, pool, rawToken)
	require.NoError(t, err)
	require.Equal(t, user.ID, session.UserID)

	require.NoError(t, RevokeSession(ctx, pool, rawToken))

	_, err = ResolveSession(ctx, pool, rawToken)
	require.ErrorIs(t, err, ErrSessionInvalid, "a revoked session no longer resolves")
}

func TestResolveExpiredSession(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	user := insertTestUser(t, pool)

	rawToken, err := IssueSession(ctx, pool, user.ID, -time.Minute)
	require.NoError(t, err)

	_, err = ResolveSession(ctx, pool, rawToken)
	require.ErrorIs(t, err, ErrSessionInvalid, "an expired session does not resolve")
}

func TestResolveUnknownToken(t *testing.T) {
	pool := dbtest.AppPool(t)
	_, err := ResolveSession(context.Background(), pool, "not-a-real-token")
	require.ErrorIs(t, err, ErrSessionInvalid)
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()

	email := dbtest.RandString() + "@example.com"
	_, _, err := Signup(ctx, pool, time.Hour, email, "a-good-password", "Ada")
	require.NoError(t, err)

	_, _, err = Login(ctx, pool, time.Hour, email, "the-wrong-password")
	require.ErrorIs(t, err, ErrInvalidCredentials)

	_, _, err = Login(ctx, pool, time.Hour, "nobody-"+email, "a-good-password")
	require.ErrorIs(t, err, ErrInvalidCredentials, "an unknown email yields the same error")
}

func TestSignupRejectsDuplicateEmail(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()

	email := dbtest.RandString() + "@example.com"
	_, _, err := Signup(ctx, pool, time.Hour, email, "a-good-password", "Ada")
	require.NoError(t, err)

	_, _, err = Signup(ctx, pool, time.Hour, email, "another-password", "Imposter")
	require.ErrorIs(t, err, ErrEmailTaken)
}
