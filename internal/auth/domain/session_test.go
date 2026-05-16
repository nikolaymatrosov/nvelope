package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

func TestNewSessionSetsExpiry(t *testing.T) {
	t.Parallel()

	s, err := domain.NewSession("user-1", time.Hour)
	require.NoError(t, err)
	require.Equal(t, "user-1", s.UserID())
	require.WithinDuration(t, time.Now().Add(time.Hour), s.ExpiresAt(), time.Minute)

	_, err = domain.NewSession("", time.Hour)
	require.Error(t, err, "a session needs an owning user")
}

func TestSessionIsLive(t *testing.T) {
	t.Parallel()
	now := time.Now()

	live := domain.HydrateSession("s1", "u1", now.Add(time.Hour), nil)
	require.True(t, live.IsLive(now))

	expired := domain.HydrateSession("s2", "u1", now.Add(-time.Hour), nil)
	require.False(t, expired.IsLive(now), "an expired session is not live")

	revokedAt := now.Add(-time.Minute)
	revoked := domain.HydrateSession("s3", "u1", now.Add(time.Hour), &revokedAt)
	require.False(t, revoked.IsLive(now), "a revoked session is not live")
}

func TestSessionRevoke(t *testing.T) {
	t.Parallel()
	now := time.Now()

	s := domain.HydrateSession("s1", "u1", now.Add(time.Hour), nil)
	require.True(t, s.IsLive(now))

	s.Revoke(now)
	require.False(t, s.IsLive(now), "a revoked session is no longer live")

	s.Revoke(now.Add(time.Minute)) // revoking again is a harmless no-op
	require.False(t, s.IsLive(now))
}
