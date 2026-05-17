package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestNewRoleValid(t *testing.T) {
	t.Parallel()
	r, err := domain.NewRole("t1", "  Editor  ",
		[]domain.Permission{domain.PermListsGet, domain.PermListsGet, domain.PermListsManage})
	require.NoError(t, err)
	require.Equal(t, "Editor", r.Name())
	require.Len(t, r.Permissions(), 2, "duplicate permissions are deduplicated")
}

func TestNewRoleRejectsInvalid(t *testing.T) {
	t.Parallel()
	_, err := domain.NewRole("t1", "", nil)
	require.Error(t, err, "an empty name is rejected")

	_, err = domain.NewRole("t1", "R", []domain.Permission{"lists:explode"})
	require.Error(t, err, "an unknown permission is rejected")
}

func TestRoleRenameAndSetPermissions(t *testing.T) {
	t.Parallel()
	r, err := domain.NewRole("t1", "R", nil)
	require.NoError(t, err)

	require.NoError(t, r.Rename("Renamed"))
	require.Equal(t, "Renamed", r.Name())
	require.Error(t, r.Rename("   "))

	require.NoError(t, r.SetPermissions([]domain.Permission{domain.PermRolesManage}))
	require.Equal(t, []domain.Permission{domain.PermRolesManage}, r.Permissions())
	require.Error(t, r.SetPermissions([]domain.Permission{"nope"}))
}
