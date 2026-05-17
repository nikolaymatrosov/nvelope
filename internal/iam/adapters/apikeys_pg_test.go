package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestAPIKeysAddByTokenHashRevoke(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewAPIKeys(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	userID := seedTenantUser(t, pool, tenantID)

	key, err := domain.NewAPIKey(tenantID, "CI", "token-hash",
		[]domain.Permission{domain.PermSubscribersGet}, userID)
	require.NoError(t, err)
	id, err := repo.Add(ctx, tenantID, key)
	require.NoError(t, err)

	got, err := repo.ByTokenHash(ctx, tenantID, "token-hash")
	require.NoError(t, err)
	require.Equal(t, id, got.ID())
	require.Equal(t, []domain.Permission{domain.PermSubscribersGet}, got.Permissions())
	require.False(t, got.IsRevoked())

	require.NoError(t, repo.TouchLastUsed(ctx, tenantID, id))

	require.NoError(t, repo.Revoke(ctx, tenantID, id))
	got, err = repo.ByTokenHash(ctx, tenantID, "token-hash")
	require.NoError(t, err)
	require.True(t, got.IsRevoked())

	// Revoking again, or an unknown key, reports not found.
	require.ErrorIs(t, repo.Revoke(ctx, tenantID, id), domain.ErrAPIKeyNotFound)
}

func TestAPIKeysByTokenHashMissing(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewAPIKeys(pool)
	tenantID := seedTenant(t, pool)

	_, err := repo.ByTokenHash(context.Background(), tenantID, "nope")
	require.ErrorIs(t, err, domain.ErrAPIKeyNotFound)
}

func TestAPIKeysAll(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewAPIKeys(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	userID := seedTenantUser(t, pool, tenantID)

	for _, name := range []string{"one", "two"} {
		key, err := domain.NewAPIKey(tenantID, name, "hash-"+name, nil, userID)
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, key)
		require.NoError(t, err)
	}
	keys, err := repo.All(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, keys, 2)
}
