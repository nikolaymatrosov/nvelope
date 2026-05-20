package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func newField(t *testing.T, tenantID, slug, displayName string, pos int) *domain.Field {
	t.Helper()
	f, err := domain.NewField(tenantID, slug, displayName, domain.FieldTypeText, "", pos)
	require.NoError(t, err)
	return f
}

func TestFields_AddAndAll(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewFields(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id1, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 1))
	require.NoError(t, err)
	require.NotEmpty(t, id1)

	id2, err := repo.Add(ctx, tenantID, newField(t, tenantID, "plan_tier", "Plan tier", 2))
	require.NoError(t, err)

	all, err := repo.All(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.Equal(t, "country", all[0].Slug())
	require.Equal(t, "plan_tier", all[1].Slug())
	require.Equal(t, id1, all[0].ID())
	require.Equal(t, id2, all[1].ID())
}

func TestFields_AddDuplicateSlug(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewFields(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	_, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
	require.NoError(t, err)
	_, err = repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country dup", 1))
	require.ErrorIs(t, err, domain.ErrFieldSlugTaken)
}

func TestFields_AddRejectsBuiltinSlug(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewFields(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	_, err := repo.Add(ctx, tenantID, newField(t, tenantID, "first_name", "First name", 0))
	require.ErrorIs(t, err, domain.ErrFieldBuiltinSlug)
}

func TestFields_UpdateAndGet(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewFields(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
	require.NoError(t, err)

	err = repo.Update(ctx, tenantID, id, func(f *domain.Field) (*domain.Field, error) {
		require.NoError(t, f.Rename("Region"))
		require.NoError(t, f.Retype(domain.FieldTypeText))
		f.SetDefaultValue("US")
		return f, nil
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, "Region", got.DisplayName())
	require.Equal(t, "US", got.DefaultValue())
}

func TestFields_Delete(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewFields(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
	require.NoError(t, err)
	require.NoError(t, repo.Delete(ctx, tenantID, id))

	_, err = repo.Get(ctx, tenantID, id)
	require.ErrorIs(t, err, domain.ErrFieldNotFound)
}

func TestFields_Reorder(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewFields(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id1, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
	require.NoError(t, err)
	id2, err := repo.Add(ctx, tenantID, newField(t, tenantID, "plan_tier", "Plan tier", 1))
	require.NoError(t, err)

	require.NoError(t, repo.Reorder(ctx, tenantID, map[string]int{id1: 5, id2: 0}))

	all, err := repo.All(ctx, tenantID)
	require.NoError(t, err)
	require.Equal(t, "plan_tier", all[0].Slug(), "lower position is listed first")
	require.Equal(t, "country", all[1].Slug())
}

func TestFields_TenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewFields(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)

	idA, err := repo.Add(ctx, tenantA, newField(t, tenantA, "country", "Country", 0))
	require.NoError(t, err)

	// Tenant B must NOT see tenant A's rows via the standard query path.
	listB, err := repo.All(ctx, tenantB)
	require.NoError(t, err)
	require.Empty(t, listB)

	// Looking up tenant A's id under tenant B's RLS scope returns not-found.
	_, err = repo.Get(ctx, tenantB, idA)
	require.ErrorIs(t, err, domain.ErrFieldNotFound)
}
