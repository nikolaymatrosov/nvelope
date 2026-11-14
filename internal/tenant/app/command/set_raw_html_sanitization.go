package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// SetRawHTMLSanitization is the request to change how a workspace's raw
// campaign and template HTML is sanitized on save.
type SetRawHTMLSanitization struct {
	TenantID   string
	Level      string
	AcceptRisk bool
}

// SetRawHTMLSanitizationResult carries the level before and after, for the
// audit trail.
type SetRawHTMLSanitizationResult struct {
	Previous string
	Current  string
}

// SetRawHTMLSanitizationHandler handles the SetRawHTMLSanitization command.
type SetRawHTMLSanitizationHandler struct {
	settings domain.SettingsRepository
}

// NewSetRawHTMLSanitizationHandler builds the handler, failing fast on a nil
// dependency.
func NewSetRawHTMLSanitizationHandler(settings domain.SettingsRepository) SetRawHTMLSanitizationHandler {
	if settings == nil {
		panic("nil settings repository")
	}
	return SetRawHTMLSanitizationHandler{settings: settings}
}

// Handle applies the new level inside the tenant-bound transaction.
func (h SetRawHTMLSanitizationHandler) Handle(ctx context.Context,
	cmd SetRawHTMLSanitization) (SetRawHTMLSanitizationResult, error) {
	var res SetRawHTMLSanitizationResult
	err := h.settings.Update(ctx, cmd.TenantID,
		func(s *domain.TenantSettings) (*domain.TenantSettings, error) {
			res.Previous = string(s.RawHTMLSanitization())
			if err := s.SetRawHTMLSanitization(domain.RawHTMLSanitization(cmd.Level), cmd.AcceptRisk); err != nil {
				return nil, err
			}
			res.Current = string(s.RawHTMLSanitization())
			return s, nil
		})
	if err != nil {
		return SetRawHTMLSanitizationResult{}, err
	}
	return res, nil
}
