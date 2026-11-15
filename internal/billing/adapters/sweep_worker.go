package adapters

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/billing/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// SweepWorker is the job worker for billing.sweep. It is a thin driving
// adapter over the RunBillingSweep use case.
type SweepWorker struct {
	handler command.RunBillingSweepHandler
}

// NewSweepWorker builds the billing.sweep worker over a RunBillingSweep handler.
func NewSweepWorker(handler command.RunBillingSweepHandler) *SweepWorker {
	return &SweepWorker{handler: handler}
}

// Work runs one billing.sweep job.
func (w *SweepWorker) Work(ctx context.Context, _ jobs.BillingSweepArgs) error {
	return w.handler.Handle(ctx, command.RunBillingSweep{})
}
