package domain

import (
	"encoding/json"
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
	id          string
	tenantID    string
	name        string
	kind        Kind
	subject     string
	bodyHTML    string
	bodyText    string
	bodyDoc     *VisualDoc
	theme       *Theme
	bodyDocJSON json.RawMessage
	themeJSON   json.RawMessage
	warnings    []RenderWarning
	createdAt   time.Time
	updatedAt   time.Time
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
// only — it performs no validation. bodyDocJSON and themeJSON are the raw
// jsonb column bytes (nil for legacy raw-HTML rows that predate Phase 7 or
// for rows where the operator opted out of the visual editor); the typed
// bodyDoc and theme pointers are not reconstructed from the bytes — the
// editor reloads from the JSON via the query view, and save-time validation
// runs against the freshly-decoded BFF payload.
func HydrateTemplate(id, tenantID, name string, kind Kind, subject, bodyHTML, bodyText string,
	bodyDocJSON, themeJSON json.RawMessage,
	createdAt, updatedAt time.Time) *Template {
	return &Template{
		id: id, tenantID: tenantID, name: name, kind: kind,
		subject: subject, bodyHTML: bodyHTML, bodyText: bodyText,
		bodyDocJSON: normalizeRawJSON(bodyDocJSON),
		themeJSON:   normalizeRawJSON(themeJSON),
		createdAt:   createdAt, updatedAt: updatedAt,
	}
}

// NewVisualTemplate builds a template authored visually. The caller (the
// save_visual_template command) is responsible for rendering the doc to
// HTML and plain text — typically by side-calling the BFF render tier
// (see specs/014-visual-email-editor/research.md § R4) and then running
// the BFF-rendered HTML through the Go-side bluemonday sanitizer
// (internal/campaign/adapters/visualrender.Sanitize). This constructor
// revalidates the doc against the registry and media-ref rules as
// defense in depth and returns the populated aggregate with all three
// pieces (body_doc, body_html, body_text) atomically.
//
// docJSON and themeJSON are the raw wire bytes the BFF sent — persisted
// pass-through so the editor reloads losslessly from the JSON form. The
// typed doc is held only for the lifetime of this construction (for
// save-time validation); the wire bytes are what reach the row.
//
// pinnedTheme is the operator's explicit theme override and may be nil —
// when nil the row persists a NULL theme and inherits tenant branding at
// future render time. warnings are the sanitizer-emitted notes from the
// caller's sanitization pass; the aggregate carries them so the handler
// can return them to the operator in the save response.
func NewVisualTemplate(
	tenantID, name string, kind Kind, subject string,
	doc *VisualDoc, pinnedTheme *Theme,
	bodyHTML, bodyText string,
	docJSON, themeJSON json.RawMessage,
	warnings []RenderWarning,
	fields FieldSet, mediaRefs MediaRefValidator,
) (*Template, error) {
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
	if doc == nil {
		return nil, ErrVisualDocInvalid.WithMessage("document is required")
	}
	if err := Validate(doc, ValidateContext{Fields: fields, MediaRefs: mediaRefs}); err != nil {
		return nil, err
	}
	return &Template{
		tenantID: tenantID, name: name, kind: kind,
		subject: subject, bodyHTML: bodyHTML, bodyText: bodyText,
		bodyDoc: doc, theme: pinnedTheme,
		bodyDocJSON: normalizeRawJSON(docJSON),
		themeJSON:   normalizeRawJSON(themeJSON),
		warnings:    warnings,
	}, nil
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

// BodyDocJSON returns the persisted JSON bytes of the visual document, or
// nil for legacy raw-HTML / code-only templates. The bytes are the same
// shape the BFF sent on the most recent visual save — the read view
// passes them through verbatim so the editor reloads losslessly.
func (t *Template) BodyDocJSON() json.RawMessage { return t.bodyDocJSON }

// Theme returns the explicit theme override, or nil when the row inherits
// tenant Phase 6 branding defaults at render time.
func (t *Template) Theme() *Theme { return t.theme }

// ThemeJSON returns the persisted JSON bytes of the operator's pinned
// theme override, or nil when the row inherits tenant branding defaults.
func (t *Template) ThemeJSON() json.RawMessage { return t.themeJSON }

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

// OptOutVisual clears the template's structured visual document and theme
// override, leaving body_html / body_text intact so the template remains
// usable as a code-only template (per FR-029 / contracts/tenant-api.md
// "opt-out-visual"). Idempotent: calling it on a row that already has no
// body_doc is a no-op success. Templates have no draft gate — every
// template is editable like the existing Recompose method.
func (t *Template) OptOutVisual() error {
	t.bodyDoc = nil
	t.theme = nil
	t.bodyDocJSON = nil
	t.themeJSON = nil
	t.warnings = nil
	return nil
}

// ApplyVisualSave replaces a template's editable visual content with a
// validated, pre-rendered, and sanitized snapshot. The caller (the
// SaveVisualTemplate command) supplies the BFF-rendered HTML/text that
// has already been run through the Go-side sanitizer, plus any warnings
// the sanitizer emitted; this method revalidates the doc against the
// registry and media-ref rules as defense in depth and applies all
// pieces atomically.
//
// docJSON and themeJSON are the raw wire bytes the BFF sent — persisted
// pass-through so the editor reloads losslessly. Subject is required;
// the template's name and kind are preserved.
func (t *Template) ApplyVisualSave(
	subject string, doc *VisualDoc, pinnedTheme *Theme,
	bodyHTML, bodyText string,
	docJSON, themeJSON json.RawMessage,
	warnings []RenderWarning,
	fields FieldSet, mediaRefs MediaRefValidator,
) error {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ErrTemplateInvalid.WithMessage("template subject is required")
	}
	if doc == nil {
		return ErrVisualDocInvalid.WithMessage("document is required")
	}
	if err := Validate(doc, ValidateContext{Fields: fields, MediaRefs: mediaRefs}); err != nil {
		return err
	}
	t.subject = subject
	t.bodyDoc = doc
	t.theme = pinnedTheme
	t.bodyHTML = bodyHTML
	t.bodyText = bodyText
	t.bodyDocJSON = normalizeRawJSON(docJSON)
	t.themeJSON = normalizeRawJSON(themeJSON)
	t.warnings = warnings
	return nil
}
