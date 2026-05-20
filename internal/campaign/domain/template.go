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
//
// A template may also carry a structured visual document and an explicit
// theme override (populated by NewVisualTemplate). When bodyDoc is nil the
// row is either a legacy raw-HTML template (authored before Phase 7) or a
// code-only template the operator opted out of the visual editor on. When
// theme is nil the renderer inherits the tenant's Phase 6 branding defaults
// at render time.
type Template struct {
	id        string
	tenantID  string
	name      string
	kind      Kind
	subject   string
	bodyHTML  string
	bodyText  string
	bodyDoc   *VisualDoc
	theme     *Theme
	warnings  []RenderWarning
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
// only — it performs no validation. bodyDoc and theme are nil for legacy
// raw-HTML rows that predate Phase 7 or for rows where the operator opted
// out of the visual editor.
func HydrateTemplate(id, tenantID, name string, kind Kind, subject, bodyHTML, bodyText string,
	bodyDoc *VisualDoc, theme *Theme,
	createdAt, updatedAt time.Time) *Template {
	return &Template{
		id: id, tenantID: tenantID, name: name, kind: kind,
		subject: subject, bodyHTML: bodyHTML, bodyText: bodyText,
		bodyDoc: bodyDoc, theme: theme,
		createdAt: createdAt, updatedAt: updatedAt,
	}
}

// NewVisualTemplate builds a template authored visually. It validates the
// document against the supplied registry and media-ref rules, renders it to
// HTML and plain text using the supplied renderer, and returns the populated
// aggregate together with any non-fatal warnings the renderer emitted (for
// example, content the sanitizer stripped from a RawHTML block).
//
// pinnedTheme is the operator's explicit theme override and may be nil —
// when nil the row persists a NULL theme and inherits tenant branding at
// future render time. effectiveTheme is the value the renderer uses NOW
// (callers resolve nil pinnedTheme to DefaultsFromBranding before invoking).
//
// The three pieces of content (body_doc, body_html, body_text) end up on
// the aggregate together — there is no path that produces fewer than three.
func NewVisualTemplate(
	tenantID, name string, kind Kind, subject string,
	doc *VisualDoc, pinnedTheme *Theme, effectiveTheme Theme,
	renderer Renderer, fields FieldSet, mediaRefs MediaRefValidator,
) (*Template, []RenderWarning, error) {
	if tenantID == "" {
		return nil, nil, ErrTemplateInvalid.WithMessage("a tenant is required")
	}
	if !validKind(kind) {
		return nil, nil, ErrTemplateInvalid.WithMessage("kind must be campaign or transactional")
	}
	name = strings.TrimSpace(name)
	subject = strings.TrimSpace(subject)
	if name == "" {
		return nil, nil, ErrTemplateInvalid.WithMessage("template name is required")
	}
	if subject == "" {
		return nil, nil, ErrTemplateInvalid.WithMessage("template subject is required")
	}
	if doc == nil {
		return nil, nil, ErrVisualDocInvalid.WithMessage("document is required")
	}
	if renderer == nil {
		return nil, nil, ErrTemplateInvalid.WithMessage("renderer is required")
	}
	if err := Validate(doc, ValidateContext{Fields: fields, MediaRefs: mediaRefs}); err != nil {
		return nil, nil, err
	}
	html, text, warnings, err := renderer.Render(doc, effectiveTheme)
	if err != nil {
		return nil, nil, err
	}
	return &Template{
		tenantID: tenantID, name: name, kind: kind,
		subject: subject, bodyHTML: html, bodyText: text,
		bodyDoc: doc, theme: pinnedTheme, warnings: warnings,
	}, warnings, nil
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

// BodyDoc returns the structured visual document, or nil for legacy
// raw-HTML / code-only templates.
func (t *Template) BodyDoc() *VisualDoc { return t.bodyDoc }

// Theme returns the explicit theme override, or nil when the row inherits
// tenant Phase 6 branding defaults at render time.
func (t *Template) Theme() *Theme { return t.theme }

// RenderWarnings returns the non-fatal warnings emitted by the most recent
// NewVisualTemplate construction. Empty for hydrated rows.
func (t *Template) RenderWarnings() []RenderWarning { return t.warnings }

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
