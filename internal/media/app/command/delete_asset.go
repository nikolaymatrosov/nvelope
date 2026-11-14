package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// DeleteAsset is the request to remove one media asset.
type DeleteAsset struct {
	TenantID string
	AssetID  string
	// Unpublish also removes the asset's public copy, which breaks the image
	// in sent emails and the archive. It exists for takedown requests and
	// mistakes; by default the public copy outlives the library asset.
	Unpublish bool
}

// DeleteAssetResult reports what was removed, for the audit trail.
type DeleteAssetResult struct {
	Filename string
	// Unpublished is true when a public copy existed and was removed.
	Unpublished bool
}

// DeleteAssetHandler handles the DeleteAsset command.
type DeleteAssetHandler struct {
	assets domain.MediaRepository
	blobs  domain.BlobStore
}

// NewDeleteAssetHandler builds the handler, failing fast on a nil dependency.
func NewDeleteAssetHandler(assets domain.MediaRepository, blobs domain.BlobStore) DeleteAssetHandler {
	if assets == nil || blobs == nil {
		panic("nil dependency")
	}
	return DeleteAssetHandler{assets: assets, blobs: blobs}
}

// Handle loads the asset (so RLS confirms the tenant owns it), removes the
// public copy first when asked (so a failure leaves everything in place for
// a retry), then the metadata row, then the private object. The row is the
// source of truth: once it is gone the listing no longer shows the asset,
// even if the object delete fails and leaves an orphan in storage.
func (h DeleteAssetHandler) Handle(ctx context.Context, cmd DeleteAsset) (DeleteAssetResult, error) {
	asset, err := h.assets.Get(ctx, cmd.TenantID, cmd.AssetID)
	if err != nil {
		return DeleteAssetResult{}, err
	}
	res := DeleteAssetResult{Filename: asset.Filename()}
	if cmd.Unpublish && asset.IsPublished() {
		key := h.blobs.PublishedKey(asset.TenantID(), asset.ID(), asset.Filename())
		if err := h.blobs.Delete(ctx, key); err != nil {
			return DeleteAssetResult{}, err
		}
		res.Unpublished = true
	}
	if err := h.assets.Delete(ctx, cmd.TenantID, cmd.AssetID); err != nil {
		return DeleteAssetResult{}, err
	}
	return res, h.blobs.Delete(ctx, asset.StorageKey())
}
