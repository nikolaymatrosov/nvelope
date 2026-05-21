package query

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// CampaignView is the read model of a campaign, including its send progress.
// ListIDs and Segments carry the campaign's send targets; they are populated
// by the single-campaign query (GetCampaign) only, so the editor can show and
// preserve the current targeting. BodyDoc and Theme carry the JSON
// pass-through of the structured visual document and the operator's pinned
// theme override; both are nil for legacy raw-HTML / code-only campaigns.
type CampaignView struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Subject         string            `json:"subject"`
	BodyHTML        string            `json:"body_html"`
	BodyText        string            `json:"body_text"`
	BodyDoc         json.RawMessage   `json:"body_doc,omitempty"`
	Theme           json.RawMessage   `json:"theme,omitempty"`
	FromName        string            `json:"from_name"`
	FromLocalPart   string            `json:"from_local_part"`
	SendingDomainID string            `json:"sending_domain_id,omitempty"`
	TemplateID      string            `json:"template_id,omitempty"`
	Status          string            `json:"status"`
	MaxSendErrors   int               `json:"max_send_errors"`
	SentCount       int               `json:"sent_count"`
	FailedCount     int               `json:"failed_count"`
	RecipientCount  int               `json:"recipient_count"`
	ListIDs         []string          `json:"list_ids"`
	Segments        []json.RawMessage `json:"segments"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	StartedAt       *time.Time        `json:"started_at,omitempty"`
	FinishedAt      *time.Time        `json:"finished_at,omitempty"`
}

// campaignView projects a Campaign aggregate onto its read model.
func campaignView(c *domain.Campaign) CampaignView {
	return CampaignView{
		ID: c.ID(), Name: c.Name(), Subject: c.Subject(),
		BodyHTML: c.BodyHTML(), BodyText: c.BodyText(),
		BodyDoc: c.BodyDocJSON(), Theme: c.ThemeJSON(),
		FromName: c.FromName(), FromLocalPart: c.FromLocalPart(),
		SendingDomainID: c.SendingDomainID(), TemplateID: c.TemplateID(),
		Status: string(c.Status()), MaxSendErrors: c.MaxSendErrors(),
		SentCount: c.SentCount(), FailedCount: c.FailedCount(),
		RecipientCount: c.RecipientCount(),
		CreatedAt:      c.CreatedAt(), UpdatedAt: c.UpdatedAt(),
		StartedAt: c.StartedAt(), FinishedAt: c.FinishedAt(),
	}
}

// CampaignPage is a page of campaigns with the total count.
type CampaignPage struct {
	Campaigns []CampaignView
	Total     int
}

// ListCampaigns is the request for a page of the tenant's campaigns.
type ListCampaigns struct {
	TenantID string
	Page     domain.Page
}

// ListCampaignsHandler handles the ListCampaigns query.
type ListCampaignsHandler struct {
	campaigns domain.CampaignRepository
}

// NewListCampaignsHandler builds the handler, failing fast on a nil dependency.
func NewListCampaignsHandler(campaigns domain.CampaignRepository) ListCampaignsHandler {
	if campaigns == nil {
		panic("nil campaign repository")
	}
	return ListCampaignsHandler{campaigns: campaigns}
}

// Handle returns a page of the tenant's campaigns with their progress counts.
func (h ListCampaignsHandler) Handle(ctx context.Context, q ListCampaigns) (CampaignPage, error) {
	campaigns, total, err := h.campaigns.All(ctx, q.TenantID, q.Page)
	if err != nil {
		return CampaignPage{}, err
	}
	page := CampaignPage{Total: total, Campaigns: make([]CampaignView, 0, len(campaigns))}
	for _, c := range campaigns {
		page.Campaigns = append(page.Campaigns, campaignView(c))
	}
	return page, nil
}

// GetCampaign is the request for a single campaign.
type GetCampaign struct {
	TenantID   string
	CampaignID string
}

// GetCampaignHandler handles the GetCampaign query.
type GetCampaignHandler struct {
	campaigns domain.CampaignRepository
}

// NewGetCampaignHandler builds the handler, failing fast on a nil dependency.
func NewGetCampaignHandler(campaigns domain.CampaignRepository) GetCampaignHandler {
	if campaigns == nil {
		panic("nil campaign repository")
	}
	return GetCampaignHandler{campaigns: campaigns}
}

// Handle returns the requested campaign and its send targets, or
// domain.ErrCampaignNotFound.
func (h GetCampaignHandler) Handle(ctx context.Context, q GetCampaign) (CampaignView, error) {
	c, err := h.campaigns.Get(ctx, q.TenantID, q.CampaignID)
	if err != nil {
		return CampaignView{}, err
	}
	view := campaignView(c)
	targets, err := h.campaigns.Targets(ctx, q.TenantID, q.CampaignID)
	if err != nil {
		return CampaignView{}, err
	}
	for _, t := range targets {
		if t.ListID != "" {
			view.ListIDs = append(view.ListIDs, t.ListID)
		}
		if len(t.SegmentQuery) > 0 {
			view.Segments = append(view.Segments, json.RawMessage(t.SegmentQuery))
		}
	}
	return view, nil
}
