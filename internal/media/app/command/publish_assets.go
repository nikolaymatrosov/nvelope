package command

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// PublishAssets is the request to make the library assets that texts
// reference publicly reachable and to rewrite their library URLs to the
// public copies. Callers run it when content goes live: a campaign start, a
// transactional template save, a branding save.
type PublishAssets struct {
	TenantID string
	Texts    []string
}

// PublishAssetsResult carries the rewritten texts, in input order.
type PublishAssetsResult struct {
	Texts []string
}

// PublishAssetsHandler handles the PublishAssets command.
type PublishAssetsHandler struct {
	assets domain.MediaRepository
	blobs  domain.BlobStore
	urls   domain.LibraryURLs
}

// NewPublishAssetsHandler builds the handler, failing fast on a nil
// dependency. baseURL is the app origin library URLs use.
func NewPublishAssetsHandler(assets domain.MediaRepository, blobs domain.BlobStore,
	baseURL string) PublishAssetsHandler {
	if assets == nil || blobs == nil {
		panic("nil dependency")
	}
	return PublishAssetsHandler{assets: assets, blobs: blobs, urls: domain.NewLibraryURLs(baseURL)}
}

// Handle resolves every referenced asset under RLS first and fails with
// ErrMediaMissing, copying nothing, if any is gone. Then it copies each
// not-yet-published asset to its public key, marks it published, and
// rewrites the texts.
func (h PublishAssetsHandler) Handle(ctx context.Context, cmd PublishAssets) (PublishAssetsResult, error) {
	ids := h.urls.AssetIDs(cmd.Texts...)
	if len(ids) == 0 {
		return PublishAssetsResult{Texts: cmd.Texts}, nil
	}
	assets := make(map[string]*domain.MediaAsset, len(ids))
	var missing []string
	for _, id := range ids {
		a, err := h.assets.Get(ctx, cmd.TenantID, id)
		if errors.Is(err, domain.ErrMediaNotFound) {
			missing = append(missing, id)
			continue
		}
		if err != nil {
			return PublishAssetsResult{}, err
		}
		assets[id] = a
	}
	if len(missing) > 0 {
		return PublishAssetsResult{}, domain.ErrMediaMissing.WithMessage(
			"these media assets were deleted from the library: " + strings.Join(missing, ", "))
	}

	for _, id := range ids {
		a := assets[id]
		if a.IsPublished() {
			continue
		}
		if err := h.blobs.Publish(ctx, a.StorageKey(), h.publishedKey(a),
			a.ContentType(), a.Disposition()); err != nil {
			return PublishAssetsResult{}, err
		}
		if err := h.assets.MarkPublished(ctx, cmd.TenantID, id, time.Now()); err != nil {
			return PublishAssetsResult{}, err
		}
	}

	out := make([]string, len(cmd.Texts))
	for i, t := range cmd.Texts {
		out[i] = h.urls.Replace(t, func(id string) string {
			return h.blobs.PublicURL(h.publishedKey(assets[id]))
		})
	}
	return PublishAssetsResult{Texts: out}, nil
}

func (h PublishAssetsHandler) publishedKey(a *domain.MediaAsset) string {
	return h.blobs.PublishedKey(a.TenantID(), a.ID(), a.Filename())
}
