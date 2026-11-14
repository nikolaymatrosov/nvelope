package adapters

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/billing/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// RollupWorker is the job worker for usage.rollup. It is a thin driving
// adapter over the RollupUsage use case.
type RollupWorker struct {
	handler command.RollupUsageHandler
}

// NewRollupWorker builds the usage.rollup worker over a RollupUsage handler.
func NewRollupWorker(handler command.RollupUsageHandler) *RollupWorker {
	return &RollupWorker{handler: handler}
}

// Work runs one usage.rollup job.
func (w *RollupWorker) Work(ctx context.Context, args jobs.UsageRollupArgs) error {
	return w.handler.Handle(ctx, command.RollupUsage{TenantID: args.TenantID})
}
