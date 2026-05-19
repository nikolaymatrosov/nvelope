package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// GetSubscription is the request for a tenant's current subscription.
type GetSubscription struct {
	TenantID string
}

// PlanRef is the plan summary embedded in a subscription view.
type PlanRef struct {
	ID          string
	Code        string
	Name        string
	OverageMode string
}

// UsageView is the current-period usage block of a subscription view.
type UsageView struct {
	IncludedSends  int64
	UsedSends      int64
	OverageSends   int64
	RemainingSends int64
}

// SubscriptionView is a tenant's subscription with its current-period usage.
type SubscriptionView struct {
	ID                 string
	Plan               PlanRef
	State              string
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CancelAtPeriodEnd  bool
	Usage              UsageView
}

// UsageReader reports a tenant's metered usage for the current period. It is
// declared here, by the consuming query, and implemented by a billing adapter.
type UsageReader interface {
	// CurrentUsage returns the campaign+transactional send count attributed to
	// the period starting at periodStart — the rolled-up counter total plus the
	// not-yet-rolled usage_events tail.
	CurrentUsage(ctx context.Context, tenantID string, periodStart time.Time) (int64, error)
}

// GetSubscriptionHandler handles GetSubscription.
type GetSubscriptionHandler struct {
	subscriptions domain.SubscriptionRepository
	plans         domain.PlanRepository
	usage         UsageReader
}

// NewGetSubscriptionHandler builds the handler, failing fast on a nil
// dependency. usage may be nil before usage metering is wired (Phase 5 US3),
// in which case the usage block reports zero consumption.
func NewGetSubscriptionHandler(subscriptions domain.SubscriptionRepository,
	plans domain.PlanRepository, usage UsageReader) GetSubscriptionHandler {
	if subscriptions == nil || plans == nil {
		panic("nil dependency")
	}
	return GetSubscriptionHandler{subscriptions: subscriptions, plans: plans, usage: usage}
}

// Handle returns the tenant's subscription, or domain.ErrNoSubscription.
func (h GetSubscriptionHandler) Handle(ctx context.Context, q GetSubscription) (SubscriptionView, error) {
	sub, found, err := h.subscriptions.Current(ctx, q.TenantID)
	if err != nil {
		return SubscriptionView{}, err
	}
	if !found {
		return SubscriptionView{}, domain.ErrNoSubscription
	}
	plan, err := h.plans.Get(ctx, sub.PlanID())
	if err != nil {
		return SubscriptionView{}, err
	}

	used := int64(0)
	if h.usage != nil {
		used, err = h.usage.CurrentUsage(ctx, q.TenantID, sub.CurrentPeriodStart())
		if err != nil {
			return SubscriptionView{}, err
		}
	}
	included := plan.IncludedSends()
	overage := int64(0)
	remaining := included - used
	if remaining < 0 {
		overage = -remaining
		remaining = 0
	}

	return SubscriptionView{
		ID: sub.ID(),
		Plan: PlanRef{
			ID:          plan.ID(),
			Code:        plan.Code(),
			Name:        plan.Name(),
			OverageMode: string(plan.OverageMode()),
		},
		State:              string(sub.State()),
		CurrentPeriodStart: sub.CurrentPeriodStart(),
		CurrentPeriodEnd:   sub.CurrentPeriodEnd(),
		CancelAtPeriodEnd:  sub.CancelAtPeriodEnd(),
		Usage: UsageView{
			IncludedSends:  included,
			UsedSends:      used,
			OverageSends:   overage,
			RemainingSends: remaining,
		},
	}, nil
}
