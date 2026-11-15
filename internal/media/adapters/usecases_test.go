package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/media/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/media/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

const useCaseBaseURL = "https://app.test"

// storeAsset adds an asset to the repository and its bytes to the blob store,
// as UploadAsset does, and returns the asset with its library URL.
func storeAsset(t *testing.T, repo domain.MediaRepository, blobs *adapters.MemoryBlobStore,
	tenantID string) (*domain.MediaAsset, string) {
	t.Helper()
	a := newAsset(t, tenantID)
	require.NoError(t, blobs.Put(context.Background(), a.StorageKey(), a.ContentType(), 4,
		bytes.NewReader([]byte("png!"))))
	require.NoError(t, repo.Add(context.Background(), a))
	return a, domain.LibraryURL(useCaseBaseURL, "acme", a.ID())
}

// PublishAssets runs inside the campaign Update closures, which the YDB SDK
// re-runs on a retry. The second run must copy nothing and give the same text.
func TestPublishAssetsIsRepeatable(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		blobs := adapters.NewMemoryBlobStore("https://media.test")
		ctx := context.Background()
		tenantID := b.newTenant(t)
		a, url := storeAsset(t, repo, blobs, tenantID)
		publish := command.NewPublishAssetsHandler(repo, blobs, useCaseBaseURL)

		html := `<img src="` + url + `">`
		first, err := publish.Handle(ctx, command.PublishAssets{TenantID: tenantID, Texts: []string{html}})
		require.NoError(t, err)
		key := publishedKey(a)
		require.Equal(t, `<img src="https://media.test/`+key+`">`, first.Texts[0])
		require.True(t, blobs.IsPublic(key))

		blobs.PublishErr = errors.New("must not copy twice")
		second, err := publish.Handle(ctx, command.PublishAssets{TenantID: tenantID, Texts: []string{html}})
		require.NoError(t, err)
		require.Equal(t, first.Texts, second.Texts)

		got, err := repo.Get(ctx, tenantID, a.ID())
		require.NoError(t, err)
		require.True(t, got.IsPublished())
	})
}

func TestPublishAssetsFailsOnAnotherTenantsAsset(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		blobs := adapters.NewMemoryBlobStore("https://media.test")
		owner, other := b.newTenant(t), b.newTenant(t)
		a, url := storeAsset(t, repo, blobs, owner)

		_, err := command.NewPublishAssetsHandler(repo, blobs, useCaseBaseURL).Handle(context.Background(),
			command.PublishAssets{TenantID: other, Texts: []string{`<img src="` + url + `">`}})
		require.ErrorIs(t, err, domain.ErrMediaMissing)
		require.False(t, blobs.IsPublic(publishedKey(a)), "nothing was copied")
	})
}

// Deleting a library asset keeps its public copy unless the caller asks for
// it to go, as the media-library spec decides.
func TestDeleteAssetKeepsOrRemovesThePublishedCopy(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.assets(t)
		blobs := adapters.NewMemoryBlobStore("https://media.test")
		ctx := context.Background()
		tenantID := b.newTenant(t)
		publish := command.NewPublishAssetsHandler(repo, blobs, useCaseBaseURL)
		del := command.NewDeleteAssetHandler(repo, blobs)

		keep, keepURL := storeAsset(t, repo, blobs, tenantID)
		drop, dropURL := storeAsset(t, repo, blobs, tenantID)
		_, err := publish.Handle(ctx, command.PublishAssets{TenantID: tenantID,
			Texts: []string{keepURL + " " + dropURL}})
		require.NoError(t, err)
		require.True(t, blobs.IsPublic(publishedKey(keep)))
		require.True(t, blobs.IsPublic(publishedKey(drop)))

		res, err := del.Handle(ctx, command.DeleteAsset{TenantID: tenantID, AssetID: keep.ID()})
		require.NoError(t, err)
		require.False(t, res.Unpublished)
		require.True(t, blobs.IsPublic(publishedKey(keep)), "the public copy outlives the library asset")

		res, err = del.Handle(ctx, command.DeleteAsset{TenantID: tenantID, AssetID: drop.ID(), Unpublish: true})
		require.NoError(t, err)
		require.True(t, res.Unpublished)
		require.False(t, blobs.IsPublic(publishedKey(drop)))
		require.True(t, blobs.IsPublic(publishedKey(keep)))

		list, err := repo.List(ctx, tenantID)
		require.NoError(t, err)
		require.Empty(t, list)
		_, err = del.Handle(ctx, command.DeleteAsset{TenantID: tenantID, AssetID: keep.ID()})
		require.ErrorIs(t, err, domain.ErrMediaNotFound)
	})
}

func publishedKey(a *domain.MediaAsset) string {
	return "published/" + a.TenantID() + "/" + a.ID() + "/" + a.Filename()
}
