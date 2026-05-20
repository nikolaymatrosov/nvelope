package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/media/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"media-"+dbtest.RandString()).Scan(&id))
	return id
}

func newAsset(t *testing.T, tenantID string) *domain.MediaAsset {
	t.Helper()
	id := uuid.NewString()
	a, err := domain.NewMediaAsset(id, tenantID, "logo.png", "image/png", 42, 1<<20,
		"media/"+tenantID+"/"+id+"/logo.png",
		"https://media.test/media/"+tenantID+"/"+id+"/logo.png", "")
	require.NoError(t, err)
	return a
}

func TestAssetsAddGetListDelete(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewAssets(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	a := newAsset(t, tenantID)
	require.NoError(t, repo.Add(ctx, a))

	got, err := repo.Get(ctx, tenantID, a.ID())
	require.NoError(t, err)
	require.Equal(t, "logo.png", got.Filename())
	require.Equal(t, "image/png", got.ContentType())
	require.EqualValues(t, 42, got.SizeBytes())
	require.NotEmpty(t, got.CreatedAt())

	list, err := repo.List(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, repo.Delete(ctx, tenantID, a.ID()))
	_, err = repo.Get(ctx, tenantID, a.ID())
	require.ErrorIs(t, err, domain.ErrMediaNotFound)
}

func TestAssetsGetUnknownIsNotFound(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewAssets(pool)
	tenantID := seedTenant(t, pool)

	_, err := repo.Get(context.Background(), tenantID, uuid.NewString())
	require.ErrorIs(t, err, domain.ErrMediaNotFound)

	err = repo.Delete(context.Background(), tenantID, uuid.NewString())
	require.ErrorIs(t, err, domain.ErrMediaNotFound)
}

func TestAssetsListOrdersNewestFirst(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewAssets(pool)
	tenantID := seedTenant(t, pool)
	ctx := context.Background()

	first := newAsset(t, tenantID)
	require.NoError(t, repo.Add(ctx, first))
	second := newAsset(t, tenantID)
	require.NoError(t, repo.Add(ctx, second))

	list, err := repo.List(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, second.ID(), list[0].ID(),
		"the newest asset is returned first")
}
