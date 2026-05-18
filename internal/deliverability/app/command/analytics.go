package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// RefreshAnalytics is the request to recompute one tenant's campaign analytics
// summary rows.
type RefreshAnalytics struct {
	TenantID string
}

// RefreshAnalyticsHandler handles RefreshAnalytics: it recomputes every
// campaign_analytics row for the tenant. The recompute is idempotent — a
// re-run produces the same rows.
type RefreshAnalyticsHandler struct {
	analytics domain.AnalyticsRepository
}

// NewRefreshAnalyticsHandler builds the handler, failing fast on a nil
// dependency.
func NewRefreshAnalyticsHandler(analytics domain.AnalyticsRepository) RefreshAnalyticsHandler {
	if analytics == nil {
		panic("nil dependency")
	}
	return RefreshAnalyticsHandler{analytics: analytics}
}

// Handle recomputes the tenant's campaign analytics.
func (h RefreshAnalyticsHandler) Handle(ctx context.Context, cmd RefreshAnalytics) error {
	return h.analytics.Refresh(ctx, cmd.TenantID)
}
