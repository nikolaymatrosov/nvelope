package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// ContentURLTTL is how long the URL behind a library URL's redirect stays
// valid. The API caches the redirect for 10 minutes, so a cached redirect
// never points at an expired signature.
const ContentURLTTL = 15 * time.Minute

// AssetContentURL is the request for a short-lived URL that reads one
// library asset.
type AssetContentURL struct {
	TenantID string
	AssetID  string
}

// AssetContentURLHandler handles the AssetContentURL query.
type AssetContentURLHandler struct {
	assets domain.MediaRepository
	blobs  domain.BlobStore
}

// NewAssetContentURLHandler builds the handler, failing fast on a nil dependency.
func NewAssetContentURLHandler(assets domain.MediaRepository, blobs domain.BlobStore) AssetContentURLHandler {
	if assets == nil || blobs == nil {
		panic("nil dependency")
	}
	return AssetContentURLHandler{assets: assets, blobs: blobs}
}

// Handle loads the asset under RLS (another tenant's id is not found) and
// presigns a read of its private object.
func (h AssetContentURLHandler) Handle(ctx context.Context, q AssetContentURL) (string, error) {
	a, err := h.assets.Get(ctx, q.TenantID, q.AssetID)
	if err != nil {
		return "", err
	}
	return h.blobs.PresignGet(ctx, a.StorageKey(), ContentURLTTL, a.Disposition())
}
