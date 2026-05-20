// Package command holds the media context's state-changing use cases:
// uploading a new asset and deleting one. Read-only access lives in the
// sibling query package.
package command

import (
	"context"
	"io"
	"path"
	"strings"

	"github.com/google/uuid"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// UploadAsset is the request to add one file to a tenant's media library.
type UploadAsset struct {
	TenantID    string
	Filename    string
	ContentType string
	SizeBytes   int64
	Body        io.Reader
	// UploadedBy carries the requesting user's id for audit; "" when absent.
	UploadedBy string
}

// UploadAssetResult carries the asset's id and stable public URL — the caller
// embeds the URL in campaign content and identifies the asset by id later.
type UploadAssetResult struct {
	AssetID   string
	PublicURL string
}

// UploadAssetHandler handles the UploadAsset command.
type UploadAssetHandler struct {
	assets   domain.MediaRepository
	blobs    domain.BlobStore
	maxBytes int64
}

// NewUploadAssetHandler builds the handler, failing fast on a nil dependency
// or a non-positive size cap.
func NewUploadAssetHandler(assets domain.MediaRepository, blobs domain.BlobStore,
	maxBytes int64) UploadAssetHandler {
	if assets == nil || blobs == nil {
		panic("nil dependency")
	}
	if maxBytes <= 0 {
		panic("media upload size cap must be positive")
	}
	return UploadAssetHandler{assets: assets, blobs: blobs, maxBytes: maxBytes}
}

// Handle generates the asset id, writes the bytes to object storage first,
// then persists the metadata row. The order matters: an interrupted upload
// leaves an orphaned object (cheap, invisible to the listing) rather than a
// listed-but-missing asset (FR-029).
func (h UploadAssetHandler) Handle(ctx context.Context, cmd UploadAsset) (UploadAssetResult, error) {
	id := uuid.NewString()
	keyFilename := path.Base(strings.TrimSpace(cmd.Filename))
	storageKey := h.blobs.BuildKey(cmd.TenantID, id, keyFilename)
	publicURL := h.blobs.PublicURL(storageKey)
	asset, err := domain.NewMediaAsset(id, cmd.TenantID, cmd.Filename, cmd.ContentType,
		cmd.SizeBytes, h.maxBytes, storageKey, publicURL, cmd.UploadedBy)
	if err != nil {
		return UploadAssetResult{}, err
	}
	if err := h.blobs.Put(ctx, asset.StorageKey(), asset.ContentType(),
		asset.SizeBytes(), cmd.Body); err != nil {
		return UploadAssetResult{}, err
	}
	if err := h.assets.Add(ctx, asset); err != nil {
		// Best-effort cleanup of the orphaned object so a metadata-insert
		// failure doesn't leave a publicly fetchable file behind.
		_ = h.blobs.Delete(ctx, asset.StorageKey())
		return UploadAssetResult{}, err
	}
	return UploadAssetResult{AssetID: asset.ID(), PublicURL: asset.PublicURL()}, nil
}
