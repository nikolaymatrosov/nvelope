package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func TestSettingsGetReturnsInitialRow(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSettings(pool)
	ctx := context.Background()

	ownerID := insertUser(t, pool)
	ws := createWorkspace(t, adapters.NewTenants(pool), ownerID)

	settings, err := repo.Get(ctx, ws.ID())
	require.NoError(t, err)
	require.Equal(t, "Workspace", settings.DisplayName())
	require.Equal(t, "UTC", settings.Timezone(), "the timezone defaults at the database level")
}

func TestSettingsUpdate(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSettings(pool)
	ctx := context.Background()

	ownerID := insertUser(t, pool)
	ws := createWorkspace(t, adapters.NewTenants(pool), ownerID)

	err := repo.Update(ctx, ws.ID(), func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
		if err := s.Rename("Renamed"); err != nil {
			return nil, err
		}
		if err := s.SetTimezone("Europe/Madrid"); err != nil {
			return nil, err
		}
		return s, nil
	})
	require.NoError(t, err)

	settings, err := repo.Get(ctx, ws.ID())
	require.NoError(t, err)
	require.Equal(t, "Renamed", settings.DisplayName())
	require.Equal(t, "Europe/Madrid", settings.Timezone())
}

func TestSettingsUpdateRollsBackOnClosureError(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSettings(pool)
	ctx := context.Background()

	ownerID := insertUser(t, pool)
	ws := createWorkspace(t, adapters.NewTenants(pool), ownerID)

	sentinel := domain.ErrTenantNotFound
	err := repo.Update(ctx, ws.ID(), func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
		_ = s.Rename("Should Not Persist")
		return nil, sentinel
	})
	require.ErrorIs(t, err, sentinel)

	settings, err := repo.Get(ctx, ws.ID())
	require.NoError(t, err)
	require.Equal(t, "Workspace", settings.DisplayName(), "a closure error rolls the transaction back")
}
