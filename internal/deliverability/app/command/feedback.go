// Package command holds the deliverability context's state-changing use cases:
// ingesting inbound feedback, managing the suppression list and bounce
// settings, and refreshing analytics.
package command

import (
	"context"
	"log/slog"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// NotificationParser decodes a raw stream notification into a domain
// InboundNotification. It is declared here, by the consuming use case, and
// implemented by the adapters layer. recognized is false for event types
// Phase 4 does not act on.
type NotificationParser interface {
	Parse(raw []byte) (n domain.InboundNotification, recognized bool, err error)
}

// FeedbackEnqueuer enqueues asynchronous feedback processing. It is declared
// here, by the consuming use case, and implemented by the platform/jobs
// adapter.
type FeedbackEnqueuer interface {
	EnqueueFeedbackProcess(ctx context.Context, inboundEventID string) error
}

// Suppressor applies suppression for a newly recorded bounce or complaint,
// honouring the tenant's bounce settings. It is declared here and implemented
// by a deliverability adapter. ProcessFeedback tolerates a nil Suppressor —
// the suppression step is then skipped (the ingestion-only slice).
type Suppressor interface {
	Apply(ctx context.Context, e *domain.DeliveryEvent) error
}

// IngestNotification is the request to ingest one raw notification read from
// the Postbox feedback stream.
type IngestNotification struct {
	RawPayload []byte
}

// IngestNotificationHandler handles IngestNotification: parse the notification,
// stage it idempotently, and enqueue asynchronous attribution. A notification
// of an event type Phase 4 ignores is a no-op.
type IngestNotificationHandler struct {
	parser   NotificationParser
	events   domain.EventRepository
	enqueuer FeedbackEnqueuer
}

// NewIngestNotificationHandler builds the handler, failing fast on a nil
// dependency.
func NewIngestNotificationHandler(parser NotificationParser,
	events domain.EventRepository, enqueuer FeedbackEnqueuer) IngestNotificationHandler {
	if parser == nil || events == nil || enqueuer == nil {
		panic("nil dependency")
	}
	return IngestNotificationHandler{parser: parser, events: events, enqueuer: enqueuer}
}

// Handle parses, stages, and enqueues one notification. A duplicate dedupe key
// stages nothing but the job is still enqueued — a redelivered job finds the
// row already processed and no-ops.
func (h IngestNotificationHandler) Handle(ctx context.Context, cmd IngestNotification) error {
	n, recognized, err := h.parser.Parse(cmd.RawPayload)
	if err != nil {
		return err
	}
	if !recognized {
		return nil // Send / DeliveryDelay / Unsubscribe — read past it.
	}
	eventID, _, err := h.events.StageInbound(ctx, n)
	if err != nil {
		return err
	}
	return h.enqueuer.EnqueueFeedbackProcess(ctx, eventID)
}

// ProcessFeedback is the request to process one staged notification.
type ProcessFeedback struct {
	InboundEventID string
}

// ProcessFeedbackHandler handles ProcessFeedback: resolve the owning tenant,
// attribute the notification to a send, record the delivery event, and apply
// suppression. It is idempotent — a redelivered or retried job converges to
// the same state.
type ProcessFeedbackHandler struct {
	events     domain.EventRepository
	suppressor Suppressor
	logger     *slog.Logger
}

// NewProcessFeedbackHandler builds the handler, failing fast on a nil events
// repository. suppressor may be nil — the suppression step is then skipped.
func NewProcessFeedbackHandler(events domain.EventRepository, suppressor Suppressor,
	logger *slog.Logger) ProcessFeedbackHandler {
	if events == nil {
		panic("nil dependency")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return ProcessFeedbackHandler{events: events, suppressor: suppressor, logger: logger}
}

// Handle processes one staged notification.
func (h ProcessFeedbackHandler) Handle(ctx context.Context, cmd ProcessFeedback) error {
	n, err := h.events.LoadInbound(ctx, cmd.InboundEventID)
	if err != nil {
		return err
	}
	if n.IsProcessed() {
		return nil // already attributed or unattributed — idempotent no-op
	}

	tenantID, ok, err := h.events.TenantForMessage(ctx, n.ProviderMessageID)
	if err != nil {
		return err
	}
	if !ok {
		return h.markUnattributed(ctx, n)
	}

	attr, found, err := h.events.Attribute(ctx, tenantID, n.ProviderMessageID)
	if err != nil {
		return err
	}
	if !found {
		return h.markUnattributed(ctx, n)
	}

	event, err := domain.NewDeliveryEvent(tenantID, n.ID, n.Kind,
		n.RecipientEmail, n.ProviderMessageID, n.OccurredAt, attr)
	if err != nil {
		return err
	}

	recorded, err := h.events.RecordEvent(ctx, event)
	if err != nil {
		return err
	}
	// Suppression runs only when the event was newly recorded, so a retried
	// job never applies it twice.
	if recorded && h.suppressor != nil {
		if _, drives := event.SuppressionReason(); drives {
			if err := h.suppressor.Apply(ctx, event); err != nil {
				return err
			}
		}
	}
	return h.events.MarkInbound(ctx, n.ID, domain.InboundAttributed)
}

// markUnattributed records that no send matched the notification's provider
// message id, surfacing it as a structured log for monitoring (FR-009).
func (h ProcessFeedbackHandler) markUnattributed(ctx context.Context,
	n domain.InboundNotification) error {

	h.logger.WarnContext(ctx, "inbound notification unattributed",
		"inbound_event_id", n.ID, "provider_message_id", n.ProviderMessageID,
		"event_kind", string(n.Kind))
	return h.events.MarkInbound(ctx, n.ID, domain.InboundUnattributed)
}
