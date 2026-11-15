package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

func newField(t *testing.T, tenantID, slug, displayName string, pos int) *domain.Field {
	t.Helper()
	f, err := domain.NewField(tenantID, slug, displayName, domain.FieldTypeText, "", pos)
	require.NoError(t, err)
	return f
}

func TestFields_AddAndAll(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

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
		require.Equal(t, 2, all[1].Position())
		require.Equal(t, domain.FieldTypeText, all[1].Type())
		require.False(t, all[1].BuiltIn())
	})
}

func TestFields_AddDuplicateSlug(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country dup", 1))
		require.ErrorIs(t, err, domain.ErrFieldSlugTaken)
	})
}

func TestFields_AddRejectsBuiltinSlug(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Add(ctx, tenantID, newField(t, tenantID, "first_name", "First name", 0))
		require.ErrorIs(t, err, domain.ErrFieldBuiltinSlug)
	})
}

func TestFields_UpdateAndGet(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
		require.NoError(t, err)

		err = repo.Update(ctx, tenantID, id, func(f *domain.Field) (*domain.Field, error) {
			require.NoError(t, f.Rename("Region"))
			require.NoError(t, f.Retype(domain.FieldTypeNumber))
			f.SetDefaultValue("US")
			return f, nil
		})
		require.NoError(t, err)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "Region", got.DisplayName())
		require.Equal(t, domain.FieldTypeNumber, got.Type())
		require.Equal(t, "US", got.DefaultValue())
		require.Equal(t, "country", got.Slug(), "the slug is immutable")

		require.ErrorIs(t, repo.Update(ctx, tenantID, "not-a-uuid",
			func(f *domain.Field) (*domain.Field, error) { return f, nil }), domain.ErrFieldNotFound)
	})
}

func TestFields_Delete(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
		require.NoError(t, err)
		require.NoError(t, repo.Delete(ctx, tenantID, id))

		_, err = repo.Get(ctx, tenantID, id)
		require.ErrorIs(t, err, domain.ErrFieldNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, id), domain.ErrFieldNotFound)
	})
}

func TestFields_Reorder(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id1, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
		require.NoError(t, err)
		id2, err := repo.Add(ctx, tenantID, newField(t, tenantID, "plan_tier", "Plan tier", 1))
		require.NoError(t, err)

		require.NoError(t, repo.Reorder(ctx, tenantID, map[string]int{id1: 5, id2: 0}))

		all, err := repo.All(ctx, tenantID)
		require.NoError(t, err)
		require.Equal(t, "plan_tier", all[0].Slug(), "lower position is listed first")
		require.Equal(t, "country", all[1].Slug())
		require.Equal(t, 5, all[1].Position())
	})
}

func TestFields_ReorderUnknownIDWritesNothing(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantID, other := b.newTenant(t), b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newField(t, tenantID, "country", "Country", 0))
		require.NoError(t, err)
		foreign, err := repo.Add(ctx, other, newField(t, other, "country", "Country", 0))
		require.NoError(t, err)

		err = repo.Reorder(ctx, tenantID, map[string]int{id: 3, foreign: 4})
		require.ErrorIs(t, err, domain.ErrFieldNotFound, "another tenant's field is not found")

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, 0, got.Position(), "the reorder is all or nothing")
	})
}

func TestFields_TenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.fields(t)
		ctx := context.Background()
		tenantA, tenantB := b.newTenant(t), b.newTenant(t)

		idA, err := repo.Add(ctx, tenantA, newField(t, tenantA, "country", "Country", 0))
		require.NoError(t, err)

		// Tenant B must not see tenant A's rows.
		listB, err := repo.All(ctx, tenantB)
		require.NoError(t, err)
		require.Empty(t, listB)

		_, err = repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrFieldNotFound)

		// The same slug is free in another tenant.
		_, err = repo.Add(ctx, tenantB, newField(t, tenantB, "country", "Country", 0))
		require.NoError(t, err)
	})
}
