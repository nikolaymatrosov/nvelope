package command

import (
	"context"
	"fmt"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
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
	fields  domain.FieldRepository
}

// NewSaveSubscriptionPageHandler builds the handler, failing fast on a nil
// dependency. The fields repository is the canonical source for the set of
// subscriber-field slugs a subscription-page form may reference, so the
// "visible profile fields" picker (FR-016b) shares one list with the visual
// editor's merge-tag picker.
func NewSaveSubscriptionPageHandler(pages domain.SubscriptionPageRepository,
	lists domain.ListRepository, domains domain.SendingDomainChecker,
	fields domain.FieldRepository) SaveSubscriptionPageHandler {

	if pages == nil || lists == nil || domains == nil || fields == nil {
		panic("nil dependency")
	}
	return SaveSubscriptionPageHandler{pages: pages, lists: lists, domains: domains, fields: fields}
}

// Handle validates the referenced lists and sending domain belong to the
// tenant, that every form-field key is a known subscriber-field slug from
// the registry (built-in pseudo-rows + tenant custom rows), then creates or
// updates the page.
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
	if err := h.validateFieldKeys(ctx, cmd.TenantID, cmd.Fields); err != nil {
		return SaveSubscriptionPageResult{}, err
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

// validateFieldKeys rejects any form-field key that isn't a known
// subscriber-field slug for the tenant. The merged allow-list is the
// built-in pseudo-rows plus the tenant's custom field registry — the same
// set the visual editor's merge-tag picker reads (FR-016b).
func (h SaveSubscriptionPageHandler) validateFieldKeys(ctx context.Context,
	tenantID string, formFields []domain.FormField) error {

	if len(formFields) == 0 {
		return nil
	}
	known := map[string]struct{}{}
	for _, slug := range domain.BuiltinFieldSlugs() {
		known[slug] = struct{}{}
	}
	custom, err := h.fields.All(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, f := range custom {
		known[f.Slug()] = struct{}{}
	}
	for _, f := range formFields {
		if _, ok := known[f.Key]; !ok {
			return apperr.NewIncorrectInput("validation_failed",
				fmt.Sprintf("form-field key %q is not a known subscriber field", f.Key))
		}
	}
	return nil
}
