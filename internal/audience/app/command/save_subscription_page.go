package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// SaveSubscriptionPage is the request to create a subscription page (empty
// PageID) or update an existing one.
type SaveSubscriptionPage struct {
	TenantID        string
	PageID          string
	Slug            string
	Title           string
	TargetListIDs   []string
	Fields          []domain.FormField
	SendingDomainID string
	FromName        string
	FromLocalPart   string
	Active          bool
}

// SaveSubscriptionPageResult carries the page's id.
type SaveSubscriptionPageResult struct {
	PageID string
}

// SaveSubscriptionPageHandler handles the SaveSubscriptionPage command.
type SaveSubscriptionPageHandler struct {
	pages   domain.SubscriptionPageRepository
	lists   domain.ListRepository
	domains domain.SendingDomainChecker
}

// NewSaveSubscriptionPageHandler builds the handler, failing fast on a nil
// dependency.
func NewSaveSubscriptionPageHandler(pages domain.SubscriptionPageRepository,
	lists domain.ListRepository, domains domain.SendingDomainChecker) SaveSubscriptionPageHandler {

	if pages == nil || lists == nil || domains == nil {
		panic("nil dependency")
	}
	return SaveSubscriptionPageHandler{pages: pages, lists: lists, domains: domains}
}

// Handle validates the referenced lists and sending domain belong to the
// tenant, then creates or updates the page.
func (h SaveSubscriptionPageHandler) Handle(ctx context.Context,
	cmd SaveSubscriptionPage) (SaveSubscriptionPageResult, error) {

	for _, listID := range cmd.TargetListIDs {
		if _, err := h.lists.Get(ctx, cmd.TenantID, listID); err != nil {
			return SaveSubscriptionPageResult{}, err
		}
	}
	owned, err := h.domains.OwnedByTenant(ctx, cmd.TenantID, cmd.SendingDomainID)
	if err != nil {
		return SaveSubscriptionPageResult{}, err
	}
	if !owned {
		return SaveSubscriptionPageResult{}, domain.ErrSendingDomainNotFound
	}

	if cmd.PageID == "" {
		page, err := domain.NewSubscriptionPage(cmd.TenantID, cmd.Slug, cmd.Title,
			cmd.TargetListIDs, cmd.Fields, cmd.SendingDomainID, cmd.FromName, cmd.FromLocalPart)
		if err != nil {
			return SaveSubscriptionPageResult{}, err
		}
		id, err := h.pages.Add(ctx, cmd.TenantID, page)
		if err != nil {
			return SaveSubscriptionPageResult{}, err
		}
		return SaveSubscriptionPageResult{PageID: id}, nil
	}

	err = h.pages.Update(ctx, cmd.TenantID, cmd.PageID,
		func(p *domain.SubscriptionPage) (*domain.SubscriptionPage, error) {
			if err := p.Reconfigure(cmd.Slug, cmd.Title, cmd.TargetListIDs, cmd.Fields,
				cmd.SendingDomainID, cmd.FromName, cmd.FromLocalPart, cmd.Active); err != nil {
				return nil, err
			}
			return p, nil
		})
	if err != nil {
		return SaveSubscriptionPageResult{}, err
	}
	return SaveSubscriptionPageResult{PageID: cmd.PageID}, nil
}
