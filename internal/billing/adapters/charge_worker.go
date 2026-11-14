package adapters

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/billing/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// ChargeWorker is the job worker for billing.charge. It is a thin driving
// adapter over the shared ChargeInvoice use case — a declined or errored charge
// is handled inside the command (dunning), so only an infrastructure failure
// surfaces as a job error and is retried.
type ChargeWorker struct {
	handler command.ChargeInvoiceHandler
}

// NewChargeWorker builds the billing.charge worker over a ChargeInvoice handler.
func NewChargeWorker(handler command.ChargeInvoiceHandler) *ChargeWorker {
	return &ChargeWorker{handler: handler}
}

// Work runs one billing.charge job.
func (w *ChargeWorker) Work(ctx context.Context, args jobs.BillingChargeArgs) error {
	_, err := w.handler.Handle(ctx, command.ChargeInvoice{
		TenantID:       args.TenantID,
		SubscriptionID: args.SubscriptionID,
	})
	return err
}
