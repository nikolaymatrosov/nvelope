package domain

import (
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// SubscriptionStatus is a subscriber's confirmation status within one list.
type SubscriptionStatus string

const (
	// SubscriptionUnconfirmed is a new membership awaiting confirmation.
	SubscriptionUnconfirmed SubscriptionStatus = "unconfirmed"
	// SubscriptionConfirmed is a confirmed membership.
	SubscriptionConfirmed SubscriptionStatus = "confirmed"
	// SubscriptionUnsubscribed is a membership the subscriber has left.
	SubscriptionUnsubscribed SubscriptionStatus = "unsubscribed"
)

// Membership is the link between a subscriber and a list. A new membership
// starts unconfirmed.
type Membership struct {
	tenantID     string
	subscriberID string
	listID       string
	status       SubscriptionStatus
	createdAt    time.Time
	updatedAt    time.Time
}

// NewMembership builds a membership in the unconfirmed status.
func NewMembership(tenantID, subscriberID, listID string) (*Membership, error) {
	if tenantID == "" || subscriberID == "" || listID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed",
			"tenant, subscriber, and list are required")
	}
	return &Membership{
		tenantID:     tenantID,
		subscriberID: subscriberID,
		listID:       listID,
		status:       SubscriptionUnconfirmed,
	}, nil
}

// HydrateMembership reconstructs a membership from a persisted row.
// Persistence only — it performs no validation.
func HydrateMembership(tenantID, subscriberID, listID string, status SubscriptionStatus,
	createdAt, updatedAt time.Time) *Membership {
	return &Membership{
		tenantID:     tenantID,
		subscriberID: subscriberID,
		listID:       listID,
		status:       status,
		createdAt:    createdAt,
		updatedAt:    updatedAt,
	}
}

// TenantID returns the owning tenant's id.
func (m *Membership) TenantID() string { return m.tenantID }

// SubscriberID returns the linked subscriber's id.
func (m *Membership) SubscriberID() string { return m.subscriberID }

// ListID returns the linked list's id.
func (m *Membership) ListID() string { return m.listID }

// Status returns the subscription status.
func (m *Membership) Status() SubscriptionStatus { return m.status }

// CreatedAt returns when the membership was created.
func (m *Membership) CreatedAt() time.Time { return m.createdAt }

// UpdatedAt returns when the membership was last changed.
func (m *Membership) UpdatedAt() time.Time { return m.updatedAt }

// Confirm moves the membership to confirmed. Allowed from unconfirmed only.
func (m *Membership) Confirm() error {
	if m.status != SubscriptionUnconfirmed {
		return apperr.NewIncorrectInput("invalid_transition",
			"only an unconfirmed membership can be confirmed")
	}
	m.status = SubscriptionConfirmed
	return nil
}

// Unsubscribe moves the membership to unsubscribed. Allowed from unconfirmed
// or confirmed.
func (m *Membership) Unsubscribe() error {
	if m.status == SubscriptionUnsubscribed {
		return apperr.NewIncorrectInput("invalid_transition",
			"that subscriber is already unsubscribed from the list")
	}
	m.status = SubscriptionUnsubscribed
	return nil
}

// Resubscribe moves an unsubscribed membership back to confirmed.
func (m *Membership) Resubscribe() error {
	if m.status != SubscriptionUnsubscribed {
		return apperr.NewIncorrectInput("invalid_transition",
			"only an unsubscribed membership can be resubscribed")
	}
	m.status = SubscriptionConfirmed
	return nil
}

// ChangeStatus applies a target subscription status through the state
// machine, choosing the correct transition so invariants are enforced.
func (m *Membership) ChangeStatus(target SubscriptionStatus) error {
	if target == m.status {
		return nil
	}
	switch target {
	case SubscriptionConfirmed:
		if m.status == SubscriptionUnsubscribed {
			return m.Resubscribe()
		}
		return m.Confirm()
	case SubscriptionUnsubscribed:
		return m.Unsubscribe()
	case SubscriptionUnconfirmed:
		return apperr.NewIncorrectInput("invalid_transition",
			"a membership cannot return to unconfirmed")
	default:
		return apperr.NewIncorrectInput("validation_failed", "unknown subscription status")
	}
}

// ValidSubscriptionStatus reports whether v is a known subscription status.
func ValidSubscriptionStatus(v SubscriptionStatus) bool {
	return v == SubscriptionUnconfirmed || v == SubscriptionConfirmed || v == SubscriptionUnsubscribed
}
