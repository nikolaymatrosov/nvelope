package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func TestNewTenantSettings(t *testing.T) {
	t.Parallel()

	s, err := domain.NewTenantSettings("tenant-1", "  Workspace  ")
	require.NoError(t, err)
	require.Equal(t, "tenant-1", s.TenantID())
	require.Equal(t, "Workspace", s.DisplayName(), "the display name is trimmed")

	_, err = domain.NewTenantSettings("tenant-1", "   ")
	require.Error(t, err, "an empty display name is rejected")

	_, err = domain.NewTenantSettings("", "Workspace")
	require.Error(t, err, "a tenant is required")
}

func TestTenantSettingsRename(t *testing.T) {
	t.Parallel()

	s := domain.HydrateTenantSettings("tenant-1", "Old", "UTC")
	require.NoError(t, s.Rename("  Renamed  "))
	require.Equal(t, "Renamed", s.DisplayName())

	require.Error(t, s.Rename("   "), "an empty display name is rejected")
	require.Equal(t, "Renamed", s.DisplayName(), "a rejected rename leaves the name unchanged")
}

func TestTenantSettingsSetTimezone(t *testing.T) {
	t.Parallel()

	s := domain.HydrateTenantSettings("tenant-1", "Workspace", "UTC")
	require.NoError(t, s.SetTimezone("Europe/Madrid"))
	require.Equal(t, "Europe/Madrid", s.Timezone())

	require.Error(t, s.SetTimezone("  "), "an empty timezone is rejected")
	require.Equal(t, "Europe/Madrid", s.Timezone())
}
