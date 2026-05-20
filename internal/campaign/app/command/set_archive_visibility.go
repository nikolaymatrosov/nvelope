package command

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// SetArchiveVisibility is the request to toggle whether a campaign appears on
// the tenant's public archive index and RSS feed.
type SetArchiveVisibility struct {
	TenantID   string
	CampaignID string
	Visible    bool
}

// SetArchiveVisibilityHandler handles the SetArchiveVisibility command.
type SetArchiveVisibilityHandler struct {
	campaigns domain.CampaignRepository
}

// NewSetArchiveVisibilityHandler builds the handler, failing fast on a nil
// dependency.
func NewSetArchiveVisibilityHandler(campaigns domain.CampaignRepository) SetArchiveVisibilityHandler {
	if campaigns == nil {
		panic("nil campaign repository")
	}
	return SetArchiveVisibilityHandler{campaigns: campaigns}
}

// Handle toggles archive visibility on the campaign. Only a campaign whose
// send has begun may be made archive-visible.
func (h SetArchiveVisibilityHandler) Handle(ctx context.Context, cmd SetArchiveVisibility) error {
	return h.campaigns.Update(ctx, cmd.TenantID, cmd.CampaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			return c, c.SetArchiveVisible(cmd.Visible, time.Now())
		})
}
