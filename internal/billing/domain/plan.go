package domain

import (
	"strings"
	"time"
)

// OverageMode decides what happens to a send past the plan's included
// allowance: block rejects it, meter accepts it and bills the excess.
type OverageMode string

const (
	// OverageBlock rejects a send once the included allowance is exhausted.
	OverageBlock OverageMode = "block"
	// OverageMeter accepts an over-allowance send and bills it as overage.
	OverageMeter OverageMode = "meter"
)

// PlanStatus is the catalog lifecycle of a plan.
type PlanStatus string

const (
	// PlanDraft is an unpublished plan, not yet subscribable.
	PlanDraft PlanStatus = "draft"
	// PlanPublished is a plan available for subscription.
	PlanPublished PlanStatus = "published"
	// PlanArchived is a withdrawn plan, no longer subscribable.
	PlanArchived PlanStatus = "archived"
)

// BillingPeriod is the recurrence of a plan's fee, expressed in calendar months
// and days so a "1 month" period advances by a real calendar month rather than
// a fixed span of hours.
type BillingPeriod struct {
	months int
	days   int
}

// NewBillingPeriod builds a billing period of the given calendar months and
// days.
func NewBillingPeriod(months, days int) BillingPeriod {
	return BillingPeriod{months: months, days: days}
}

// Months returns the calendar-month component of the period.
func (p BillingPeriod) Months() int { return p.months }

// Days returns the day component of the period.
func (p BillingPeriod) Days() int { return p.days }

// IsZero reports whether the period is empty.
func (p BillingPeriod) IsZero() bool { return p.months == 0 && p.days == 0 }

// AdvanceFrom returns t moved forward by one billing period.
func (p BillingPeriod) AdvanceFrom(t time.Time) time.Time {
	return t.AddDate(0, p.months, p.days).UTC()
}

// Plan is a purchasable offering in the platform catalog. It is control-plane
// data — the same catalog for every tenant — so it carries no tenant id and is
// not reached through an RLS-bound transaction.
type Plan struct {
	id            string
	code          string
	name          string
	price         Money
	billingPeriod BillingPeriod
	includedSends int64
	overageMode   OverageMode
	overagePrice  Money
	status        PlanStatus
}

// NewPlan builds a plan, rejecting any invariant violation.
func NewPlan(code, name string, price Money, period BillingPeriod, includedSends int64,
	overageMode OverageMode, overagePrice Money, status PlanStatus) (*Plan, error) {

	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	if code == "" {
		return nil, ErrInvalidPlan.WithMessage("a plan code is required")
	}
	if name == "" {
		return nil, ErrInvalidPlan.WithMessage("a plan name is required")
	}
	if price.Currency() != "RUB" {
		return nil, ErrInvalidPlan.WithMessage("Phase 5 plans must be priced in RUB")
	}
	if price.Minor() < 0 {
		return nil, ErrInvalidPlan.WithMessage("a plan price cannot be negative")
	}
	if period.IsZero() {
		return nil, ErrInvalidPlan.WithMessage("a plan needs a non-zero billing period")
	}
	if includedSends < 0 {
		return nil, ErrInvalidPlan.WithMessage("included sends cannot be negative")
	}
	if overageMode != OverageBlock && overageMode != OverageMeter {
		return nil, ErrInvalidPlan.WithMessage("unknown overage mode")
	}
	if overagePrice.Currency() != price.Currency() {
		return nil, ErrInvalidPlan.WithMessage("overage price must match the plan currency")
	}
	switch status {
	case PlanDraft, PlanPublished, PlanArchived:
	default:
		return nil, ErrInvalidPlan.WithMessage("unknown plan status")
	}
	return &Plan{
		code: code, name: name, price: price, billingPeriod: period,
		includedSends: includedSends, overageMode: overageMode,
		overagePrice: overagePrice, status: status,
	}, nil
}

// HydratePlan reconstructs a plan from a persisted row. Persistence only — it
// performs no validation and is not a constructor.
func HydratePlan(id, code, name string, price Money, period BillingPeriod,
	includedSends int64, overageMode OverageMode, overagePrice Money,
	status PlanStatus) *Plan {

	return &Plan{
		id: id, code: code, name: name, price: price, billingPeriod: period,
		includedSends: includedSends, overageMode: overageMode,
		overagePrice: overagePrice, status: status,
	}
}

// ID returns the database-assigned id.
func (p *Plan) ID() string { return p.id }

// Code returns the stable machine code.
func (p *Plan) Code() string { return p.code }

// Name returns the display name.
func (p *Plan) Name() string { return p.name }

// Price returns the recurring fee.
func (p *Plan) Price() Money { return p.price }

// Currency returns the plan's currency code.
func (p *Plan) Currency() string { return p.price.Currency() }

// BillingPeriod returns the plan's recurrence.
func (p *Plan) BillingPeriod() BillingPeriod { return p.billingPeriod }

// IncludedSends returns the sends covered by the base fee.
func (p *Plan) IncludedSends() int64 { return p.includedSends }

// OverageMode returns how the plan treats over-allowance sends.
func (p *Plan) OverageMode() OverageMode { return p.overageMode }

// OveragePrice returns the per-send price past the allowance.
func (p *Plan) OveragePrice() Money { return p.overagePrice }

// Status returns the catalog lifecycle state.
func (p *Plan) Status() PlanStatus { return p.status }

// IsSubscribable reports whether a tenant may subscribe to this plan.
func (p *Plan) IsSubscribable() bool { return p.status == PlanPublished }
