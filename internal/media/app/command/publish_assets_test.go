package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/media/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/media/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

func TestPublishAssets_copiesOnceAndRewrites(t *testing.T) {
	ctx := context.Background()
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png")
	publish := command.NewPublishAssetsHandler(repo, blobs, "https://app.test")

	html := `<img src="` + logo.URL + `"><td style="background:url(` + logo.URL + `)">`
	text := "Logo: " + logo.URL
	res, err := publish.Handle(ctx, command.PublishAssets{TenantID: "tenant-1", Texts: []string{html, text}})
	require.NoError(t, err)

	key := "published/tenant-1/" + logo.AssetID + "/logo.png"
	public := "https://media.test/" + key
	require.Equal(t, []string{
		`<img src="` + public + `"><td style="background:url(` + public + `)">`,
		"Logo: " + public,
	}, res.Texts)
	_, ct, ok := blobs.Get(key)
	require.True(t, ok)
	require.Equal(t, "image/png", ct)
	require.True(t, blobs.IsPublic(key))
	require.Equal(t, "inline", blobs.DispositionOf(key))
	asset, err := repo.Get(ctx, "tenant-1", logo.AssetID)
	require.NoError(t, err)
	require.True(t, asset.IsPublished())

	// A published asset is not copied again.
	blobs.PublishErr = errors.New("must not copy twice")
	again, err := publish.Handle(ctx, command.PublishAssets{TenantID: "tenant-1", Texts: []string{html}})
	require.NoError(t, err)
	require.Equal(t, res.Texts[0], again.Texts[0])
}

func TestPublishAssets_missingAssetPublishesNothing(t *testing.T) {
	ctx := context.Background()
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png")
	ghost := uuid.NewString()
	html := `<img src="` + logo.URL + `"><img src="` +
		domain.LibraryURL("https://app.test", "acme", ghost) + `">`

	_, err := command.NewPublishAssetsHandler(repo, blobs, "https://app.test").Handle(ctx,
		command.PublishAssets{TenantID: "tenant-1", Texts: []string{html}})
	require.ErrorIs(t, err, domain.ErrMediaMissing)
	require.ErrorContains(t, err, ghost)
	require.Equal(t, 1, blobs.Len(), "only the library object exists; nothing was published")
}

func TestPublishAssets_foreignTenantAssetIsMissing(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png") // owned by tenant-1

	_, err := command.NewPublishAssetsHandler(repo, blobs, "https://app.test").Handle(context.Background(),
		command.PublishAssets{TenantID: "tenant-2", Texts: []string{`<img src="` + logo.URL + `">`}})
	require.ErrorIs(t, err, domain.ErrMediaMissing)
}

func TestPublishAssets_passesThroughTextWithoutLibraryURLs(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	in := []string{`<img src="https://cdn.example/x.png">`, ""}
	res, err := command.NewPublishAssetsHandler(repo, blobs, "https://app.test").Handle(context.Background(),
		command.PublishAssets{TenantID: "tenant-1", Texts: in})
	require.NoError(t, err)
	require.Equal(t, in, res.Texts)
}
