package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func TestTenantsCreateWorkspace(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTenants(pool)
	ctx := context.Background()

	ownerID := insertUser(t, pool)
	created := createWorkspace(t, repo, ownerID)
	require.NotEmpty(t, created.ID())
	require.Equal(t, domain.StatusActive, created.Status())

	// The owner membership was created in the same transaction.
	role, err := repo.GetMembershipRole(ctx, ownerID, created.ID())
	require.NoError(t, err)
	require.Equal(t, domain.RoleOwner, role)

	// The initial settings row was created under the RLS binding.
	settings, err := adapters.NewSettings(pool).Get(ctx, created.ID())
	require.NoError(t, err)
	require.Equal(t, "Workspace", settings.DisplayName())
}

func TestTenantsCreateRejectsDuplicateSlug(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTenants(pool)
	ctx := context.Background()

	ownerID := insertUser(t, pool)
	slug := "dup-" + dbtest.RandString()

	first, err := domain.NewTenant("First", slug)
	require.NoError(t, err)
	_, err = repo.CreateWorkspace(ctx, first, ownerID)
	require.NoError(t, err)

	second, err := domain.NewTenant("Second", slug)
	require.NoError(t, err)
	_, err = repo.CreateWorkspace(ctx, second, ownerID)
	require.ErrorIs(t, err, domain.ErrSlugTaken)
}

func TestTenantsGetBySlugAndID(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTenants(pool)
	ctx := context.Background()

	ownerID := insertUser(t, pool)
	created := createWorkspace(t, repo, ownerID)

	bySlug, err := repo.GetBySlug(ctx, created.Slug().String())
	require.NoError(t, err)
	require.Equal(t, created.ID(), bySlug.ID())

	byID, err := repo.GetByID(ctx, created.ID())
	require.NoError(t, err)
	require.Equal(t, created.Slug().String(), byID.Slug().String())

	_, err = repo.GetBySlug(ctx, "no-such-"+dbtest.RandString())
	require.ErrorIs(t, err, domain.ErrTenantNotFound)

	_, err = repo.GetByID(ctx, "not-a-uuid")
	require.ErrorIs(t, err, domain.ErrTenantNotFound, "a malformed id is an opaque not-found")
}

func TestTenantsMembership(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTenants(pool)
	ctx := context.Background()

	ownerID := insertUser(t, pool)
	created := createWorkspace(t, repo, ownerID)
	stranger := insertUser(t, pool)

	_, err := repo.GetMembershipRole(ctx, stranger, created.ID())
	require.ErrorIs(t, err, domain.ErrNotMember)

	m, err := domain.NewMembership(stranger, created.ID(), domain.RoleAdmin)
	require.NoError(t, err)
	require.NoError(t, repo.AddMembership(ctx, m))

	role, err := repo.GetMembershipRole(ctx, stranger, created.ID())
	require.NoError(t, err)
	require.Equal(t, domain.RoleAdmin, role)

	// Adding the same membership again is a harmless no-op.
	require.NoError(t, repo.AddMembership(ctx, m))
}
