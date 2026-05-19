package adapters

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// UsageRecorder records metered sends as usage events. It implements the
// campaign context's consumer-owned UsageRecorder port by resolving the
// tenant's current billing period and writing one usage_event per send ref.
type UsageRecorder struct {
	subscriptions domain.SubscriptionRepository
	usage         domain.UsageRepository
}

var _ campaigndomain.UsageRecorder = (*UsageRecorder)(nil)

// NewUsageRecorder builds a UsageRecorder over the subscription and usage
// repositories.
func NewUsageRecorder(subscriptions domain.SubscriptionRepository,
	usage domain.UsageRepository) *UsageRecorder {
	return &UsageRecorder{subscriptions: subscriptions, usage: usage}
}

// Record writes one usage event per ref, attributed to the tenant's current
// billing period. A tenant with no subscription is not metered.
func (r *UsageRecorder) Record(ctx context.Context, tenantID, eventType string, refs []string) error {
	if len(refs) == 0 {
		return nil
	}
	sub, found, err := r.subscriptions.Current(ctx, tenantID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	now := time.Now().UTC()
	events := make([]*domain.UsageEvent, 0, len(refs))
	for _, ref := range refs {
		events = append(events, domain.NewUsageEvent(tenantID,
			domain.UsageEventType(eventType), ref, sub.CurrentPeriodStart(), now))
	}
	return r.usage.RecordEvents(ctx, tenantID, events)
}
