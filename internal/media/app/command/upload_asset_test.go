package command_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/media/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/media/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/media/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// fakeAssets is an in-memory MediaRepository for use-case tests. It keeps the
// adapters layer out of these tests so they exercise only the command/query
// orchestration.
type fakeAssets struct {
	mu     sync.Mutex
	tenant string
	items  []*domain.MediaAsset
	// AddErr, when non-nil, makes the next Add call fail — used to assert the
	// upload command compensates by deleting the orphaned object.
	AddErr error
}

func (f *fakeAssets) Add(_ context.Context, a *domain.MediaAsset) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.AddErr != nil {
		err := f.AddErr
		f.AddErr = nil
		return err
	}
	if f.tenant == "" {
		f.tenant = a.TenantID()
	}
	f.items = append(f.items, a)
	return nil
}

func (f *fakeAssets) Get(_ context.Context, tenantID, id string) (*domain.MediaAsset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.items {
		if a.TenantID() == tenantID && a.ID() == id {
			return a, nil
		}
	}
	return nil, domain.ErrMediaNotFound
}

func (f *fakeAssets) List(_ context.Context, tenantID string) ([]*domain.MediaAsset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*domain.MediaAsset{}
	for _, a := range f.items {
		if a.TenantID() == tenantID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeAssets) Delete(_ context.Context, tenantID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, a := range f.items {
		if a.TenantID() == tenantID && a.ID() == id {
			f.items = append(f.items[:i], f.items[i+1:]...)
			return nil
		}
	}
	return domain.ErrMediaNotFound
}

func (f *fakeAssets) MarkPublished(_ context.Context, tenantID, id string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, a := range f.items {
		if a.TenantID() == tenantID && a.ID() == id {
			if !a.IsPublished() {
				f.items[i] = domain.HydrateMediaAsset(a.ID(), a.TenantID(), a.Filename(),
					a.ContentType(), a.SizeBytes(), a.Width(), a.Height(), a.StorageKey(),
					a.UploadedBy(), &at, a.CreatedAt())
			}
			return nil
		}
	}
	return domain.ErrMediaNotFound
}

const webpAlpha1x1 = "UklGRkoAAABXRUJQVlA4WAoAAAAQAAAAAAAAAAAAQUxQSAwAAAARBxAR/Q9ERP8DAABWUDggGAAAABQBAJ0BKgEAAQAAAP4AAA3AAP7mtQAAAA=="

// pngBytes encodes a transparent w×h PNG.
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, w, h))))
	return buf.Bytes()
}

func newUploader(repo *fakeAssets, blobs *adapters.MemoryBlobStore, maxBytes int64) command.UploadAssetHandler {
	return command.NewUploadAssetHandler(repo, blobs, maxBytes, "https://app.test")
}

// uploadPNG uploads a 4×4 PNG named name for tenant-1 (slug acme).
func uploadPNG(t *testing.T, repo *fakeAssets, blobs *adapters.MemoryBlobStore, name string) command.UploadAssetResult {
	t.Helper()
	res, err := newUploader(repo, blobs, 1<<20).Handle(context.Background(), command.UploadAsset{
		TenantID: "tenant-1", TenantSlug: "acme", Filename: name,
		Body: bytes.NewReader(pngBytes(t, 4, 4)),
	})
	require.NoError(t, err)
	return res
}

func TestUploadAsset_persistsAfterBytesWritten(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")

	result, err := newUploader(repo, blobs, 1<<20).Handle(context.Background(), command.UploadAsset{
		TenantID: "tenant-1", TenantSlug: "acme", Filename: "logo.png",
		Body: bytes.NewReader(pngBytes(t, 10, 10)), UploadedBy: "user-1",
	})
	require.NoError(t, err)
	require.Equal(t, "https://app.test/t/acme/api/media/"+result.AssetID+"/content", result.URL)
	require.Equal(t, 10, result.Width)

	_, ct, ok := blobs.Get("media/tenant-1/" + result.AssetID + "/logo.png")
	require.True(t, ok, "object must be written")
	require.Equal(t, "image/png", ct)
	require.False(t, blobs.IsPublic("media/tenant-1/"+result.AssetID+"/logo.png"))

	list, err := query.NewListAssetsHandler(repo, blobs, "https://app.test").Handle(context.Background(),
		query.ListAssets{TenantID: "tenant-1", TenantSlug: "acme"})
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, result.URL, list[0].URL)
	require.Equal(t, "https://media.test/media/tenant-1/"+result.AssetID+"/logo.png?signature=test",
		list[0].PreviewURL)
	require.Equal(t, 10, list[0].Width)
	require.False(t, list[0].Published)
}

func TestUploadAsset_detectsTypeFromBytes(t *testing.T) {
	for name, body := range map[string][]byte{
		"evil.png": []byte("MZ\x90\x00 an executable"),
		"logo.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
	} {
		repo := &fakeAssets{}
		blobs := adapters.NewMemoryBlobStore("https://media.test")
		_, err := newUploader(repo, blobs, 1<<20).Handle(context.Background(), command.UploadAsset{
			TenantID: "tenant-1", TenantSlug: "acme", Filename: name, Body: bytes.NewReader(body),
		})
		require.ErrorIs(t, err, domain.ErrUnsupportedMediaType, name)
		require.Zero(t, blobs.Len(), "no bytes are written when the type is rejected")
		require.Empty(t, repo.items)
	}
}

