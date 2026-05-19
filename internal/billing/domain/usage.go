package domain

import "time"

// UsageEventType classifies a metered send.
type UsageEventType string

const (
	// UsageCampaignSend is one campaign recipient sent.
	UsageCampaignSend UsageEventType = "campaign_send"
	// UsageTransactionalSend is one transactional message sent.
	UsageTransactionalSend UsageEventType = "transactional_send"
)

// UsageEvent is one recorded billable send. Its source_ref — a campaign
// recipient id or a transactional message id — makes recording the same send
// twice a no-op.
type UsageEvent struct {
	id          string
	tenantID    string
	eventType   UsageEventType
	quantity    int64
	sourceRef   string
	occurredAt  time.Time
	periodStart time.Time
	rolledUpAt  *time.Time
}

// NewUsageEvent builds a usage event of quantity one, attributed to the billing
// period beginning at periodStart.
func NewUsageEvent(tenantID string, eventType UsageEventType, sourceRef string,
	periodStart, occurredAt time.Time) *UsageEvent {

	return &UsageEvent{
		tenantID:    tenantID,
		eventType:   eventType,
		quantity:    1,
		sourceRef:   sourceRef,
		occurredAt:  occurredAt.UTC(),
		periodStart: periodStart.UTC(),
	}
}

// HydrateUsageEvent reconstructs a usage event from a persisted row.
func HydrateUsageEvent(id, tenantID string, eventType UsageEventType, quantity int64,
	sourceRef string, occurredAt, periodStart time.Time, rolledUpAt *time.Time) *UsageEvent {

	return &UsageEvent{
		id: id, tenantID: tenantID, eventType: eventType, quantity: quantity,
		sourceRef: sourceRef, occurredAt: occurredAt, periodStart: periodStart,
		rolledUpAt: rolledUpAt,
	}
}

// ID returns the database-assigned id.
func (e *UsageEvent) ID() string { return e.id }

// TenantID returns the owning tenant's id.
func (e *UsageEvent) TenantID() string { return e.tenantID }

// EventType returns the kind of send.
func (e *UsageEvent) EventType() UsageEventType { return e.eventType }

// Quantity returns the billed quantity.
func (e *UsageEvent) Quantity() int64 { return e.quantity }

// SourceRef returns the stable per-send identifier.
func (e *UsageEvent) SourceRef() string { return e.sourceRef }

// OccurredAt returns when the send happened.
func (e *UsageEvent) OccurredAt() time.Time { return e.occurredAt }

// PeriodStart returns the billing period the event is attributed to.
func (e *UsageEvent) PeriodStart() time.Time { return e.periodStart }

// RolledUp reports whether the event has been included in a counter.
func (e *UsageEvent) RolledUp() bool { return e.rolledUpAt != nil }

// UsageCounter is a per-tenant, per-period aggregate of metered sends produced
// by the usage.rollup job.
type UsageCounter struct {
	id               string
	tenantID         string
	periodStart      time.Time
	periodEnd        time.Time
	eventType        UsageEventType
	totalQuantity    int64
	includedQuantity int64
	overageQuantity  int64
}

// NewUsageCounter builds a zeroed counter for a period and event type.
func NewUsageCounter(tenantID string, periodStart, periodEnd time.Time,
	eventType UsageEventType) *UsageCounter {

	return &UsageCounter{
		tenantID: tenantID, periodStart: periodStart.UTC(), periodEnd: periodEnd.UTC(),
		eventType: eventType,
	}
}

// HydrateUsageCounter reconstructs a counter from a persisted row.
func HydrateUsageCounter(id, tenantID string, periodStart, periodEnd time.Time,
	eventType UsageEventType, total, included, overage int64) *UsageCounter {

	return &UsageCounter{
		id: id, tenantID: tenantID, periodStart: periodStart, periodEnd: periodEnd,
		eventType: eventType, totalQuantity: total, includedQuantity: included,
		overageQuantity: overage,
	}
}

// ID returns the database-assigned id.
func (c *UsageCounter) ID() string { return c.id }

// TenantID returns the owning tenant's id.
func (c *UsageCounter) TenantID() string { return c.tenantID }

// PeriodStart returns the start of the counted period.
func (c *UsageCounter) PeriodStart() time.Time { return c.periodStart }

// PeriodEnd returns the end of the counted period.
func (c *UsageCounter) PeriodEnd() time.Time { return c.periodEnd }

// EventType returns the counted send kind.
func (c *UsageCounter) EventType() UsageEventType { return c.eventType }

// TotalQuantity returns the rolled-up send count for the period.
func (c *UsageCounter) TotalQuantity() int64 { return c.totalQuantity }

// IncludedQuantity returns the part of the total within the plan allowance.
func (c *UsageCounter) IncludedQuantity() int64 { return c.includedQuantity }

// OverageQuantity returns the part of the total beyond the plan allowance.
func (c *UsageCounter) OverageQuantity() int64 { return c.overageQuantity }

// SplitUsage divides a usage total into the part within an allowance and the
// overage beyond it.
func SplitUsage(total, allowance int64) (included, overage int64) {
	if total <= allowance {
		return total, 0
	}
	return allowance, total - allowance
}
