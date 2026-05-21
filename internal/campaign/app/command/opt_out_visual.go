package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// OptOutVisualCampaign clears a campaign's structured visual document and
// theme override so the row reverts to a code-only campaign (per
// FR-029 / contracts/tenant-api.md "opt-out-visual"). body_html and
// body_text stay intact so the campaign remains sendable.
type OptOutVisualCampaign struct {
	TenantID   string
	CampaignID string
}

// OptOutVisualCampaignHandler handles the OptOutVisualCampaign command.
type OptOutVisualCampaignHandler struct {
	campaigns domain.CampaignRepository
}

// NewOptOutVisualCampaignHandler builds the handler, failing fast on a nil
// dependency.
func NewOptOutVisualCampaignHandler(campaigns domain.CampaignRepository) OptOutVisualCampaignHandler {
	if campaigns == nil {
		panic("nil campaign repository")
	}
	return OptOutVisualCampaignHandler{campaigns: campaigns}
}

// Handle clears the campaign's structured visual document under the
// tenant-bound transaction owned by the repository.
func (h OptOutVisualCampaignHandler) Handle(ctx context.Context, cmd OptOutVisualCampaign) error {
	return h.campaigns.Update(ctx, cmd.TenantID, cmd.CampaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			return c, c.OptOutVisual()
		})
}

// OptOutVisualTemplate clears a template's structured visual document and
// theme override so the row reverts to a code-only template.
type OptOutVisualTemplate struct {
	TenantID   string
	TemplateID string
}

// OptOutVisualTemplateHandler handles the OptOutVisualTemplate command.
type OptOutVisualTemplateHandler struct {
	templates domain.TemplateRepository
}

// NewOptOutVisualTemplateHandler builds the handler, failing fast on a nil
// dependency.
func NewOptOutVisualTemplateHandler(templates domain.TemplateRepository) OptOutVisualTemplateHandler {
	if templates == nil {
		panic("nil template repository")
	}
	return OptOutVisualTemplateHandler{templates: templates}
}

// Handle clears the template's structured visual document under the
// tenant-bound transaction owned by the repository.
func (h OptOutVisualTemplateHandler) Handle(ctx context.Context, cmd OptOutVisualTemplate) error {
	return h.templates.Update(ctx, cmd.TenantID, cmd.TemplateID,
		func(t *domain.Template) (*domain.Template, error) {
			return t, t.OptOutVisual()
		})
}
