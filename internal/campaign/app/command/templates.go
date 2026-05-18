// Package command holds the campaign context's state-changing handlers, named
// in business language.
package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// CreateTemplate is the request to create a template.
type CreateTemplate struct {
	TenantID string
	Name     string
	Kind     string
	Subject  string
	BodyHTML string
	BodyText string
}

// CreateTemplateResult carries the new template's id.
type CreateTemplateResult struct {
	TemplateID string
}

// CreateTemplateHandler handles the CreateTemplate command.
type CreateTemplateHandler struct {
	templates domain.TemplateRepository
}

// NewCreateTemplateHandler builds the handler, failing fast on a nil dependency.
func NewCreateTemplateHandler(templates domain.TemplateRepository) CreateTemplateHandler {
	if templates == nil {
		panic("nil template repository")
	}
	return CreateTemplateHandler{templates: templates}
}

// Handle validates the request through the domain constructor and persists the
// new template.
func (h CreateTemplateHandler) Handle(ctx context.Context, cmd CreateTemplate) (CreateTemplateResult, error) {
	tpl, err := domain.NewTemplate(cmd.TenantID, cmd.Name, domain.Kind(cmd.Kind),
		cmd.Subject, cmd.BodyHTML, cmd.BodyText)
	if err != nil {
		return CreateTemplateResult{}, err
	}
	id, err := h.templates.Add(ctx, cmd.TenantID, tpl)
	if err != nil {
		return CreateTemplateResult{}, err
	}
	return CreateTemplateResult{TemplateID: id}, nil
}

// UpdateTemplate is the request to change a template's name and content.
type UpdateTemplate struct {
	TenantID   string
	TemplateID string
	Name       string
	Subject    string
	BodyHTML   string
	BodyText   string
}

// UpdateTemplateHandler handles the UpdateTemplate command.
type UpdateTemplateHandler struct {
	templates domain.TemplateRepository
}

// NewUpdateTemplateHandler builds the handler, failing fast on a nil dependency.
func NewUpdateTemplateHandler(templates domain.TemplateRepository) UpdateTemplateHandler {
	if templates == nil {
		panic("nil template repository")
	}
	return UpdateTemplateHandler{templates: templates}
}

// Handle applies the new template content inside the tenant-bound transaction.
func (h UpdateTemplateHandler) Handle(ctx context.Context, cmd UpdateTemplate) error {
	return h.templates.Update(ctx, cmd.TenantID, cmd.TemplateID,
		func(tpl *domain.Template) (*domain.Template, error) {
			return tpl, tpl.Recompose(cmd.Name, cmd.Subject, cmd.BodyHTML, cmd.BodyText)
		})
}

// DeleteTemplate is the request to remove a template.
type DeleteTemplate struct {
	TenantID   string
	TemplateID string
}

// DeleteTemplateHandler handles the DeleteTemplate command.
type DeleteTemplateHandler struct {
	templates domain.TemplateRepository
}

// NewDeleteTemplateHandler builds the handler, failing fast on a nil dependency.
func NewDeleteTemplateHandler(templates domain.TemplateRepository) DeleteTemplateHandler {
	if templates == nil {
		panic("nil template repository")
	}
	return DeleteTemplateHandler{templates: templates}
}

// Handle removes the template inside the tenant-bound transaction.
func (h DeleteTemplateHandler) Handle(ctx context.Context, cmd DeleteTemplate) error {
	return h.templates.Delete(ctx, cmd.TenantID, cmd.TemplateID)
}
