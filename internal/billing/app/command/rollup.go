package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// RollupUsage is the request to aggregate one tenant's raw usage events into
// period counters.
type RollupUsage struct {
	TenantID string
}

// RollupUsageHandler handles RollupUsage. The rollup is idempotent — it reads
// and stamps only the not-yet-rolled events in a single transaction, so a
// re-run counts nothing twice.
type RollupUsageHandler struct {
	usage         domain.UsageRepository
	subscriptions domain.SubscriptionRepository
	plans         domain.PlanRepository
}

// NewRollupUsageHandler builds the handler, failing fast on a nil dependency.
func NewRollupUsageHandler(usage domain.UsageRepository,
	subscriptions domain.SubscriptionRepository, plans domain.PlanRepository) RollupUsageHandler {
	if usage == nil || subscriptions == nil || plans == nil {
		panic("nil dependency")
	}
	return RollupUsageHandler{usage: usage, subscriptions: subscriptions, plans: plans}
}

// Handle rolls the tenant's usage events into counters, sizing the included /
// overage split by the tenant's current plan allowance.
func (h RollupUsageHandler) Handle(ctx context.Context, cmd RollupUsage) error {
	allowance := int64(0)
	period := domain.NewBillingPeriod(1, 0)

	sub, found, err := h.subscriptions.Current(ctx, cmd.TenantID)
	if err != nil {
		return err
	}
	if found {
		plan, err := h.plans.Get(ctx, sub.PlanID())
		if err != nil {
			return err
		}
		allowance = plan.IncludedSends()
		period = plan.BillingPeriod()
	}
	return h.usage.Rollup(ctx, cmd.TenantID, allowance, period)
}
