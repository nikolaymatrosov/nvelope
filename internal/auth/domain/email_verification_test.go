package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

func TestNewEmailVerificationValidates(t *testing.T) {
	t.Parallel()

	_, err := domain.NewEmailVerification("", time.Hour)
	require.Error(t, err, "a user is required")

	_, err = domain.NewEmailVerification("user-1", 0)
	require.Error(t, err, "a positive lifetime is required")

	v, err := domain.NewEmailVerification("user-1", time.Hour)
	require.NoError(t, err)
	require.Equal(t, "user-1", v.UserID())
	require.False(t, v.IsConsumed(), "a fresh challenge is unconsumed")
	require.False(t, v.IsExpired(time.Now()), "a fresh challenge is not expired")
	require.True(t, v.IsExpired(time.Now().Add(2*time.Hour)), "it expires after its window")
}

func TestEmailVerificationHydration(t *testing.T) {
	t.Parallel()

	consumedAt := time.Now()
	v := domain.HydrateEmailVerification("v1", "user-1",
		time.Now().Add(time.Hour), time.Now().Add(-time.Hour), &consumedAt)
	require.Equal(t, "v1", v.ID())
	require.Equal(t, "user-1", v.UserID())
	require.True(t, v.IsConsumed(), "a row with a consumed_at is consumed")

	live := domain.HydrateEmailVerification("v2", "user-2",
		time.Now().Add(time.Hour), time.Now(), nil)
	require.False(t, live.IsConsumed())
}
