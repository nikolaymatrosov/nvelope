// Package query holds the media context's read-only use cases — listing a
// tenant's library and looking up one asset.
package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// PreviewTTL is how long a listing's preview URLs stay valid. The SPA
// refetches the listing well before (staleTime 30 min).
const PreviewTTL = time.Hour

// AssetView is the read model for one media asset, shaped for the admin API.
type AssetView struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	// URL is the library URL content embeds.
	URL string `json:"url"`
	// PreviewURL is a presigned read valid for PreviewTTL — for thumbnails
	// and the sandboxed preview iframe, which carries no session cookie.
	PreviewURL string    `json:"preview_url"`
	Width      int       `json:"width,omitempty"`
	Height     int       `json:"height,omitempty"`
	Published  bool      `json:"published"`
	UploadedBy string    `json:"uploaded_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListAssets is the request for one tenant's media library. The slug builds
// the library URLs.
type ListAssets struct {
	TenantID   string
	TenantSlug string
}

// ListAssetsHandler handles the ListAssets query.
type ListAssetsHandler struct {
	assets  domain.MediaRepository
	blobs   domain.BlobStore
	baseURL string
}

// NewListAssetsHandler builds the handler, failing fast on a nil dependency.
func NewListAssetsHandler(assets domain.MediaRepository, blobs domain.BlobStore,
	baseURL string) ListAssetsHandler {
	if assets == nil || blobs == nil {
		panic("nil dependency")
	}
	return ListAssetsHandler{assets: assets, blobs: blobs, baseURL: baseURL}
}

// Handle returns the tenant's media assets, newest first, each with a fresh
// preview URL.
func (h ListAssetsHandler) Handle(ctx context.Context, q ListAssets) ([]AssetView, error) {
	rows, err := h.assets.List(ctx, q.TenantID)
	if err != nil {
		return nil, err
	}
	out := make([]AssetView, 0, len(rows))
	for _, a := range rows {
		preview, err := h.blobs.PresignGet(ctx, a.StorageKey(), PreviewTTL, a.Disposition())
		if err != nil {
			return nil, err
		}
		out = append(out, AssetView{
			ID: a.ID(), Filename: a.Filename(), ContentType: a.ContentType(),
			SizeBytes: a.SizeBytes(), URL: domain.LibraryURL(h.baseURL, q.TenantSlug, a.ID()),
			PreviewURL: preview, Width: a.Width(), Height: a.Height(),
			Published: a.IsPublished(), UploadedBy: a.UploadedBy(), CreatedAt: a.CreatedAt(),
		})
	}
	return out, nil
}
