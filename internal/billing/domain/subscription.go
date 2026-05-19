package domain

import "time"

// SubscriptionState is the lifecycle state of a tenant's subscription.
type SubscriptionState string

const (
	// SubscriptionPending is a freshly created subscription whose first charge
	// has not yet succeeded.
	SubscriptionPending SubscriptionState = "pending"
	// SubscriptionActive is a paid-up subscription; sending is allowed.
	SubscriptionActive SubscriptionState = "active"
	// SubscriptionPastDue is a subscription with a failed charge in the dunning
	// grace window; sending is still allowed.
	SubscriptionPastDue SubscriptionState = "past_due"
	// SubscriptionSuspended is a subscription whose dunning retries were
	// exhausted; sending is blocked until the balance is settled.
	SubscriptionSuspended SubscriptionState = "suspended"
	// SubscriptionCanceled is a terminated subscription.
	SubscriptionCanceled SubscriptionState = "canceled"
)

// Subscription is the billing relationship between a tenant and a plan — a
// tenant-plane aggregate root carrying an explicit lifecycle state machine. It
// is reached only through the RLS-bound transaction owned by its repository.
type Subscription struct {
	id                 string
	tenantID           string
	planID             string
	state              SubscriptionState
	currentPeriodStart time.Time
	currentPeriodEnd   time.Time
	cancelAtPeriodEnd  bool
	canceledAt         *time.Time
}

// NewSubscription builds a pending subscription for the given plan and initial
// billing period.
func NewSubscription(tenantID, planID string, periodStart, periodEnd time.Time) (*Subscription, error) {
	if tenantID == "" || planID == "" {
		return nil, ErrInvalidPlan.WithMessage("a subscription needs a tenant and a plan")
	}
	if !periodEnd.After(periodStart) {
		return nil, ErrInvalidPlan.WithMessage("the billing period end must be after its start")
	}
	return &Subscription{
		tenantID:           tenantID,
		planID:             planID,
		state:              SubscriptionPending,
		currentPeriodStart: periodStart.UTC(),
		currentPeriodEnd:   periodEnd.UTC(),
	}, nil
}

// HydrateSubscription reconstructs a subscription from a persisted row.
// Persistence only — it performs no validation and is not a constructor.
func HydrateSubscription(id, tenantID, planID string, state SubscriptionState,
	periodStart, periodEnd time.Time, cancelAtPeriodEnd bool, canceledAt *time.Time) *Subscription {

	return &Subscription{
		id: id, tenantID: tenantID, planID: planID, state: state,
		currentPeriodStart: periodStart, currentPeriodEnd: periodEnd,
		cancelAtPeriodEnd: cancelAtPeriodEnd, canceledAt: canceledAt,
	}
}

// ID returns the database-assigned id.
func (s *Subscription) ID() string { return s.id }

// TenantID returns the owning tenant's id.
func (s *Subscription) TenantID() string { return s.tenantID }

// PlanID returns the subscribed plan's id.
func (s *Subscription) PlanID() string { return s.planID }

// State returns the lifecycle state.
func (s *Subscription) State() SubscriptionState { return s.state }

// CurrentPeriodStart returns the start of the current billing period.
func (s *Subscription) CurrentPeriodStart() time.Time { return s.currentPeriodStart }

// CurrentPeriodEnd returns the end of the current billing period.
func (s *Subscription) CurrentPeriodEnd() time.Time { return s.currentPeriodEnd }

// CancelAtPeriodEnd reports whether a cancellation is pending for the end of
// the current period.
func (s *Subscription) CancelAtPeriodEnd() bool { return s.cancelAtPeriodEnd }

// CanceledAt returns when the subscription was canceled, or nil.
func (s *Subscription) CanceledAt() *time.Time { return s.canceledAt }

// IsActive reports whether the subscription is in the active state.
func (s *Subscription) IsActive() bool { return s.state == SubscriptionActive }

// IsCanceled reports whether the subscription has been terminated.
func (s *Subscription) IsCanceled() bool { return s.state == SubscriptionCanceled }

// AllowsSending reports whether the subscription state permits metered sends.
// A past_due subscription is in the dunning grace window and may still send; a
// suspended, canceled, or pending one may not (research R8).
func (s *Subscription) AllowsSending() bool {
	return s.state == SubscriptionActive || s.state == SubscriptionPastDue
}

// Activate transitions the subscription to active on a successful charge —
// from pending (first charge), past_due (a dunning retry), or suspended (the
// balance settled). It rejects an illegal move with ErrInvalidSubscriptionTransition.
func (s *Subscription) Activate() error {
	switch s.state {
	case SubscriptionPending, SubscriptionActive, SubscriptionPastDue, SubscriptionSuspended:
		s.state = SubscriptionActive
		return nil
	default:
		return ErrInvalidSubscriptionTransition.WithMessage(
			"a " + string(s.state) + " subscription cannot be activated")
	}
}

// SetPeriod moves the current billing period window. It is used after a
// successful renewal charge to advance the subscription into the next period.
func (s *Subscription) SetPeriod(start, end time.Time) {
	s.currentPeriodStart = start.UTC()
	s.currentPeriodEnd = end.UTC()
}

// MarkPastDue transitions the subscription to past_due on a failed charge —
// from pending (a failed first charge) or active (a failed renewal). Calling it
// on an already past_due subscription is a no-op.
func (s *Subscription) MarkPastDue() error {
	switch s.state {
	case SubscriptionPending, SubscriptionActive, SubscriptionPastDue:
		s.state = SubscriptionPastDue
		return nil
	default:
		return ErrInvalidSubscriptionTransition.WithMessage(
			"a " + string(s.state) + " subscription cannot be marked past due")
	}
}

// Suspend transitions a past_due subscription to suspended once its dunning
// retries are exhausted.
func (s *Subscription) Suspend() error {
	if s.state != SubscriptionPastDue {
		return ErrInvalidSubscriptionTransition.WithMessage(
			"only a past_due subscription can be suspended")
	}
	s.state = SubscriptionSuspended
	return nil
}

// RequestCancellation flags the subscription to cancel at the end of the
// current period. The subscription stays in its current state until the period
// elapses.
func (s *Subscription) RequestCancellation() error {
	switch s.state {
	case SubscriptionActive, SubscriptionPastDue:
		s.cancelAtPeriodEnd = true
		return nil
	default:
		return ErrInvalidSubscriptionTransition.WithMessage(
			"a " + string(s.state) + " subscription cannot be canceled")
	}
}

// Cancel terminates the subscription. It is the terminal transition, applied by
// the sweep once a cancellation pending for period end has elapsed.
func (s *Subscription) Cancel(at time.Time) error {
	if s.state == SubscriptionCanceled {
		return ErrInvalidSubscriptionTransition.WithMessage(
			"the subscription is already canceled")
	}
	s.state = SubscriptionCanceled
	at = at.UTC()
	s.canceledAt = &at
	return nil
}
