// Package query holds the billing context's read-only use cases: the plan
// catalog, the current subscription, and invoices.
package query

import (
	"context"
	"fmt"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// ListPlans is the request for the subscribable plan catalog.
type ListPlans struct{}

// PlanView is one plan shaped for the API.
type PlanView struct {
	ID                string
	Code              string
	Name              string
	PriceMinor        int64
	Currency          string
	BillingPeriod     string
	IncludedSends     int64
	OverageMode       string
	OveragePriceMinor int64
}

// PlansView is the published plan catalog.
type PlansView struct {
	Plans []PlanView
}

// ListPlansHandler handles ListPlans.
type ListPlansHandler struct {
	plans domain.PlanRepository
}

// NewListPlansHandler builds the handler, failing fast on a nil dependency.
func NewListPlansHandler(plans domain.PlanRepository) ListPlansHandler {
	if plans == nil {
		panic("nil dependency")
	}
	return ListPlansHandler{plans: plans}
}

// Handle returns the published plan catalog.
func (h ListPlansHandler) Handle(ctx context.Context, _ ListPlans) (PlansView, error) {
	plans, err := h.plans.ListPublished(ctx)
	if err != nil {
		return PlansView{}, err
	}
	views := make([]PlanView, 0, len(plans))
	for _, p := range plans {
		views = append(views, PlanView{
			ID:                p.ID(),
			Code:              p.Code(),
			Name:              p.Name(),
			PriceMinor:        p.Price().Minor(),
			Currency:          p.Currency(),
			BillingPeriod:     formatBillingPeriod(p.BillingPeriod()),
			IncludedSends:     p.IncludedSends(),
			OverageMode:       string(p.OverageMode()),
			OveragePriceMinor: p.OveragePrice().Minor(),
		})
	}
	return PlansView{Plans: views}, nil
}

// formatBillingPeriod renders a billing period as a human-readable string such
// as "1 month" or "30 days".
func formatBillingPeriod(p domain.BillingPeriod) string {
	switch {
	case p.Months() > 0 && p.Days() == 0:
		return pluralize(p.Months(), "month")
	case p.Days() > 0 && p.Months() == 0:
		return pluralize(p.Days(), "day")
	default:
		return fmt.Sprintf("%s %s", pluralize(p.Months(), "month"), pluralize(p.Days(), "day"))
	}
}

// pluralize renders a count with a noun, adding an "s" for any count but one.
func pluralize(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
