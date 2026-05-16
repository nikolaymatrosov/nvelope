package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// UpdateSettings is the request to change a workspace's settings.
type UpdateSettings struct {
	TenantID    string
	DisplayName string
	Timezone    string
}

// UpdateSettingsResult carries the updated settings.
type UpdateSettingsResult struct {
	DisplayName string
	Timezone    string
}

// UpdateSettingsHandler handles the UpdateSettings command.
type UpdateSettingsHandler struct {
	settings domain.SettingsRepository
}

// NewUpdateSettingsHandler builds the handler, failing fast on a nil
// dependency.
func NewUpdateSettingsHandler(settings domain.SettingsRepository) UpdateSettingsHandler {
	if settings == nil {
		panic("nil settings repository")
	}
	return UpdateSettingsHandler{settings: settings}
}

// Handle applies the new display name and timezone inside the tenant-bound
// transaction.
func (h UpdateSettingsHandler) Handle(ctx context.Context, cmd UpdateSettings) (UpdateSettingsResult, error) {
	var updated *domain.TenantSettings
	err := h.settings.Update(ctx, cmd.TenantID,
		func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
			if err := s.Rename(cmd.DisplayName); err != nil {
				return nil, err
			}
			if err := s.SetTimezone(cmd.Timezone); err != nil {
				return nil, err
			}
			updated = s
			return s, nil
		})
	if err != nil {
		return UpdateSettingsResult{}, err
	}
	return UpdateSettingsResult{
		DisplayName: updated.DisplayName(),
		Timezone:    updated.Timezone(),
	}, nil
}
