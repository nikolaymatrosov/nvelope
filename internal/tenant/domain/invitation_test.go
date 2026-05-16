package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func mustEmail(t *testing.T, raw string) domain.Email {
	t.Helper()
	e, err := domain.NewEmail(raw)
	require.NoError(t, err)
	return e
}

func TestNewInvitationIsPending(t *testing.T) {
	t.Parallel()

	inv, err := domain.NewInvitation("tenant-1", mustEmail(t, "grace@example.com"), "user-1", time.Hour)
	require.NoError(t, err)
	require.Equal(t, domain.InvitationPending, inv.Status())
	require.Equal(t, "grace@example.com", inv.Email().String())
	require.WithinDuration(t, time.Now().Add(time.Hour), inv.ExpiresAt(), time.Minute)

	_, err = domain.NewInvitation("", mustEmail(t, "grace@example.com"), "user-1", time.Hour)
	require.Error(t, err, "an invitation needs a tenant")
}

func TestInvitationAccept(t *testing.T) {
	t.Parallel()
	now := time.Now()

	pending := domain.HydrateInvitation("i1", "t1", "g@example.com",
		"pending", "u1", now, now.Add(time.Hour))
	require.True(t, pending.IsAcceptable(now))
	require.NoError(t, pending.Accept(now))
	require.Equal(t, domain.InvitationAccepted, pending.Status())

	// Accepting again is no longer acceptable, and the error is the opaque one.
	require.ErrorIs(t, pending.Accept(now), domain.ErrInvitationNotFound)

	expired := domain.HydrateInvitation("i2", "t1", "g@example.com",
		"pending", "u1", now, now.Add(-time.Hour))
	require.False(t, expired.IsAcceptable(now))
	require.ErrorIs(t, expired.Accept(now), domain.ErrInvitationNotFound,
		"an expired invitation fails with the opaque error")
}

func TestInvitationRevoke(t *testing.T) {
	t.Parallel()
	now := time.Now()

	pending := domain.HydrateInvitation("i1", "t1", "g@example.com",
		"pending", "u1", now, now.Add(time.Hour))
	require.NoError(t, pending.Revoke())
	require.Equal(t, domain.InvitationRevoked, pending.Status())

	// Revoking a non-pending invitation fails with the opaque error.
	require.ErrorIs(t, pending.Revoke(), domain.ErrInvitationNotFound)

	accepted := domain.HydrateInvitation("i2", "t1", "g@example.com",
		"accepted", "u1", now, now.Add(time.Hour))
	require.ErrorIs(t, accepted.Revoke(), domain.ErrInvitationNotFound)

	// An expired-but-pending invitation can still be revoked.
	expiredPending := domain.HydrateInvitation("i3", "t1", "g@example.com",
		"pending", "u1", now, now.Add(-time.Hour))
	require.NoError(t, expiredPending.Revoke())
}
