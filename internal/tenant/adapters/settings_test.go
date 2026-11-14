package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func TestSettingsGetReturnsInitialRow(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		settings, err := b.settings(t).Get(context.Background(), ws.ID())
		require.NoError(t, err)
		require.Equal(t, ws.ID(), settings.TenantID())
		require.Equal(t, "Workspace", settings.DisplayName())
		require.Equal(t, "UTC", settings.Timezone(), "a new workspace starts in UTC")
	})
}

func TestSettingsUpdate(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.settings(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

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
	})
}

func TestSettingsUpdateRollsBackOnClosureError(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.settings(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		sentinel := domain.ErrTenantNotFound
		err := repo.Update(ctx, ws.ID(), func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
			_ = s.Rename("Should Not Persist")
			return nil, sentinel
		})
		require.ErrorIs(t, err, sentinel)

		settings, err := repo.Get(ctx, ws.ID())
		require.NoError(t, err)
		require.Equal(t, "Workspace", settings.DisplayName(), "a closure error rolls the transaction back")
	})
}

func TestSettingsAreTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.settings(t)
		tenants := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		first := createWorkspace(t, tenants, ownerID)
		second := createWorkspace(t, tenants, ownerID)

		err := repo.Update(ctx, first.ID(), func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
			if err := s.Rename("First Only"); err != nil {
				return nil, err
			}
			return s, nil
		})
		require.NoError(t, err)

		untouched, err := repo.Get(ctx, second.ID())
		require.NoError(t, err)
		require.Equal(t, "Workspace", untouched.DisplayName(), "another tenant's settings are untouched")

		unknown := uuid.NewString()
		_, err = repo.Get(ctx, unknown)
		require.ErrorIs(t, err, domain.ErrTenantNotFound)
		err = repo.Update(ctx, unknown, func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
			return s, nil
		})
		require.ErrorIs(t, err, domain.ErrTenantNotFound)
	})
}

func TestSettingsRawHTMLSanitizationRoundTrips(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.settings(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		settings, err := repo.Get(ctx, ws.ID())
		require.NoError(t, err)
		require.Equal(t, domain.RawHTMLStandard, settings.RawHTMLSanitization(), "new tenants default to standard")

		require.NoError(t, repo.Update(ctx, ws.ID(), func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
			return s, s.SetRawHTMLSanitization(domain.RawHTMLOff, true)
		}))
		settings, err = repo.Get(ctx, ws.ID())
		require.NoError(t, err)
		require.Equal(t, domain.RawHTMLOff, settings.RawHTMLSanitization())
		require.Equal(t, "Workspace", settings.DisplayName(), "other fields survive the update")
	})
}
