package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func newRole(t *testing.T, tenantID, name string, perms ...domain.Permission) *domain.Role {
	t.Helper()
	r, err := domain.NewRole(tenantID, name, perms)
	require.NoError(t, err)
	return r
}

func roleNames(roles []*domain.Role) []string {
	out := make([]string, len(roles))
	for i, r := range roles {
		out[i] = r.Name()
	}
	return out
}

func TestRolesAddGetAll(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.roles(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Viewer", domain.PermListsGet, domain.PermRolesGet))
		require.NoError(t, err)
		require.NotEmpty(t, id, "the adapter assigns an id")

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, "Viewer", got.Name())
		require.Equal(t, []domain.Permission{domain.PermListsGet, domain.PermRolesGet}, got.Permissions())

		_, err = repo.Add(ctx, tenantID, newRole(t, tenantID, "Viewer"))
		require.ErrorIs(t, err, domain.ErrRoleNameTaken)

		_, err = repo.Add(ctx, tenantID, newRole(t, tenantID, "Editor"))
		require.NoError(t, err)
		empty, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Empty"))
		require.NoError(t, err)
		gotEmpty, err := repo.Get(ctx, tenantID, empty)
		require.NoError(t, err)
		require.Empty(t, gotEmpty.Permissions())

		all, err := repo.All(ctx, tenantID)
		require.NoError(t, err)
		require.Equal(t, []string{"Editor", "Empty", "Viewer"}, roleNames(all), "every role, ordered by name")

		otherTenant := b.newTenant(t)
		_, err = repo.Add(ctx, otherTenant, newRole(t, otherTenant, "Viewer"))
		require.NoError(t, err, "role names are unique per tenant")
		_, err = repo.Get(ctx, otherTenant, id)
		require.ErrorIs(t, err, domain.ErrRoleNotFound, "another tenant's role is not visible")
		others, err := repo.All(ctx, otherTenant)
		require.NoError(t, err)
		require.Equal(t, []string{"Viewer"}, roleNames(others))

		_, err = repo.Get(ctx, tenantID, uuid.NewString())
		require.ErrorIs(t, err, domain.ErrRoleNotFound)
		_, err = repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrRoleNotFound, "a malformed id is an opaque not-found")
	})
}

func TestRolesUpdate(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.roles(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "R"))
		require.NoError(t, err)
		require.NoError(t, repo.Update(ctx, tenantID, id, func(r *domain.Role) (*domain.Role, error) {
			if err := r.Rename("Renamed"); err != nil {
				return nil, err
			}
			return r, r.SetPermissions([]domain.Permission{domain.PermRolesGet})
		}))
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Renamed", got.Name())
		require.Equal(t, []domain.Permission{domain.PermRolesGet}, got.Permissions())

		// Keeping its own name is not a collision.
		require.NoError(t, repo.Update(ctx, tenantID, id, func(r *domain.Role) (*domain.Role, error) {
			return r, r.SetPermissions(nil)
		}))

		_, err = repo.Add(ctx, tenantID, newRole(t, tenantID, "Taken"))
		require.NoError(t, err)
		err = repo.Update(ctx, tenantID, id, func(r *domain.Role) (*domain.Role, error) {
			return r, r.Rename("Taken")
		})
		require.ErrorIs(t, err, domain.ErrRoleNameTaken)
		got, err = repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Renamed", got.Name(), "a rejected rename changes nothing")

		err = repo.Update(ctx, tenantID, uuid.NewString(), func(r *domain.Role) (*domain.Role, error) {
			return r, nil
		})
		require.ErrorIs(t, err, domain.ErrRoleNotFound)
	})
}

