package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// AuditWriter records a privileged deliverability action in the shared audit
// log. It is declared here, by the consuming use cases, and implemented by a
// deliverability adapter.
type AuditWriter interface {
	Record(ctx context.Context, tenantID, actorID, action, target string) error
}

// AddSuppression is the request to manually add an address to the tenant's
// suppression list.
type AddSuppression struct {
	TenantID string
	ActorID  string
	Email    string
	Note     string
}

// AddSuppressionHandler handles AddSuppression: it adds a manual suppression
// entry and records the action in the audit log. Adding an address already
// suppressed is idempotent.
type AddSuppressionHandler struct {
	suppressions domain.SuppressionRepository
	audit        AuditWriter
}

// NewAddSuppressionHandler builds the handler, failing fast on a nil dependency.
func NewAddSuppressionHandler(suppressions domain.SuppressionRepository,
	audit AuditWriter) AddSuppressionHandler {
	if suppressions == nil || audit == nil {
		panic("nil dependency")
	}
	return AddSuppressionHandler{suppressions: suppressions, audit: audit}
}

// Handle adds the address and writes the audit entry.
func (h AddSuppressionHandler) Handle(ctx context.Context, cmd AddSuppression) error {
	entry, err := domain.NewManualSuppression(cmd.TenantID, cmd.Email, cmd.Note)
	if err != nil {
		return err
	}
	if err := h.suppressions.Upsert(ctx, entry); err != nil {
		return err
	}
	return h.audit.Record(ctx, cmd.TenantID, cmd.ActorID, "suppression.added", entry.Email())
}

// RemoveSuppression is the request to remove an address from the suppression
// list, making it mailable again.
type RemoveSuppression struct {
	TenantID string
	ActorID  string
	Email    string
}

// RemoveSuppressionHandler handles RemoveSuppression.
type RemoveSuppressionHandler struct {
	suppressions domain.SuppressionRepository
	audit        AuditWriter
}

// NewRemoveSuppressionHandler builds the handler, failing fast on a nil
// dependency.
func NewRemoveSuppressionHandler(suppressions domain.SuppressionRepository,
	audit AuditWriter) RemoveSuppressionHandler {
	if suppressions == nil || audit == nil {
		panic("nil dependency")
	}
	return RemoveSuppressionHandler{suppressions: suppressions, audit: audit}
}

// Handle removes the address and writes the audit entry. A missing entry
// returns ErrSuppressionNotFound before any audit record is written.
func (h RemoveSuppressionHandler) Handle(ctx context.Context, cmd RemoveSuppression) error {
	if err := h.suppressions.Remove(ctx, cmd.TenantID, cmd.Email); err != nil {
		return err
	}
	return h.audit.Record(ctx, cmd.TenantID, cmd.ActorID, "suppression.removed", cmd.Email)
}

// UpdateBounceSettings is the request to change a tenant's bounce-action
// configuration.
type UpdateBounceSettings struct {
	TenantID           string
	ActorID            string
	SuppressHardBounce bool
	SuppressComplaint  bool
}

// UpdateBounceSettingsHandler handles UpdateBounceSettings.
type UpdateBounceSettingsHandler struct {
	settings domain.SettingsRepository
	audit    AuditWriter
}

// NewUpdateBounceSettingsHandler builds the handler, failing fast on a nil
// dependency.
func NewUpdateBounceSettingsHandler(settings domain.SettingsRepository,
	audit AuditWriter) UpdateBounceSettingsHandler {
	if settings == nil || audit == nil {
		panic("nil dependency")
	}
	return UpdateBounceSettingsHandler{settings: settings, audit: audit}
}

// Handle upserts the tenant's bounce settings and writes the audit entry.
func (h UpdateBounceSettingsHandler) Handle(ctx context.Context, cmd UpdateBounceSettings) error {
	s := domain.NewBounceSettings(cmd.TenantID, cmd.SuppressHardBounce, cmd.SuppressComplaint)
	if err := h.settings.Put(ctx, cmd.TenantID, s); err != nil {
		return err
	}
	return h.audit.Record(ctx, cmd.TenantID, cmd.ActorID, "bounce_settings.updated", "")
}
