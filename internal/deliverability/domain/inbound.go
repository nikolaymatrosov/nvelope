package domain

import (
	"strings"
	"time"
)

// InboundStatus is the lifecycle state of a staged inbound notification.
type InboundStatus string

const (
	// InboundPending marks a notification staged but not yet processed.
	InboundPending InboundStatus = "pending"
	// InboundAttributed marks a notification whose delivery event was recorded
	// and, for a bounce or complaint, suppression applied.
	InboundAttributed InboundStatus = "attributed"
	// InboundUnattributed marks a notification whose provider message id
	// matched no send.
	InboundUnattributed InboundStatus = "unattributed"
	// InboundFailed marks a notification whose processing failed terminally.
	InboundFailed InboundStatus = "failed"
)

// InboundNotification is a parsed delivery-feedback notification before
// attribution. It is constructed by cmd/consumer from a stream record and
// staged for asynchronous processing; the feedback.process worker reloads it
// by id.
type InboundNotification struct {
	// ID is the staged inbound_feedback_events row id, empty before staging.
	ID string
	// DedupeKey is the stable idempotency key — the provider eventId.
	DedupeKey string
	Kind      EventKind
	// ProviderMessageID is the mail.messageId of the originating send.
	ProviderMessageID string
	RecipientEmail    string
	OccurredAt        time.Time
	// RawPayload is the raw notification body, retained for audit.
	RawPayload []byte
	// Status is the staged row's lifecycle state; InboundPending for a freshly
	// constructed notification.
	Status InboundStatus
}

// NewInboundNotification builds a notification from a parsed stream record,
// rejecting an unknown kind or missing identifiers.
func NewInboundNotification(dedupeKey string, kind EventKind,
	providerMessageID, recipientEmail string, occurredAt time.Time,
	rawPayload []byte) (InboundNotification, error) {

	if dedupeKey == "" {
		return InboundNotification{}, ErrValidationFailed.WithMessage("a dedupe key is required")
	}
	if !validKind(kind) {
		return InboundNotification{}, ErrValidationFailed.WithMessage("unknown notification kind")
	}
	if providerMessageID == "" {
		return InboundNotification{}, ErrValidationFailed.WithMessage("a provider message id is required")
	}
	recipientEmail = strings.ToLower(strings.TrimSpace(recipientEmail))
	if recipientEmail == "" {
		return InboundNotification{}, ErrValidationFailed.WithMessage("a recipient email is required")
	}
	return InboundNotification{
		DedupeKey:         dedupeKey,
		Kind:              kind,
		ProviderMessageID: providerMessageID,
		RecipientEmail:    recipientEmail,
		OccurredAt:        occurredAt.UTC(),
		RawPayload:        rawPayload,
		Status:            InboundPending,
	}, nil
}

// IsProcessed reports whether the staged notification has already reached a
// terminal status — the worker no-ops on a redelivered or retried job.
func (n InboundNotification) IsProcessed() bool {
	return n.Status == InboundAttributed || n.Status == InboundUnattributed
}
