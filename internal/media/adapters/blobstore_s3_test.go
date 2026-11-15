package adapters_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/media/adapters"
)

func newTestStore(t *testing.T) *adapters.S3BlobStore {
	t.Helper()
	endpoint := dbtest.S3Endpoint(t)
	store, err := adapters.NewS3BlobStore(adapters.S3Config{
		Endpoint:        endpoint,
		Region:          dbtest.S3Region,
		Bucket:          dbtest.S3Bucket,
		AccessKeyID:     dbtest.S3AccessKey,
		SecretAccessKey: dbtest.S3SecretKey,
		PublicBaseURL:   endpoint + "/" + dbtest.S3Bucket,
	})
	require.NoError(t, err)
	return store
}

func TestS3BlobStore_LibraryObjectsArePrivate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	key := store.BuildKey("tenant-a", "asset-1", "hello.png")
	body := []byte("hello-png-bytes")
	require.NoError(t, store.Put(ctx, key, "image/png", int64(len(body)), bytes.NewReader(body)))

	resp, err := http.Get(store.PublicURL(key))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusForbidden, resp.StatusCode,
		"a library object must not be anonymously readable")

	signed, err := store.PresignGet(ctx, key, time.Minute, "inline")
	require.NoError(t, err)
	resp, err = http.Get(signed)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "inline", resp.Header.Get("Content-Disposition"))
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, body, got)

	require.NoError(t, store.Delete(ctx, key))
}

func TestS3BlobStore_PublishMakesAPublicCopy(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	src := store.BuildKey("tenant-a", "asset-2", "price list.pdf")
	dst := store.PublishedKey("tenant-a", "asset-2", "price list.pdf")
	body := []byte("%PDF-1.4 fake")
	require.NoError(t, store.Put(ctx, src, "application/pdf", int64(len(body)), bytes.NewReader(body)))

	const disposition = `attachment; filename="price list.pdf"`
	require.NoError(t, store.Publish(ctx, src, dst, "application/pdf", disposition))
	require.NoError(t, store.Publish(ctx, src, dst, "application/pdf", disposition),
		"publishing again overwrites the copy")

	resp, err := http.Get(store.PublicURL(dst))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/pdf", resp.Header.Get("Content-Type"))
	require.Equal(t, disposition, resp.Header.Get("Content-Disposition"))
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, body, got)

	require.NoError(t, store.Delete(ctx, src))
	again, err := http.Get(store.PublicURL(dst))
	require.NoError(t, err)
	require.NoError(t, again.Body.Close())
	require.Equal(t, http.StatusOK, again.StatusCode, "the public copy outlives the library object")
	require.NoError(t, store.Delete(ctx, dst))
}

func TestS3BlobStore_DeleteMissingIsNotAnError(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Delete(context.Background(),
		store.BuildKey("tenant-a", "no-such-asset", "missing.png")))
}

func TestS3BlobStore_KeysAreTenantPrefixed(t *testing.T) {
	store := newTestStore(t)
	require.Equal(t, "media/tenant-a/asset-1/file.png",
		store.BuildKey("tenant-a", "asset-1", "file.png"))
	require.Equal(t, "media/tenant-b/asset-1/file.png",
		store.BuildKey("tenant-b", "asset-1", "file.png"))
	require.Equal(t, "published/tenant-a/asset-1/file.png",
		store.PublishedKey("tenant-a", "asset-1", "file.png"))
}
