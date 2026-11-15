package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/media/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// backend is one storage implementation of the media repository under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name   string
	assets func(t *testing.T) domain.MediaRepository
	// newTenant is a real row on Postgres, where media_assets carries a
	// foreign key to tenants. YDB has none, so a bare UUID stands in.
	newTenant func(t *testing.T) string
}

var backends = []backend{
	{
		name: "pg",
		assets: func(t *testing.T) domain.MediaRepository {
			return adapters.NewAssets(dbtest.AppPool(t))
		},
		newTenant: func(t *testing.T) string { return seedTenant(t, dbtest.AppPool(t)) },
	},
	{
		name: "ydb",
		assets: func(t *testing.T) domain.MediaRepository {
			return adapters.NewYDBAssets(dbtest.YDBMigrated(t))
		},
		newTenant: func(*testing.T) string { return uuid.NewString() },
	},
}

func eachBackend(t *testing.T, body func(t *testing.T, b backend)) {
	t.Helper()
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			body(t, b)
		})
	}
}

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
	a, err := domain.NewMediaAsset(id, tenantID, "logo.png", "image/png", 42, 1<<20, 64, 32,
		"media/"+tenantID+"/"+id+"/logo.png", "")
	require.NoError(t, err)
	return a
}

func TestAssetsAddGetListDelete(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		a := newAsset(t, tenantID)
		require.NoError(t, repo.Add(ctx, a))

		got, err := repo.Get(ctx, tenantID, a.ID())
		require.NoError(t, err)
		require.Equal(t, "logo.png", got.Filename())
		require.Equal(t, "image/png", got.ContentType())
		require.EqualValues(t, 42, got.SizeBytes())
		require.Equal(t, 64, got.Width())
		require.Equal(t, 32, got.Height())
		require.Equal(t, a.StorageKey(), got.StorageKey())
		require.Equal(t, tenantID, got.TenantID())
		require.False(t, got.IsPublished())
		require.WithinDuration(t, time.Now(), got.CreatedAt(), time.Minute)

		list, err := repo.List(ctx, tenantID)
		require.NoError(t, err)
		require.Len(t, list, 1)

		require.NoError(t, repo.Delete(ctx, tenantID, a.ID()))
		_, err = repo.Get(ctx, tenantID, a.ID())
		require.ErrorIs(t, err, domain.ErrMediaNotFound)
	})
}

func TestAssetsGetUnknownIsNotFound(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Get(ctx, tenantID, uuid.NewString())
		require.ErrorIs(t, err, domain.ErrMediaNotFound)
		_, err = repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrMediaNotFound)
		require.ErrorIs(t, repo.Delete(ctx, tenantID, uuid.NewString()), domain.ErrMediaNotFound)
	})
}

func TestAssetsAreTenantScoped(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		ctx := context.Background()
		owner, other := b.newTenant(t), b.newTenant(t)

		a := newAsset(t, owner)
		require.NoError(t, repo.Add(ctx, a))

		_, err := repo.Get(ctx, other, a.ID())
		require.ErrorIs(t, err, domain.ErrMediaNotFound)
		require.ErrorIs(t, repo.Delete(ctx, other, a.ID()), domain.ErrMediaNotFound)
		require.ErrorIs(t, repo.MarkPublished(ctx, other, a.ID(), time.Now()), domain.ErrMediaNotFound)
		list, err := repo.List(ctx, other)
		require.NoError(t, err)
		require.Empty(t, list)

		got, err := repo.Get(ctx, owner, a.ID())
		require.NoError(t, err)
		require.False(t, got.IsPublished(), "another tenant's MarkPublished changed nothing")
	})
}

func TestAssetsListOrdersNewestFirst(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		first := newAsset(t, tenantID)
		require.NoError(t, repo.Add(ctx, first))
		second := newAsset(t, tenantID)
		require.NoError(t, repo.Add(ctx, second))

		list, err := repo.List(ctx, tenantID)
		require.NoError(t, err)
		require.Len(t, list, 2)
		require.Equal(t, second.ID(), list[0].ID(), "the newest asset is returned first")
	})
}

func TestAssetsMarkPublished(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		a := newAsset(t, tenantID)
		require.NoError(t, repo.Add(ctx, a))
		first := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
		require.NoError(t, repo.MarkPublished(ctx, tenantID, a.ID(), first))
		require.NoError(t, repo.MarkPublished(ctx, tenantID, a.ID(), time.Now()), "marking twice is fine")

		got, err := repo.Get(ctx, tenantID, a.ID())
		require.NoError(t, err)
		require.True(t, got.IsPublished())

		require.ErrorIs(t, repo.MarkPublished(ctx, tenantID, uuid.NewString(), time.Now()),
			domain.ErrMediaNotFound)
	})
}

func TestAssetsStorePDFWithoutDimensions(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id := uuid.NewString()
		pdf, err := domain.NewMediaAsset(id, tenantID, "a.pdf", "application/pdf", 42, 1<<20, 0, 0,
			"media/"+tenantID+"/"+id+"/a.pdf", "")
		require.NoError(t, err)
		require.NoError(t, repo.Add(ctx, pdf))
		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Zero(t, got.Width())
		require.Zero(t, got.Height())
		require.Empty(t, got.UploadedBy())
	})
}

// The uploader is a real user on Postgres (a foreign key), which the media
// package cannot seed, so this one runs on YDB only.
func TestYDBAssetsStoreTheUploader(t *testing.T) {
	t.Parallel()
	repo := adapters.NewYDBAssets(dbtest.YDBMigrated(t))
	ctx := context.Background()
	tenantID, user := uuid.NewString(), uuid.NewString()

	id := uuid.NewString()
	a, err := domain.NewMediaAsset(id, tenantID, "logo.png", "image/png", 42, 1<<20, 64, 32,
		"media/"+tenantID+"/"+id+"/logo.png", user)
	require.NoError(t, err)
	require.NoError(t, repo.Add(ctx, a))

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, user, got.UploadedBy())
	require.Error(t, repo.Add(ctx, a), "the id is the key: a second Add of it fails")
}
