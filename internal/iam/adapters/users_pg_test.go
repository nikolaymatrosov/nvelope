package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestUsersAddGetByPlatformUser(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewUsers(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	platformID := seedPlatformUser(t, pool)

	u, err := domain.NewTenantUser(tenantID, platformID, "user@example.com", "Pat")
	require.NoError(t, err)
	id, err := repo.Add(ctx, tenantID, u)
	require.NoError(t, err)

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, "user@example.com", got.Email())
	require.False(t, got.TOTPEnabled())

	byPlatform, err := repo.ByPlatformUser(ctx, tenantID, platformID)
	require.NoError(t, err)
	require.Equal(t, id, byPlatform.ID())

	_, err = repo.ByPlatformUser(ctx, tenantID, "00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, domain.ErrUserNotFound)
}

func TestUsersUpdateTOTP(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewUsers(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	platformID := seedPlatformUser(t, pool)

	u, err := domain.NewTenantUser(tenantID, platformID, "totp@example.com", "Pat")
	require.NoError(t, err)
	id, err := repo.Add(ctx, tenantID, u)
	require.NoError(t, err)

	require.NoError(t, repo.Update(ctx, tenantID, id, func(u *domain.TenantUser) (*domain.TenantUser, error) {
		return u, u.EnableTOTP([]byte("encrypted-secret"))
	}))
	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.True(t, got.TOTPEnabled())
	require.Equal(t, []byte("encrypted-secret"), got.TOTPSecret())
}
