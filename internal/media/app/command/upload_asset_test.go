package command_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

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

func TestUploadAsset_persistsAfterBytesWritten(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	upload := command.NewUploadAssetHandler(repo, blobs, 1<<20)

	body := []byte("hello-png")
	result, err := upload.Handle(context.Background(), command.UploadAsset{
		TenantID:    "tenant-1",
		Filename:    "logo.png",
		ContentType: "image/png",
		SizeBytes:   int64(len(body)),
		Body:        bytes.NewReader(body),
		UploadedBy:  "user-1",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.AssetID)
	require.Equal(t, "https://media.test/media/tenant-1/"+result.AssetID+"/logo.png", result.PublicURL)

	stored, ct, ok := blobs.Get("media/tenant-1/" + result.AssetID + "/logo.png")
	require.True(t, ok, "object must be written")
	require.Equal(t, body, stored)
	require.Equal(t, "image/png", ct)

	list, err := query.NewListAssetsHandler(repo).Handle(context.Background(),
		query.ListAssets{TenantID: "tenant-1"})
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "logo.png", list[0].Filename)
}

func TestUploadAsset_rejectsUnsupportedType(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	upload := command.NewUploadAssetHandler(repo, blobs, 1<<20)

	_, err := upload.Handle(context.Background(), command.UploadAsset{
		TenantID:    "tenant-1",
		Filename:    "evil.exe",
		ContentType: "application/x-msdownload",
		SizeBytes:   16,
		Body:        bytes.NewReader(bytes.Repeat([]byte("x"), 16)),
	})
	require.ErrorIs(t, err, domain.ErrUnsupportedMediaType)
	require.Zero(t, blobs.Len(), "no bytes should be written when the type is rejected")
	require.Empty(t, repo.items)
}

func TestUploadAsset_rejectsOversize(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	upload := command.NewUploadAssetHandler(repo, blobs, 4)

	_, err := upload.Handle(context.Background(), command.UploadAsset{
		TenantID:    "tenant-1",
		Filename:    "big.png",
		ContentType: "image/png",
		SizeBytes:   8,
		Body:        bytes.NewReader(bytes.Repeat([]byte("x"), 8)),
	})
	require.ErrorIs(t, err, domain.ErrMediaTooLarge)
	require.Zero(t, blobs.Len())
}

func TestUploadAsset_rejectsEmpty(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	upload := command.NewUploadAssetHandler(repo, blobs, 1<<20)

	_, err := upload.Handle(context.Background(), command.UploadAsset{
		TenantID:    "tenant-1",
		Filename:    "empty.png",
		ContentType: "image/png",
		SizeBytes:   0,
		Body:        bytes.NewReader(nil),
	})
	require.ErrorIs(t, err, domain.ErrEmptyUpload)
	require.Zero(t, blobs.Len())
}

func TestUploadAsset_metadataFailureCompensatesBlob(t *testing.T) {
	repo := &fakeAssets{AddErr: errors.New("database is sad")}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	upload := command.NewUploadAssetHandler(repo, blobs, 1<<20)

	body := []byte("hello-png")
	_, err := upload.Handle(context.Background(), command.UploadAsset{
		TenantID:    "tenant-1",
		Filename:    "logo.png",
		ContentType: "image/png",
		SizeBytes:   int64(len(body)),
		Body:        bytes.NewReader(body),
	})
	require.Error(t, err)
	require.Zero(t, blobs.Len(), "an orphaned object must be cleaned up on metadata failure")
}

func TestDeleteAsset_removesRowAndObject(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	upload := command.NewUploadAssetHandler(repo, blobs, 1<<20)
	del := command.NewDeleteAssetHandler(repo, blobs)

	body := []byte("hello-png")
	result, err := upload.Handle(context.Background(), command.UploadAsset{
		TenantID:    "tenant-1",
		Filename:    "logo.png",
		ContentType: "image/png",
		SizeBytes:   int64(len(body)),
		Body:        bytes.NewReader(body),
	})
	require.NoError(t, err)

	require.NoError(t, del.Handle(context.Background(), command.DeleteAsset{
		TenantID: "tenant-1", AssetID: result.AssetID,
	}))
	require.Zero(t, blobs.Len())
	_, err = repo.Get(context.Background(), "tenant-1", result.AssetID)
	require.ErrorIs(t, err, domain.ErrMediaNotFound)
}

func TestDeleteAsset_unknownIsNotFound(t *testing.T) {
	repo := &fakeAssets{}
	blobs := adapters.NewMemoryBlobStore("https://media.test")
	del := command.NewDeleteAssetHandler(repo, blobs)

	err := del.Handle(context.Background(), command.DeleteAsset{
		TenantID: "tenant-1", AssetID: "nope",
	})
	require.ErrorIs(t, err, domain.ErrMediaNotFound)
}
