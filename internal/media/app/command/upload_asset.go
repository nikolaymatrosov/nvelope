// Package command holds the media context's state-changing use cases:
// uploading a new asset and deleting one. Read-only access lives in the
// sibling query package.
package command

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// UploadAsset is the request to add one file to a tenant's media library.
// The content type is detected from the bytes; the client's claim is not
// used.
type UploadAsset struct {
	TenantID   string
	TenantSlug string
	Filename   string
	Body       io.Reader
	// UploadedBy carries the requesting user's id for audit; "" when absent.
	UploadedBy string
}

// UploadAssetResult describes the stored asset. URL is the library URL —
// what content embeds; only workspace members can read it until content
// that uses it is published.
type UploadAssetResult struct {
	AssetID  string
	URL      string
	Filename string
	Width    int
	Height   int
}

// UploadAssetHandler handles the UploadAsset command.
type UploadAssetHandler struct {
	assets   domain.MediaRepository
	blobs    domain.BlobStore
	maxBytes int64
	baseURL  string
}

// NewUploadAssetHandler builds the handler, failing fast on a nil dependency
// or a non-positive size cap. baseURL is the app origin library URLs use.
func NewUploadAssetHandler(assets domain.MediaRepository, blobs domain.BlobStore,
	maxBytes int64, baseURL string) UploadAssetHandler {
	if assets == nil || blobs == nil {
		panic("nil dependency")
	}
	if maxBytes <= 0 {
		panic("media upload size cap must be positive")
	}
	return UploadAssetHandler{assets: assets, blobs: blobs, maxBytes: maxBytes, baseURL: baseURL}
}

// decodeSlots bounds concurrent upload processing in this process: a 25 MP
// image holds about 100 MB of pixels while it is decoded.
// ponytail: per-process limit; move processing to a worker job if upload
// volume ever needs it.
var decodeSlots = make(chan struct{}, 2)

// Handle reads and normalizes the upload, writes the private object first,
// then persists the metadata row. The order matters: an interrupted upload
// leaves an orphaned object (cheap, invisible to the listing) rather than a
// listed-but-missing asset (FR-029).
func (h UploadAssetHandler) Handle(ctx context.Context, cmd UploadAsset) (UploadAssetResult, error) {
	name, err := domain.CleanFilename(cmd.Filename)
	if err != nil {
		return UploadAssetResult{}, err
	}
	data, err := io.ReadAll(io.LimitReader(cmd.Body, h.maxBytes+1))
	if err != nil {
		return UploadAssetResult{}, fmt.Errorf("reading upload: %w", err)
	}
	if len(data) == 0 {
		return UploadAssetResult{}, domain.ErrEmptyUpload
	}
	if int64(len(data)) > h.maxBytes {
		return UploadAssetResult{}, domain.ErrMediaTooLarge
	}
	processed, err := process(ctx, data)
	if err != nil {
		return UploadAssetResult{}, err
	}

	id := uuid.NewString()
	filename := domain.FilenameFor(name, processed.ContentType)
	asset, err := domain.NewMediaAsset(id, cmd.TenantID, filename, processed.ContentType,
		int64(len(processed.Data)), h.maxBytes, processed.Width, processed.Height,
		h.blobs.BuildKey(cmd.TenantID, id, filename), cmd.UploadedBy)
	if err != nil {
		return UploadAssetResult{}, err
	}
	if err := h.blobs.Put(ctx, asset.StorageKey(), asset.ContentType(),
		asset.SizeBytes(), bytes.NewReader(processed.Data)); err != nil {
		return UploadAssetResult{}, err
	}
	if err := h.assets.Add(ctx, asset); err != nil {
		// Best-effort cleanup so a metadata-insert failure leaves no orphan.
		_ = h.blobs.Delete(ctx, asset.StorageKey())
		return UploadAssetResult{}, err
	}
	return UploadAssetResult{
		AssetID:  asset.ID(),
		URL:      domain.LibraryURL(h.baseURL, cmd.TenantSlug, asset.ID()),
		Filename: asset.Filename(),
		Width:    asset.Width(),
		Height:   asset.Height(),
	}, nil
}

// process runs ProcessUpload while holding one of the decode slots.
func process(ctx context.Context, data []byte) (domain.Processed, error) {
	select {
	case decodeSlots <- struct{}{}:
	case <-ctx.Done():
		return domain.Processed{}, ctx.Err()
	}
	defer func() { <-decodeSlots }()
	return domain.ProcessUpload(data)
}
