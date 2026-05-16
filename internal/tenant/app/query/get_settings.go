package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// GetSettings is the request for a workspace's settings.
type GetSettings struct {
	TenantID string
}

// GetSettingsHandler handles the GetSettings query.
type GetSettingsHandler struct {
	settings domain.SettingsRepository
}

// NewGetSettingsHandler builds the handler, failing fast on a nil dependency.
func NewGetSettingsHandler(settings domain.SettingsRepository) GetSettingsHandler {
	if settings == nil {
		panic("nil settings repository")
	}
	return GetSettingsHandler{settings: settings}
}

// Handle returns the workspace's settings, read inside the tenant-bound
// transaction.
func (h GetSettingsHandler) Handle(ctx context.Context, q GetSettings) (SettingsView, error) {
	s, err := h.settings.Get(ctx, q.TenantID)
	if err != nil {
		return SettingsView{}, err
	}
	return SettingsView{DisplayName: s.DisplayName(), Timezone: s.Timezone()}, nil
}
