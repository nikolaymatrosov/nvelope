package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// ArchiveEntryView is the read model for one archive-visible campaign — the
// data the public archive index and RSS feed render.
type ArchiveEntryView struct {
	ID         string
	Name       string
	Subject    string
	BodyHTML   string
	BodyText   string
	ArchivedAt time.Time
}

// ListArchive is the request for the tenant's public archive index.
type ListArchive struct {
	TenantID string
	Page     domain.Page
}

// ListArchiveHandler handles the ListArchive query.
type ListArchiveHandler struct {
	campaigns domain.CampaignRepository
}

// NewListArchiveHandler builds the handler, failing fast on a nil dependency.
func NewListArchiveHandler(campaigns domain.CampaignRepository) ListArchiveHandler {
	if campaigns == nil {
		panic("nil campaign repository")
	}
	return ListArchiveHandler{campaigns: campaigns}
}

// Handle returns the archive-visible campaigns of the tenant, newest-first.
func (h ListArchiveHandler) Handle(ctx context.Context, q ListArchive) ([]ArchiveEntryView, error) {
	campaigns, _, err := h.campaigns.Archived(ctx, q.TenantID, q.Page)
	if err != nil {
		return nil, err
	}
	out := make([]ArchiveEntryView, 0, len(campaigns))
	for _, c := range campaigns {
		out = append(out, archiveEntryView(c))
	}
	return out, nil
}

// GetArchivedCampaign is the request for one archive-visible campaign.
type GetArchivedCampaign struct {
	TenantID   string
	CampaignID string
}

// GetArchivedCampaignHandler handles the GetArchivedCampaign query.
type GetArchivedCampaignHandler struct {
	campaigns domain.CampaignRepository
}

// NewGetArchivedCampaignHandler builds the handler, failing fast on a nil
// dependency.
func NewGetArchivedCampaignHandler(campaigns domain.CampaignRepository) GetArchivedCampaignHandler {
	if campaigns == nil {
		panic("nil campaign repository")
	}
	return GetArchivedCampaignHandler{campaigns: campaigns}
}

// Handle returns the archived campaign. A campaign that is not archive-visible
// is reported as not-found — no distinction is leaked between "missing" and
// "hidden".
func (h GetArchivedCampaignHandler) Handle(ctx context.Context, q GetArchivedCampaign) (ArchiveEntryView, error) {
	c, err := h.campaigns.Get(ctx, q.TenantID, q.CampaignID)
	if err != nil {
		return ArchiveEntryView{}, err
	}
	if !c.ArchiveVisible() {
		return ArchiveEntryView{}, domain.ErrCampaignNotFound
	}
	return archiveEntryView(c), nil
}

func archiveEntryView(c *domain.Campaign) ArchiveEntryView {
	v := ArchiveEntryView{
		ID:       c.ID(),
		Name:     c.Name(),
		Subject:  c.Subject(),
		BodyHTML: c.BodyHTML(),
		BodyText: c.BodyText(),
	}
	if at := c.ArchivedAt(); at != nil {
		v.ArchivedAt = *at
	}
	return v
}
