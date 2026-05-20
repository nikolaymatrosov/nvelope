// Package query holds the media context's read-only use cases — listing a
// tenant's library and looking up one asset.
package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// AssetView is the read model for one media asset, shaped for the admin API.
type AssetView struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	PublicURL   string    `json:"public_url"`
	UploadedBy  string    `json:"uploaded_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func viewFor(a *domain.MediaAsset) AssetView {
	return AssetView{
		ID: a.ID(), Filename: a.Filename(), ContentType: a.ContentType(),
		SizeBytes: a.SizeBytes(), PublicURL: a.PublicURL(),
		UploadedBy: a.UploadedBy(), CreatedAt: a.CreatedAt(),
	}
}

// ListAssets is the request for one tenant's media library.
type ListAssets struct {
	TenantID string
}

// ListAssetsHandler handles the ListAssets query.
type ListAssetsHandler struct {
	assets domain.MediaRepository
}

// NewListAssetsHandler builds the handler, failing fast on a nil dependency.
func NewListAssetsHandler(assets domain.MediaRepository) ListAssetsHandler {
	if assets == nil {
		panic("nil media repository")
	}
	return ListAssetsHandler{assets: assets}
}

// Handle returns the tenant's media assets, newest first.
func (h ListAssetsHandler) Handle(ctx context.Context, q ListAssets) ([]AssetView, error) {
	rows, err := h.assets.List(ctx, q.TenantID)
	if err != nil {
		return nil, err
	}
	out := make([]AssetView, 0, len(rows))
	for _, a := range rows {
		out = append(out, viewFor(a))
	}
	return out, nil
}
