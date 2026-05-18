// Package query holds the campaign context's read-only handlers.
package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// TemplateView is the read model of a template.
type TemplateView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Subject   string    `json:"subject"`
	BodyHTML  string    `json:"body_html"`
	BodyText  string    `json:"body_text"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// templateView projects a Template aggregate onto its read model.
func templateView(t *domain.Template) TemplateView {
	return TemplateView{
		ID: t.ID(), Name: t.Name(), Kind: string(t.Kind()), Subject: t.Subject(),
		BodyHTML: t.BodyHTML(), BodyText: t.BodyText(),
		CreatedAt: t.CreatedAt(), UpdatedAt: t.UpdatedAt(),
	}
}

// TemplatePage is a page of templates with the total count.
type TemplatePage struct {
	Templates []TemplateView
	Total     int
}

// ListTemplates is the request for a page of the tenant's templates.
type ListTemplates struct {
	TenantID string
	Page     domain.Page
}

// ListTemplatesHandler handles the ListTemplates query.
type ListTemplatesHandler struct {
	templates domain.TemplateRepository
}

// NewListTemplatesHandler builds the handler, failing fast on a nil dependency.
func NewListTemplatesHandler(templates domain.TemplateRepository) ListTemplatesHandler {
	if templates == nil {
		panic("nil template repository")
	}
	return ListTemplatesHandler{templates: templates}
}

// Handle returns a page of the tenant's templates.
func (h ListTemplatesHandler) Handle(ctx context.Context, q ListTemplates) (TemplatePage, error) {
	templates, total, err := h.templates.All(ctx, q.TenantID, q.Page)
	if err != nil {
		return TemplatePage{}, err
	}
	page := TemplatePage{Total: total, Templates: make([]TemplateView, 0, len(templates))}
	for _, t := range templates {
		page.Templates = append(page.Templates, templateView(t))
	}
	return page, nil
}

// GetTemplate is the request for a single template.
type GetTemplate struct {
	TenantID   string
	TemplateID string
}

// GetTemplateHandler handles the GetTemplate query.
type GetTemplateHandler struct {
	templates domain.TemplateRepository
}

// NewGetTemplateHandler builds the handler, failing fast on a nil dependency.
func NewGetTemplateHandler(templates domain.TemplateRepository) GetTemplateHandler {
	if templates == nil {
		panic("nil template repository")
	}
	return GetTemplateHandler{templates: templates}
}

// Handle returns the requested template, or domain.ErrTemplateNotFound.
func (h GetTemplateHandler) Handle(ctx context.Context, q GetTemplate) (TemplateView, error) {
	t, err := h.templates.Get(ctx, q.TenantID, q.TemplateID)
	if err != nil {
		return TemplateView{}, err
	}
	return templateView(t), nil
}
