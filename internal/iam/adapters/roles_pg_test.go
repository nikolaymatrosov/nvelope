package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func newRole(t *testing.T, tenantID, name string, perms ...domain.Permission) *domain.Role {
	t.Helper()
	r, err := domain.NewRole(tenantID, name, perms)
	require.NoError(t, err)
	return r
}

func TestRolesAddGetAll(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewRoles(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Editor", domain.PermListsManage))
	require.NoError(t, err)

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, "Editor", got.Name())
	require.Equal(t, []domain.Permission{domain.PermListsManage}, got.Permissions())

	_, err = repo.Add(ctx, tenantID, newRole(t, tenantID, "Editor"))
	require.ErrorIs(t, err, domain.ErrRoleNameTaken)

	all, err := repo.All(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, all, 1)
}

func TestRolesUpdate(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewRoles(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "R"))
	require.NoError(t, err)
	require.NoError(t, repo.Update(ctx, tenantID, id, func(r *domain.Role) (*domain.Role, error) {
		return r, r.SetPermissions([]domain.Permission{domain.PermRolesGet})
	}))
	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, []domain.Permission{domain.PermRolesGet}, got.Permissions())
}

func TestRolesDeleteRejectsAssigned(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewRoles(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	userID := seedTenantUser(t, pool, tenantID)

	id, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Assigned", domain.PermListsGet))
	require.NoError(t, err)
	require.NoError(t, repo.AssignTenantRole(ctx, tenantID, userID, id))

	require.ErrorIs(t, repo.Delete(ctx, tenantID, id), domain.ErrRoleInUse)

	// An unassigned role deletes cleanly.
	free, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Free"))
	require.NoError(t, err)
	require.NoError(t, repo.Delete(ctx, tenantID, free))
}

func TestRolesEffectiveFor(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewRoles(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	userID := seedTenantUser(t, pool, tenantID)

	tenantRole, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Tenant", domain.PermListsGet))
	require.NoError(t, err)
	listRole, err := repo.Add(ctx, tenantID,
		newRole(t, tenantID, "ListEditor", domain.PermSubscribersManage))
	require.NoError(t, err)

	require.NoError(t, repo.AssignTenantRole(ctx, tenantID, userID, tenantRole))

	listID := seedList(t, pool, tenantID)
	require.NoError(t, repo.AssignListRole(ctx, tenantID, userID, listID, listRole))

	tenantPerms, listPerms, err := repo.EffectiveFor(ctx, tenantID, userID)
	require.NoError(t, err)
	require.Equal(t, []domain.Permission{domain.PermListsGet}, tenantPerms)
	require.Equal(t, []domain.Permission{domain.PermSubscribersManage}, listPerms[listID])
}
