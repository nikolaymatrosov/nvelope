package domain

import (
	"strings"
	"time"
)

// EventKind is the kind of an inbound delivery-feedback event. Postbox writes
// several notification types to the feedback stream; Phase 4 records these
// five and reads past the rest (Send, DeliveryDelay, Unsubscribe).
type EventKind string

const (
	// KindBounce marks a permanent delivery failure. Every Postbox bounce is
	// permanent (hard) — there is no soft-bounce classification.
	KindBounce EventKind = "bounce"
	// KindComplaint marks a recipient marking the message as spam.
	KindComplaint EventKind = "complaint"
	// KindDelivery marks a message the recipient's mail server accepted.
	KindDelivery EventKind = "delivery"
	// KindOpen marks a recipient opening the message.
	KindOpen EventKind = "open"
	// KindClick marks a recipient clicking a link in the message.
	KindClick EventKind = "click"
)

// validKind reports whether k is a recognised event kind.
func validKind(k EventKind) bool {
	switch k {
	case KindBounce, KindComplaint, KindDelivery, KindOpen, KindClick:
		return true
	default:
		return false
	}
}

// SuppressionReason is why an address was added to the suppression list.
type SuppressionReason string

const (
	// ReasonHardBounce marks an address suppressed after a hard bounce.
	ReasonHardBounce SuppressionReason = "hard_bounce"
	// ReasonComplaint marks an address suppressed after a spam complaint.
	ReasonComplaint SuppressionReason = "complaint"
	// ReasonManual marks an address suppressed by an operator.
	ReasonManual SuppressionReason = "manual"
)

// Attribution ties a delivery event to the originating send. Exactly one of
// CampaignRecipientID / TransactionalMessageID is set; CampaignID accompanies a
// campaign attribution.
type Attribution struct {
	CampaignID             string
	CampaignRecipientID    string
	TransactionalMessageID string
}

// DeliveryEvent is an attributed feedback event — a bounce, complaint,
// delivery, open, or click. It is a tenant-plane aggregate reached only
// through its repository's RLS-bound transaction. Fields are unexported;
// construction validates every invariant.
type DeliveryEvent struct {
	id                     string
	tenantID               string
	inboundEventID         string
	kind                   EventKind
	recipientEmail         string
	campaignID             string
	campaignRecipientID    string
	transactionalMessageID string
	providerMessageID      string
	occurredAt             time.Time
	createdAt              time.Time
}

// NewDeliveryEvent builds an attributed delivery event, rejecting an unknown
// kind or an attribution naming neither a campaign recipient nor a
// transactional message (or naming both).
func NewDeliveryEvent(tenantID, inboundEventID string, kind EventKind,
	recipientEmail, providerMessageID string, occurredAt time.Time,
	attr Attribution) (*DeliveryEvent, error) {

	if tenantID == "" || inboundEventID == "" {
		return nil, ErrValidationFailed.WithMessage("a tenant and an inbound event are required")
	}
	if !validKind(kind) {
		return nil, ErrValidationFailed.WithMessage("unknown delivery event kind")
	}
	recipientEmail = strings.ToLower(strings.TrimSpace(recipientEmail))
	if recipientEmail == "" {
		return nil, ErrValidationFailed.WithMessage("a recipient email is required")
	}
	if providerMessageID == "" {
		return nil, ErrValidationFailed.WithMessage("a provider message id is required")
	}
	hasCampaign := attr.CampaignRecipientID != ""
	hasTransactional := attr.TransactionalMessageID != ""
	if hasCampaign == hasTransactional {
		return nil, ErrValidationFailed.WithMessage(
			"a delivery event must be attributed to exactly one of a campaign recipient or a transactional message")
	}
	return &DeliveryEvent{
		tenantID:               tenantID,
		inboundEventID:         inboundEventID,
		kind:                   kind,
		recipientEmail:         recipientEmail,
		campaignID:             attr.CampaignID,
		campaignRecipientID:    attr.CampaignRecipientID,
		transactionalMessageID: attr.TransactionalMessageID,
		providerMessageID:      providerMessageID,
		occurredAt:             occurredAt.UTC(),
	}, nil
}

// HydrateDeliveryEvent reconstructs a delivery event from a persisted row.
// Persistence only — it performs no validation and is not a constructor.
func HydrateDeliveryEvent(id, tenantID, inboundEventID string, kind EventKind,
	recipientEmail, campaignID, campaignRecipientID, transactionalMessageID,
	providerMessageID string, occurredAt, createdAt time.Time) *DeliveryEvent {

	return &DeliveryEvent{
		id: id, tenantID: tenantID, inboundEventID: inboundEventID,
		kind: kind, recipientEmail: recipientEmail, campaignID: campaignID,
		campaignRecipientID: campaignRecipientID, transactionalMessageID: transactionalMessageID,
		providerMessageID: providerMessageID, occurredAt: occurredAt, createdAt: createdAt,
	}
}

// ID returns the database-assigned id.
func (e *DeliveryEvent) ID() string { return e.id }

// TenantID returns the owning tenant's id.
func (e *DeliveryEvent) TenantID() string { return e.tenantID }

// InboundEventID returns the id of the staged inbound notification.
func (e *DeliveryEvent) InboundEventID() string { return e.inboundEventID }

// Kind returns the event kind.
func (e *DeliveryEvent) Kind() EventKind { return e.kind }

// RecipientEmail returns the lower-cased recipient address.
func (e *DeliveryEvent) RecipientEmail() string { return e.recipientEmail }

// CampaignID returns the originating campaign's id, or "".
func (e *DeliveryEvent) CampaignID() string { return e.campaignID }

// CampaignRecipientID returns the originating campaign-recipient id, or "".
func (e *DeliveryEvent) CampaignRecipientID() string { return e.campaignRecipientID }

// TransactionalMessageID returns the originating transactional-message id, or "".
func (e *DeliveryEvent) TransactionalMessageID() string { return e.transactionalMessageID }

// ProviderMessageID returns the provider message id of the originating send.
func (e *DeliveryEvent) ProviderMessageID() string { return e.providerMessageID }

// OccurredAt returns the provider-reported event time.
func (e *DeliveryEvent) OccurredAt() time.Time { return e.occurredAt }

// CreatedAt returns when the event row was written.
func (e *DeliveryEvent) CreatedAt() time.Time { return e.createdAt }

// IsBounce reports whether the event is a bounce.
func (e *DeliveryEvent) IsBounce() bool { return e.kind == KindBounce }

// IsComplaint reports whether the event is a spam complaint.
func (e *DeliveryEvent) IsComplaint() bool { return e.kind == KindComplaint }

// SuppressionReason maps the event to the reason it would suppress the
// recipient. ok is false for a delivery, open, or click — those events record
// a row but never drive suppression.
func (e *DeliveryEvent) SuppressionReason() (reason SuppressionReason, ok bool) {
	switch e.kind {
	case KindBounce:
		return ReasonHardBounce, true
	case KindComplaint:
		return ReasonComplaint, true
	default:
		return "", false
	}
}
