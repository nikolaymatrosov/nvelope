package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func newTemplate(t *testing.T, tenantID, name string) *domain.Template {
	t.Helper()
	tpl, err := domain.NewTemplate(tenantID, name, domain.KindCampaign, "Subject", "<p>b</p>", "")
	require.NoError(t, err)
	return tpl
}

func TestTemplatesAddGetAll(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTemplates(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id, err := repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Welcome"))
	require.NoError(t, err)

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, "Welcome", got.Name())
	require.Equal(t, domain.KindCampaign, got.Kind())

	all, total, err := repo.All(ctx, tenantID, domain.DefaultPage)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, all, 1)
}

func TestTemplatesDuplicateNameConflicts(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTemplates(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	_, err := repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Dup"))
	require.NoError(t, err)
	_, err = repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Dup"))
	require.ErrorIs(t, err, domain.ErrTemplateNameTaken)
}

func TestTemplatesUpdate(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTemplates(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id, err := repo.Add(ctx, tenantID, newTemplate(t, tenantID, "Old"))
	require.NoError(t, err)
	require.NoError(t, repo.Update(ctx, tenantID, id,
		func(tpl *domain.Template) (*domain.Template, error) {
			return tpl, tpl.Recompose("New", "New subject", "<p>new</p>", "")
		}))

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, "New", got.Name())
}

func TestTemplatesGetMissing(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTemplates(pool)
	tenantID := seedTenant(t, pool)

	_, err := repo.Get(context.Background(), tenantID, "not-a-uuid")
	require.ErrorIs(t, err, domain.ErrTemplateNotFound)
}

func TestTemplatesCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewTemplates(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)

	idA, err := repo.Add(ctx, tenantA, newTemplate(t, tenantA, "Welcome"))
	require.NoError(t, err)

	_, err = repo.Get(ctx, tenantB, idA)
	require.ErrorIs(t, err, domain.ErrTemplateNotFound)

	all, _, err := repo.All(ctx, tenantB, domain.DefaultPage)
	require.NoError(t, err)
	require.Empty(t, all)
}
