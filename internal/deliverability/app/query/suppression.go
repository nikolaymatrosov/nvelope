// Package query holds the deliverability context's read-only use cases: the
// suppression list, bounce settings, and campaign analytics views.
package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// ListSuppressions is the request for a page of the tenant's suppression list.
type ListSuppressions struct {
	TenantID  string
	Cursor    string
	Limit     int
	Reason    string
	EmailLike string
}

// SuppressionEntryView is one suppression entry shaped for the API.
type SuppressionEntryView struct {
	Email        string
	Reason       string
	SuppressedAt time.Time
	Note         string
}

// SuppressionPage is a page of suppression entries with the next cursor.
type SuppressionPage struct {
	Items      []SuppressionEntryView
	NextCursor string
}

// ListSuppressionsHandler handles ListSuppressions.
type ListSuppressionsHandler struct {
	suppressions domain.SuppressionRepository
}

// NewListSuppressionsHandler builds the handler, failing fast on a nil
// dependency.
func NewListSuppressionsHandler(suppressions domain.SuppressionRepository) ListSuppressionsHandler {
	if suppressions == nil {
		panic("nil dependency")
	}
	return ListSuppressionsHandler{suppressions: suppressions}
}

// Handle returns a page of the tenant's suppression entries.
func (h ListSuppressionsHandler) Handle(ctx context.Context, q ListSuppressions) (SuppressionPage, error) {
	entries, next, err := h.suppressions.List(ctx, q.TenantID, domain.SuppressionFilter{
		Reason:    domain.SuppressionReason(q.Reason),
		EmailLike: q.EmailLike,
		Cursor:    q.Cursor,
		Limit:     q.Limit,
	})
	if err != nil {
		return SuppressionPage{}, err
	}
	items := make([]SuppressionEntryView, 0, len(entries))
	for _, e := range entries {
		items = append(items, SuppressionEntryView{
			Email:        e.Email(),
			Reason:       string(e.Reason()),
			SuppressedAt: e.SuppressedAt(),
			Note:         e.Note(),
		})
	}
	return SuppressionPage{Items: items, NextCursor: next}, nil
}

// GetBounceSettings is the request for a tenant's bounce-action configuration.
type GetBounceSettings struct {
	TenantID string
}

// BounceSettingsView is a tenant's bounce settings shaped for the API.
type BounceSettingsView struct {
	SuppressOnHardBounce bool
	SuppressOnComplaint  bool
}

// GetBounceSettingsHandler handles GetBounceSettings.
type GetBounceSettingsHandler struct {
	settings domain.SettingsRepository
}

// NewGetBounceSettingsHandler builds the handler, failing fast on a nil
// dependency.
func NewGetBounceSettingsHandler(settings domain.SettingsRepository) GetBounceSettingsHandler {
	if settings == nil {
		panic("nil dependency")
	}
	return GetBounceSettingsHandler{settings: settings}
}

// Handle returns the tenant's effective bounce settings, the defaults when no
// row exists.
func (h GetBounceSettingsHandler) Handle(ctx context.Context, q GetBounceSettings) (
	BounceSettingsView, error) {

	s, err := h.settings.Get(ctx, q.TenantID)
	if err != nil {
		return BounceSettingsView{}, err
	}
	return BounceSettingsView{
		SuppressOnHardBounce: s.SuppressOnHardBounce(),
		SuppressOnComplaint:  s.SuppressOnComplaint(),
	}, nil
}
