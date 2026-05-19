package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// DeleteAsset is the request to remove one media asset.
type DeleteAsset struct {
	TenantID string
	AssetID  string
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
// metadata row, and then deletes the object. The metadata row is the source
// of truth: once it is gone, the listing no longer shows the asset, even if
// the subsequent object delete fails and leaves an orphan in storage.
func (h DeleteAssetHandler) Handle(ctx context.Context, cmd DeleteAsset) error {
	asset, err := h.assets.Get(ctx, cmd.TenantID, cmd.AssetID)
	if err != nil {
		return err
	}
	if err := h.assets.Delete(ctx, cmd.TenantID, cmd.AssetID); err != nil {
		return err
	}
	return h.blobs.Delete(ctx, asset.StorageKey())
}
