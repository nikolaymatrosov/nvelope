package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// ChargeEnqueuer enqueues a billing.charge job for one subscription. It is
// declared here, by the sweep, and implemented by the platform/jobs adapter.
type ChargeEnqueuer interface {
	EnqueueBillingCharge(ctx context.Context, tenantID, subscriptionID string) error
}

// RunBillingSweep is the request to find every subscription due for a charge
// and fan out a billing.charge job per subscription.
type RunBillingSweep struct{}

// RunBillingSweepHandler handles RunBillingSweep. It is pure fan-out — it
// changes no billing state, so a re-run is harmless.
type RunBillingSweepHandler struct {
	due      domain.DueSubscriptionReader
	enqueuer ChargeEnqueuer
}

// NewRunBillingSweepHandler builds the handler, failing fast on a nil
// dependency.
func NewRunBillingSweepHandler(due domain.DueSubscriptionReader, enqueuer ChargeEnqueuer) RunBillingSweepHandler {
	if due == nil || enqueuer == nil {
		panic("nil dependency")
	}
	return RunBillingSweepHandler{due: due, enqueuer: enqueuer}
}

// Handle enqueues one billing.charge job per due subscription, deduplicated by
// subscription id.
func (h RunBillingSweepHandler) Handle(ctx context.Context, _ RunBillingSweep) error {
	due, err := h.due.ListDue(ctx)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(due))
	for _, d := range due {
		if seen[d.SubscriptionID] {
			continue
		}
		seen[d.SubscriptionID] = true
		if err := h.enqueuer.EnqueueBillingCharge(ctx, d.TenantID, d.SubscriptionID); err != nil {
			return err
		}
	}
	return nil
}