func TestUploadAsset_convertsWebP(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	webp, err := base64.StdEncoding.DecodeString(webpAlpha1x1)
	require.NoError(t, err)

	result, err := newUploader(repo, blobs, 1<<20).Handle(context.Background(), command.UploadAsset{
		TenantID: "tenant-1", TenantSlug: "acme", Filename: "photo.webp", Body: bytes.NewReader(webp),
	})
	require.NoError(t, err)
	require.Equal(t, "photo.png", result.Filename)
	_, ct, ok := blobs.Get("media/tenant-1/" + result.AssetID + "/photo.png")
	require.True(t, ok)
	require.Equal(t, "image/png", ct)
}

func TestUploadAsset_rejectsOversize(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	_, err := newUploader(repo, blobs, 4).Handle(context.Background(), command.UploadAsset{
		TenantID: "tenant-1", TenantSlug: "acme", Filename: "big.png",
		Body: bytes.NewReader(bytes.Repeat([]byte("x"), 8)),
	})
	require.ErrorIs(t, err, domain.ErrMediaTooLarge)
	require.Zero(t, blobs.Len())
}

func TestUploadAsset_rejectsEmpty(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	_, err := newUploader(repo, blobs, 1<<20).Handle(context.Background(), command.UploadAsset{
		TenantID: "tenant-1", TenantSlug: "acme", Filename: "empty.png", Body: bytes.NewReader(nil),
	})
	require.ErrorIs(t, err, domain.ErrEmptyUpload)
	require.Zero(t, blobs.Len())
}

func TestUploadAsset_metadataFailureCompensatesBlob(t *testing.T) {
	repo := &fakeAssets{AddErr: errors.New("database is sad")}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	_, err := newUploader(repo, blobs, 1<<20).Handle(context.Background(), command.UploadAsset{
		TenantID: "tenant-1", TenantSlug: "acme", Filename: "logo.png",
		Body: bytes.NewReader(pngBytes(t, 4, 4)),
	})
	require.Error(t, err)
	require.Zero(t, blobs.Len(), "an orphaned object must be cleaned up on metadata failure")
}

func TestAssetContentURL_presignsTheLibraryObject(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png")

	got, err := query.NewAssetContentURLHandler(repo, blobs).Handle(context.Background(),
		query.AssetContentURL{TenantID: "tenant-1", AssetID: logo.AssetID})
	require.NoError(t, err)
	require.Equal(t, "https://media.test/media/tenant-1/"+logo.AssetID+"/logo.png?signature=test", got)

	_, err = query.NewAssetContentURLHandler(repo, blobs).Handle(context.Background(),
		query.AssetContentURL{TenantID: "tenant-2", AssetID: logo.AssetID})
	require.ErrorIs(t, err, domain.ErrMediaNotFound)
}

func TestDeleteAsset_removesRowAndObject(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png")

	res, err := command.NewDeleteAssetHandler(repo, blobs).Handle(context.Background(),
		command.DeleteAsset{TenantID: "tenant-1", AssetID: logo.AssetID})
	require.NoError(t, err)
	require.False(t, res.Unpublished)
	require.Zero(t, blobs.Len())
	_, err = repo.Get(context.Background(), "tenant-1", logo.AssetID)
	require.ErrorIs(t, err, domain.ErrMediaNotFound)
}

func TestDeleteAsset_unknownIsNotFound(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	_, err := command.NewDeleteAssetHandler(repo, blobs).Handle(context.Background(),
		command.DeleteAsset{TenantID: "tenant-1", AssetID: "nope"})
	require.ErrorIs(t, err, domain.ErrMediaNotFound)
}

func TestDeleteAsset_keepsPublicCopyByDefault(t *testing.T) {
	ctx := context.Background()
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png")
	_, err := command.NewPublishAssetsHandler(repo, blobs, "https://app.test").Handle(ctx,
		command.PublishAssets{TenantID: "tenant-1", Texts: []string{logo.URL}})
	require.NoError(t, err)

	res, err := command.NewDeleteAssetHandler(repo, blobs).Handle(ctx,
		command.DeleteAsset{TenantID: "tenant-1", AssetID: logo.AssetID})
	require.NoError(t, err)
	require.False(t, res.Unpublished)
	_, _, ok := blobs.Get("published/tenant-1/" + logo.AssetID + "/logo.png")
	require.True(t, ok, "sent emails keep working")
	require.Equal(t, 1, blobs.Len())
}

func TestDeleteAsset_unpublishRemovesPublicCopy(t *testing.T) {
	ctx := context.Background()
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	logo := uploadPNG(t, repo, blobs, "logo.png")
	_, err := command.NewPublishAssetsHandler(repo, blobs, "https://app.test").Handle(ctx,
		command.PublishAssets{TenantID: "tenant-1", Texts: []string{logo.URL}})
	require.NoError(t, err)

	res, err := command.NewDeleteAssetHandler(repo, blobs).Handle(ctx,
		command.DeleteAsset{TenantID: "tenant-1", AssetID: logo.AssetID, Unpublish: true})
	require.NoError(t, err)
	require.True(t, res.Unpublished)
	require.Equal(t, "logo.png", res.Filename)
	require.Zero(t, blobs.Len())
}
