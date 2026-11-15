package adapters

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// AnalyticsWorker is the job worker for analytics.refresh. It is a thin
// driving adapter over the RefreshAnalytics use case.
type AnalyticsWorker struct {
	handler command.RefreshAnalyticsHandler
}

// NewAnalyticsWorker builds the analytics.refresh worker over a RefreshAnalytics
// handler.
func NewAnalyticsWorker(handler command.RefreshAnalyticsHandler) *AnalyticsWorker {
	return &AnalyticsWorker{handler: handler}
}

// Work runs one analytics.refresh job.
func (w *AnalyticsWorker) Work(ctx context.Context, args jobs.AnalyticsRefreshArgs) error {
	return w.handler.Handle(ctx, command.RefreshAnalytics{TenantID: args.TenantID})
}
