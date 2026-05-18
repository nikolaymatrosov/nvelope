package domain

import (
	"strings"
	"time"
)

// Kind is the intended use of a template — bulk campaign or transactional.
type Kind string

const (
	// KindCampaign marks a template usable for a bulk campaign send.
	KindCampaign Kind = "campaign"
	// KindTransactional marks a template usable for a transactional send.
	KindTransactional Kind = "transactional"
)

// validKind reports whether k is a known template kind.
func validKind(k Kind) bool {
	return k == KindCampaign || k == KindTransactional
}

// Template is a reusable message blueprint: a subject and body a campaign or
// transactional send is rendered from. It is a tenant-plane aggregate reached
// only through the RLS-bound transaction owned by its repository adapter.
type Template struct {
	id        string
	tenantID  string
	name      string
	kind      Kind
	subject   string
	bodyHTML  string
	bodyText  string
	createdAt time.Time
	updatedAt time.Time
}

// NewTemplate builds a template, rejecting any invariant violation.
func NewTemplate(tenantID, name string, kind Kind, subject, bodyHTML, bodyText string) (*Template, error) {
	if tenantID == "" {
		return nil, ErrTemplateInvalid.WithMessage("a tenant is required")
	}
	if !validKind(kind) {
		return nil, ErrTemplateInvalid.WithMessage("kind must be campaign or transactional")
	}
	name = strings.TrimSpace(name)
	subject = strings.TrimSpace(subject)
	if name == "" {
		return nil, ErrTemplateInvalid.WithMessage("template name is required")
	}
	if subject == "" {
		return nil, ErrTemplateInvalid.WithMessage("template subject is required")
	}
	if strings.TrimSpace(bodyHTML) == "" && strings.TrimSpace(bodyText) == "" {
		return nil, ErrTemplateInvalid.WithMessage("a template needs an HTML or text body")
	}
	return &Template{
		tenantID: tenantID, name: name, kind: kind,
		subject: subject, bodyHTML: bodyHTML, bodyText: bodyText,
	}, nil
}

// HydrateTemplate reconstructs a template from a persisted row. Persistence
// only — it performs no validation.
func HydrateTemplate(id, tenantID, name string, kind Kind, subject, bodyHTML, bodyText string,
	createdAt, updatedAt time.Time) *Template {
	return &Template{
		id: id, tenantID: tenantID, name: name, kind: kind,
		subject: subject, bodyHTML: bodyHTML, bodyText: bodyText,
		createdAt: createdAt, updatedAt: updatedAt,
	}
}

// ID returns the database-assigned id.
func (t *Template) ID() string { return t.id }

// TenantID returns the owning tenant's id.
func (t *Template) TenantID() string { return t.tenantID }

// Name returns the template name.
func (t *Template) Name() string { return t.name }

// Kind returns the template kind.
func (t *Template) Kind() Kind { return t.kind }

// Subject returns the subject line.
func (t *Template) Subject() string { return t.subject }

// BodyHTML returns the HTML body.
func (t *Template) BodyHTML() string { return t.bodyHTML }

// BodyText returns the plain-text body.
func (t *Template) BodyText() string { return t.bodyText }

// CreatedAt returns when the template was created.
func (t *Template) CreatedAt() time.Time { return t.createdAt }

// UpdatedAt returns when the template was last changed.
func (t *Template) UpdatedAt() time.Time { return t.updatedAt }

// Recompose replaces the template's name and content, rejecting an invariant
// violation. The kind is immutable.
func (t *Template) Recompose(name, subject, bodyHTML, bodyText string) error {
	updated, err := NewTemplate(t.tenantID, name, t.kind, subject, bodyHTML, bodyText)
	if err != nil {
		return err
	}
	t.name = updated.name
	t.subject = updated.subject
	t.bodyHTML = updated.bodyHTML
	t.bodyText = updated.bodyText
	return nil
}
