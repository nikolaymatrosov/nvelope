package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestNewSessionStartState(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(time.Hour)

	plain, err := domain.NewSession("t1", "u1", "hash", false, future)
	require.NoError(t, err)
	require.Equal(t, domain.SessionActive, plain.State(), "no TOTP → active immediately")

	totp, err := domain.NewSession("t1", "u1", "hash", true, future)
	require.NoError(t, err)
	require.Equal(t, domain.SessionTOTPPending, totp.State(), "TOTP user → totp-pending")
}

func TestSessionCompleteTOTP(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(time.Hour)
	s, _ := domain.NewSession("t1", "u1", "hash", true, future)

	require.NoError(t, s.CompleteTOTP())
	require.Equal(t, domain.SessionActive, s.State())
	require.Error(t, s.CompleteTOTP(), "an active session is not awaiting a challenge")
}

func TestSessionIsActive(t *testing.T) {
	t.Parallel()
	now := time.Now()

	active, _ := domain.NewSession("t1", "u1", "hash", false, now.Add(time.Hour))
	require.True(t, active.IsActive(now))

	expired, _ := domain.NewSession("t1", "u1", "hash", false, now.Add(-time.Hour))
	require.False(t, expired.IsActive(now), "an expired session authenticates nothing")

	pending, _ := domain.NewSession("t1", "u1", "hash", true, now.Add(time.Hour))
	require.False(t, pending.IsActive(now), "a totp-pending session grants no access")
}

func TestSessionRevoke(t *testing.T) {
	t.Parallel()
	now := time.Now()
	s, _ := domain.NewSession("t1", "u1", "hash", false, now.Add(time.Hour))
	s.Revoke(now)
	require.Equal(t, domain.SessionRevoked, s.State())
	require.False(t, s.IsActive(now))
	require.NotNil(t, s.RevokedAt())
}
