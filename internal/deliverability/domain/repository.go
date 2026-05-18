package domain

import "context"

// EventRepository persists inbound notifications and the attributed delivery
// events. The control-plane staging methods run through the pool; the
// tenant-plane methods run inside the RLS-bound transaction.
type EventRepository interface {
	// StageInbound records a parsed notification for asynchronous processing.
	// A duplicate dedupe key is a no-op; staged reports whether a new row was
	// written.
	StageInbound(ctx context.Context, n InboundNotification) (eventID string, staged bool, err error)

	// LoadInbound fetches a staged notification by id for the worker.
	LoadInbound(ctx context.Context, eventID string) (InboundNotification, error)

	// TenantForMessage resolves the owning tenant of a provider message id via
	// the SECURITY DEFINER lookup; ok is false when no send matches.
	TenantForMessage(ctx context.Context, providerMessageID string) (tenantID string, ok bool, err error)

	// Attribute matches a provider message id to a campaign recipient or a
	// transactional message within the tenant; ok is false when neither
	// matches.
	Attribute(ctx context.Context, tenantID, providerMessageID string) (Attribution, bool, error)

	// RecordEvent inserts the attributed delivery event inside the tenant
	// transaction. A row already present for the inbound event is a no-op;
	// recorded reports whether a new row was written.
	RecordEvent(ctx context.Context, e *DeliveryEvent) (recorded bool, err error)

	// MarkInbound sets the staged row's terminal status and processed-at time.
	MarkInbound(ctx context.Context, eventID string, status InboundStatus) error
}

// SuppressionRepository persists the tenant's suppression list. Every
// operation runs inside the RLS-bound tenant transaction.
type SuppressionRepository interface {
	// Upsert adds an entry; an address already suppressed for the tenant is a
	// no-op (ON CONFLICT DO NOTHING).
	Upsert(ctx context.Context, e *SuppressionEntry) error

	// Remove deletes the entry; returns ErrSuppressionNotFound when absent.
	Remove(ctx context.Context, tenantID, email string) error

	// List returns a page of the tenant's entries and the next cursor, empty
	// when the page is the last.
	List(ctx context.Context, tenantID string, f SuppressionFilter) ([]*SuppressionEntry, string, error)
}

// SettingsRepository persists per-tenant bounce settings. Every operation runs
// inside the RLS-bound tenant transaction.
type SettingsRepository interface {
	// Get returns the tenant's bounce settings, or DefaultBounceSettings when
	// no row exists.
	Get(ctx context.Context, tenantID string) (*BounceSettings, error)

	// Put upserts the tenant's bounce settings.
	Put(ctx context.Context, tenantID string, s *BounceSettings) error
}

// AnalyticsRepository serves the pre-computed campaign analytics and refreshes
// them. Every operation runs inside the RLS-bound tenant transaction.
type AnalyticsRepository interface {
	// GetCampaign returns one campaign's pre-computed roll-up; ok is false when
	// the campaign has no analytics row yet.
	GetCampaign(ctx context.Context, tenantID, campaignID string) (CampaignAnalytics, bool, error)

	// GetDashboard returns the tenant totals and recent-campaign summaries.
	GetDashboard(ctx context.Context, tenantID string) (Dashboard, error)

	// Refresh recomputes and upserts every campaign_analytics row for the
	// tenant inside its bound transaction (the analytics.refresh job).
	Refresh(ctx context.Context, tenantID string) error
}
