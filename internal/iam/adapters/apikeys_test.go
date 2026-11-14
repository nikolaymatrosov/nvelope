package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestAPIKeysAddByTokenHashRevoke(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.apiKeys(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		hash := "hash-" + dbtest.RandString()
		key, err := domain.NewAPIKey(tenantID, "CI", hash,
			[]domain.Permission{domain.PermSubscribersGet}, userID)
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, key)
		require.NoError(t, err)
		require.NotEmpty(t, id, "the adapter assigns an id")

		got, err := repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, "CI", got.Name())
		require.Equal(t, hash, got.TokenHash())
		require.Equal(t, []domain.Permission{domain.PermSubscribersGet}, got.Permissions())
		require.Equal(t, userID, got.CreatedBy())
		require.WithinDuration(t, time.Now(), got.CreatedAt(), time.Minute, "the database stamps the insert")
		require.Nil(t, got.LastUsedAt())
		require.False(t, got.IsRevoked())

		require.NoError(t, repo.TouchLastUsed(ctx, tenantID, id))
		got, err = repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.NotNil(t, got.LastUsedAt())
		require.WithinDuration(t, time.Now(), *got.LastUsedAt(), time.Minute)

		require.NoError(t, repo.Revoke(ctx, tenantID, id))
		got, err = repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.True(t, got.IsRevoked())
		require.WithinDuration(t, time.Now(), *got.RevokedAt(), time.Minute)

		// Revoking again, or an unknown key, reports not found.
		require.ErrorIs(t, repo.Revoke(ctx, tenantID, id), domain.ErrAPIKeyNotFound)
		require.ErrorIs(t, repo.Revoke(ctx, tenantID, uuid.NewString()), domain.ErrAPIKeyNotFound)
		require.ErrorIs(t, repo.Revoke(ctx, tenantID, "not-a-uuid"), domain.ErrAPIKeyNotFound,
			"a malformed id is an opaque not-found")
	})
}

func TestAPIKeysByTokenHashMissing(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		_, err := b.apiKeys(t).ByTokenHash(context.Background(), b.newTenant(t), "nope")
		require.ErrorIs(t, err, domain.ErrAPIKeyNotFound)
	})
}

func TestAPIKeysAll(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.apiKeys(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		for _, name := range []string{"one", "two"} {
			key, err := domain.NewAPIKey(tenantID, name, "hash-"+dbtest.RandString(), nil, userID)
			require.NoError(t, err)
			_, err = repo.Add(ctx, tenantID, key)
			require.NoError(t, err)
		}
		keys, err := repo.All(ctx, tenantID)
		require.NoError(t, err)
		require.Len(t, keys, 2)
		require.Equal(t, "two", keys[0].Name(), "newest first")
		require.Equal(t, "one", keys[1].Name())
		require.Empty(t, keys[0].Permissions())
	})
}

func TestAPIKeysAreTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.apiKeys(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		otherTenant := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		hash := "hash-" + dbtest.RandString()
		key, err := domain.NewAPIKey(tenantID, "CI", hash, nil, userID)
		require.NoError(t, err)
		id, err := repo.Add(ctx, tenantID, key)
		require.NoError(t, err)

		_, err = repo.ByTokenHash(ctx, otherTenant, hash)
		require.ErrorIs(t, err, domain.ErrAPIKeyNotFound)
		require.ErrorIs(t, repo.Revoke(ctx, otherTenant, id), domain.ErrAPIKeyNotFound)
		require.NoError(t, repo.TouchLastUsed(ctx, otherTenant, id))
		others, err := repo.All(ctx, otherTenant)
		require.NoError(t, err)
		require.Empty(t, others)

		got, err := repo.ByTokenHash(ctx, tenantID, hash)
		require.NoError(t, err)
		require.False(t, got.IsRevoked(), "another tenant cannot revoke the key")
		require.Nil(t, got.LastUsedAt(), "another tenant cannot touch the key")
	})
}
