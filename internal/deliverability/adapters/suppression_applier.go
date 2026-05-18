package adapters

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// SuppressionApplier applies suppression for a newly recorded bounce or
// complaint, honouring the tenant's bounce settings. It implements the command
// layer's Suppressor port.
type SuppressionApplier struct {
	suppressions domain.SuppressionRepository
	settings     domain.SettingsRepository
}

var _ command.Suppressor = (*SuppressionApplier)(nil)

// NewSuppressionApplier builds a SuppressionApplier, failing fast on a nil
// dependency.
func NewSuppressionApplier(suppressions domain.SuppressionRepository,
	settings domain.SettingsRepository) *SuppressionApplier {
	if suppressions == nil || settings == nil {
		panic("nil dependency")
	}
	return &SuppressionApplier{suppressions: suppressions, settings: settings}
}

// Apply suppresses the event's recipient when the event is a hard bounce or a
// complaint and the tenant's bounce settings call for it. A delivery, open, or
// click event applies no suppression.
func (a *SuppressionApplier) Apply(ctx context.Context, e *domain.DeliveryEvent) error {
	reason, ok := e.SuppressionReason()
	if !ok {
		return nil
	}
	settings, err := a.settings.Get(ctx, e.TenantID())
	if err != nil {
		return err
	}
	switch reason {
	case domain.ReasonHardBounce:
		if !settings.ShouldSuppressHardBounce() {
			return nil
		}
	case domain.ReasonComplaint:
		if !settings.ShouldSuppressComplaint() {
			return nil
		}
	default:
		return nil
	}
	entry, err := domain.NewSuppressionEntry(e.TenantID(), e.RecipientEmail(), reason, e.ID())
	if err != nil {
		return err
	}
	return a.suppressions.Upsert(ctx, entry)
}
