package domain

import (
	"strings"
	"time"
)

// RecipientStatus is the per-recipient send state.
type RecipientStatus string

const (
	// RecipientPending marks a recipient not yet sent to.
	RecipientPending RecipientStatus = "pending"
	// RecipientSent marks a recipient successfully sent to.
	RecipientSent RecipientStatus = "sent"
	// RecipientFailed marks a recipient whose send failed.
	RecipientFailed RecipientStatus = "failed"
)

// Recipient is one unique target of a campaign — the unit of send progress and
// the database-level dedup guarantee. Its id also serves as the per-recipient
// tracking token in rewritten links and the open pixel.
type Recipient struct {
	id            string
	tenantID      string
	campaignID    string
	subscriberID  string
	email         string
	status        RecipientStatus
	failureReason string
	sentAt        *time.Time
}

// NewRecipient builds a pending recipient.
func NewRecipient(tenantID, campaignID, subscriberID, email string) *Recipient {
	return &Recipient{
		tenantID: tenantID, campaignID: campaignID, subscriberID: subscriberID,
		email: strings.ToLower(strings.TrimSpace(email)), status: RecipientPending,
	}
}

// HydrateRecipient reconstructs a recipient from a persisted row.
func HydrateRecipient(id, tenantID, campaignID, subscriberID, email string,
	status RecipientStatus, failureReason string, sentAt *time.Time) *Recipient {
	return &Recipient{
		id: id, tenantID: tenantID, campaignID: campaignID, subscriberID: subscriberID,
		email: email, status: status, failureReason: failureReason, sentAt: sentAt,
	}
}

// ID returns the recipient id — also its tracking token.
func (r *Recipient) ID() string { return r.id }

// TenantID returns the owning tenant's id.
func (r *Recipient) TenantID() string { return r.tenantID }

// CampaignID returns the campaign this recipient belongs to.
func (r *Recipient) CampaignID() string { return r.campaignID }

// SubscriberID returns the underlying subscriber's id.
func (r *Recipient) SubscriberID() string { return r.subscriberID }

// Email returns the recipient address snapshot.
func (r *Recipient) Email() string { return r.email }

// Status returns the per-recipient send status.
func (r *Recipient) Status() RecipientStatus { return r.status }

// FailureReason returns why a failed recipient's send failed.
func (r *Recipient) FailureReason() string { return r.failureReason }

// SentAt returns when the recipient was sent to, or nil.
func (r *Recipient) SentAt() *time.Time { return r.sentAt }

// MarkSent records a successful send.
func (r *Recipient) MarkSent(at time.Time) {
	r.status = RecipientSent
	at = at.UTC()
	r.sentAt = &at
	r.failureReason = ""
}

// MarkFailed records a failed send with an actionable reason.
func (r *Recipient) MarkFailed(reason string) {
	r.status = RecipientFailed
	r.failureReason = reason
}
