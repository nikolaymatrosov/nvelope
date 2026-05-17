package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestNewAPIKeyValidatesInput(t *testing.T) {
	t.Parallel()

	t.Run("accepts a scoped permission subset", func(t *testing.T) {
		t.Parallel()
		k, err := domain.NewAPIKey("t1", "CI key", "hash", []domain.Permission{
			domain.PermSubscribersGet,
		}, "u1")
		require.NoError(t, err)
		require.Equal(t, "CI key", k.Name())
		require.False(t, k.IsRevoked())
	})

	t.Run("rejects a blank name", func(t *testing.T) {
		t.Parallel()
		_, err := domain.NewAPIKey("t1", "  ", "hash", nil, "u1")
		require.Error(t, err)
	})

	t.Run("rejects an unknown permission", func(t *testing.T) {
		t.Parallel()
		_, err := domain.NewAPIKey("t1", "key", "hash",
			[]domain.Permission{"lists:explode"}, "u1")
		require.Error(t, err)
	})

	t.Run("rejects a missing token or creator", func(t *testing.T) {
		t.Parallel()
		_, err := domain.NewAPIKey("t1", "key", "", nil, "u1")
		require.Error(t, err)
		_, err = domain.NewAPIKey("t1", "key", "hash", nil, "")
		require.Error(t, err)
	})
}

func TestAPIKeyRevoke(t *testing.T) {
	t.Parallel()
	k, err := domain.NewAPIKey("t1", "key", "hash", nil, "u1")
	require.NoError(t, err)
	require.False(t, k.IsRevoked())

	k.Revoke(time.Now())
	require.True(t, k.IsRevoked())

	first := *k.RevokedAt()
	k.Revoke(time.Now().Add(time.Hour))
	require.Equal(t, first, *k.RevokedAt(), "revoking again does not move the timestamp")
}
