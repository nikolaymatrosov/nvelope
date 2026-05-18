package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestNewPermissionRejectsUnknown(t *testing.T) {
	t.Parallel()
	_, err := domain.NewPermission("lists:get")
	require.NoError(t, err)
	_, err = domain.NewPermission("lists:destroy")
	require.Error(t, err)
}

func TestEffectivePermissionsUnion(t *testing.T) {
	t.Parallel()
	tenant := []domain.Permission{domain.PermListsGet, domain.PermSubscribersGet}
	list := []domain.Permission{domain.PermSubscribersGet, domain.PermSubscribersManage}

	got := domain.EffectivePermissions(tenant, list)
	require.ElementsMatch(t, []domain.Permission{
		domain.PermListsGet, domain.PermSubscribersGet, domain.PermSubscribersManage,
	}, got, "the effective set is the deduplicated union")
}

func TestIsListScoped(t *testing.T) {
	t.Parallel()
	require.True(t, domain.IsListScoped(domain.PermListsManage))
	require.False(t, domain.IsListScoped(domain.PermRolesManage),
		"administrative permissions are tenant-wide only")
}

func TestAllPermissionsCoversCatalogue(t *testing.T) {
	t.Parallel()
	require.Len(t, domain.AllPermissions(), 18)
}
