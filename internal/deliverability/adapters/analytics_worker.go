package adapters

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// AnalyticsWorker is the River worker for analytics.refresh. It is a thin
// driving adapter over the RefreshAnalytics use case.
type AnalyticsWorker struct {
	river.WorkerDefaults[jobs.AnalyticsRefreshArgs]
	handler command.RefreshAnalyticsHandler
}

// NewAnalyticsWorker builds the analytics.refresh worker over a RefreshAnalytics
// handler.
func NewAnalyticsWorker(handler command.RefreshAnalyticsHandler) *AnalyticsWorker {
	return &AnalyticsWorker{handler: handler}
}

// Work runs one analytics.refresh job.
func (w *AnalyticsWorker) Work(ctx context.Context, job *river.Job[jobs.AnalyticsRefreshArgs]) error {
	return w.handler.Handle(ctx, command.RefreshAnalytics{TenantID: job.Args.TenantID})
}