func TestRolesDeleteRejectsAssigned(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.roles(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		tenantRole, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Assigned", domain.PermListsGet))
		require.NoError(t, err)
		require.NoError(t, repo.AssignTenantRole(ctx, tenantID, userID, tenantRole))
		require.ErrorIs(t, repo.Delete(ctx, tenantID, tenantRole), domain.ErrRoleInUse)

		listRole, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "ListAssigned"))
		require.NoError(t, err)
		require.NoError(t, repo.AssignListRole(ctx, tenantID, userID, b.newList(t, tenantID), listRole))
		require.ErrorIs(t, repo.Delete(ctx, tenantID, listRole), domain.ErrRoleInUse,
			"a per-list assignment also keeps the role")

		// An unassigned role deletes cleanly.
		free, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Free"))
		require.NoError(t, err)
		require.NoError(t, repo.Delete(ctx, tenantID, free))
		_, err = repo.Get(ctx, tenantID, free)
		require.ErrorIs(t, err, domain.ErrRoleNotFound)

		require.ErrorIs(t, repo.Delete(ctx, tenantID, free), domain.ErrRoleNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, "not-a-uuid"), domain.ErrRoleNotFound)
		require.ErrorIs(t, repo.Delete(ctx, b.newTenant(t), tenantRole), domain.ErrRoleNotFound,
			"another tenant cannot delete the role")
	})
}

func TestRolesEffectiveFor(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.roles(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		tenantPerms, listPerms, err := repo.EffectiveFor(ctx, tenantID, userID)
		require.NoError(t, err)
		require.Empty(t, tenantPerms, "a user with no role has no permissions")
		require.Empty(t, listPerms)

		tenantRole, err := repo.Add(ctx, tenantID, newRole(t, tenantID, "Tenant", domain.PermListsGet))
		require.NoError(t, err)
		listRole, err := repo.Add(ctx, tenantID,
			newRole(t, tenantID, "ListEditor", domain.PermSubscribersManage))
		require.NoError(t, err)

		require.NoError(t, repo.AssignTenantRole(ctx, tenantID, userID, tenantRole))
		listID := b.newList(t, tenantID)
		require.NoError(t, repo.AssignListRole(ctx, tenantID, userID, listID, listRole))
		otherList := b.newList(t, tenantID)
		require.NoError(t, repo.AssignListRole(ctx, tenantID, userID, otherList, tenantRole))

		tenantPerms, listPerms, err = repo.EffectiveFor(ctx, tenantID, userID)
		require.NoError(t, err)
		require.Equal(t, []domain.Permission{domain.PermListsGet}, tenantPerms)
		require.Equal(t, map[string][]domain.Permission{
			listID:    {domain.PermSubscribersManage},
			otherList: {domain.PermListsGet},
		}, listPerms)

		// Assigning again replaces the role.
		require.NoError(t, repo.AssignTenantRole(ctx, tenantID, userID, listRole))
		require.NoError(t, repo.AssignListRole(ctx, tenantID, userID, listID, tenantRole))
		require.NoError(t, repo.RemoveListRole(ctx, tenantID, userID, otherList))
		require.NoError(t, repo.RemoveListRole(ctx, tenantID, userID, otherList), "removing twice is a no-op")

		tenantPerms, listPerms, err = repo.EffectiveFor(ctx, tenantID, userID)
		require.NoError(t, err)
		require.Equal(t, []domain.Permission{domain.PermSubscribersManage}, tenantPerms)
		require.Equal(t, map[string][]domain.Permission{listID: {domain.PermListsGet}}, listPerms)

		tenantPerms, listPerms, err = repo.EffectiveFor(ctx, b.newTenant(t), userID)
		require.NoError(t, err)
		require.Empty(t, tenantPerms, "another tenant sees none of the user's roles")
		require.Empty(t, listPerms)
	})
}

// TestRolesAssignUnknownRoleFails pins the Postgres foreign key from an
// assignment to its role: assigning a role that does not exist fails and
// grants nothing.
func TestRolesAssignUnknownRoleFails(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.roles(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		userID := newTenantUser(t, b, tenantID)

		require.Error(t, repo.AssignTenantRole(ctx, tenantID, userID, uuid.NewString()))
		require.Error(t, repo.AssignListRole(ctx, tenantID, userID, b.newList(t, tenantID), uuid.NewString()))

		tenantPerms, listPerms, err := repo.EffectiveFor(ctx, tenantID, userID)
		require.NoError(t, err)
		require.Empty(t, tenantPerms)
		require.Empty(t, listPerms)
	})
}
