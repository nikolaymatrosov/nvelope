package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// GetSubscriptionPage is the request for one subscription page by its slug —
// the lookup the public subscription form does.
type GetSubscriptionPage struct {
	TenantID string
	Slug     string
}

// GetSubscriptionPageHandler handles the GetSubscriptionPage query.
type GetSubscriptionPageHandler struct {
	pages domain.SubscriptionPageRepository
}

// NewGetSubscriptionPageHandler builds the handler, failing fast on a nil
// dependency.
func NewGetSubscriptionPageHandler(pages domain.SubscriptionPageRepository) GetSubscriptionPageHandler {
	if pages == nil {
		panic("nil subscription page repository")
	}
	return GetSubscriptionPageHandler{pages: pages}
}

// Handle returns the subscription page, or domain.ErrSubscriptionPageNotFound.
func (h GetSubscriptionPageHandler) Handle(ctx context.Context, q GetSubscriptionPage) (SubscriptionPageView, error) {
	p, err := h.pages.GetBySlug(ctx, q.TenantID, q.Slug)
	if err != nil {
		return SubscriptionPageView{}, err
	}
	return subscriptionPageView(p), nil
}

// ListSubscriptionPages is the request for every subscription page of a tenant.
type ListSubscriptionPages struct {
	TenantID string
}

// ListSubscriptionPagesHandler handles the ListSubscriptionPages query.
type ListSubscriptionPagesHandler struct {
	pages domain.SubscriptionPageRepository
}

// NewListSubscriptionPagesHandler builds the handler, failing fast on a nil
// dependency.
func NewListSubscriptionPagesHandler(pages domain.SubscriptionPageRepository) ListSubscriptionPagesHandler {
	if pages == nil {
		panic("nil subscription page repository")
	}
	return ListSubscriptionPagesHandler{pages: pages}
}

// Handle returns every subscription page of the tenant.
func (h ListSubscriptionPagesHandler) Handle(ctx context.Context, q ListSubscriptionPages) ([]SubscriptionPageView, error) {
	pages, err := h.pages.All(ctx, q.TenantID)
	if err != nil {
		return nil, err
	}
	views := make([]SubscriptionPageView, 0, len(pages))
	for _, p := range pages {
		views = append(views, subscriptionPageView(p))
	}
	return views, nil
}
