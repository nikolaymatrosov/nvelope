package adapters

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// QuotaGate is the pre-send authorization gate consumed by the campaign send
// paths. It implements the campaign context's consumer-owned QuotaGate port by
// reading the tenant's subscription state and metered usage.
type QuotaGate struct {
	subscriptions domain.SubscriptionRepository
	plans         domain.PlanRepository
	usage         domain.UsageRepository
}

var _ campaigndomain.QuotaGate = (*QuotaGate)(nil)

// NewQuotaGate builds a QuotaGate over the subscription, plan, and usage
// repositories.
func NewQuotaGate(subscriptions domain.SubscriptionRepository, plans domain.PlanRepository,
	usage domain.UsageRepository) *QuotaGate {
	return &QuotaGate{subscriptions: subscriptions, plans: plans, usage: usage}
}

// Authorize decides whether units sends may proceed for the tenant. A suspended
// subscription blocks all sends; an absent, pending, or canceled subscription
// likewise denies. For an active or past_due subscription, a block-mode plan
// denies an over-allowance request while a meter-mode plan always allows it.
func (g *QuotaGate) Authorize(ctx context.Context, tenantID, _ string, units int64) (
	campaigndomain.QuotaDecision, error) {

	sub, found, err := g.subscriptions.Current(ctx, tenantID)
	if err != nil {
		return campaigndomain.QuotaDecision{}, err
	}
	if !found {
		return campaigndomain.QuotaDecision{
			Allowed: false, Reason: campaigndomain.QuotaReasonExceeded,
		}, nil
	}
	switch sub.State() {
	case domain.SubscriptionSuspended:
		return campaigndomain.QuotaDecision{
			Allowed: false, Reason: campaigndomain.QuotaReasonSuspended,
		}, nil
	case domain.SubscriptionActive, domain.SubscriptionPastDue:
		// Sending is permitted in the dunning grace window — fall through.
	default:
		return campaigndomain.QuotaDecision{
			Allowed: false, Reason: campaigndomain.QuotaReasonExceeded,
		}, nil
	}

	plan, err := g.plans.Get(ctx, sub.PlanID())
	if err != nil {
		return campaigndomain.QuotaDecision{}, err
	}
	used, err := g.usage.CurrentUsage(ctx, tenantID, sub.CurrentPeriodStart())
	if err != nil {
		return campaigndomain.QuotaDecision{}, err
	}
	remaining := plan.IncludedSends() - used
	if remaining < 0 {
		remaining = 0
	}

	// A meter-mode plan always proceeds; the excess is billed as overage.
	if plan.OverageMode() == domain.OverageMeter {
		return campaigndomain.QuotaDecision{Allowed: true, RemainingFree: remaining}, nil
	}
	// A block-mode plan denies a request that would cross the allowance.
	if used+units > plan.IncludedSends() {
		return campaigndomain.QuotaDecision{
			Allowed: false, Reason: campaigndomain.QuotaReasonExceeded, RemainingFree: remaining,
		}, nil
	}
	return campaigndomain.QuotaDecision{Allowed: true, RemainingFree: remaining}, nil
}
