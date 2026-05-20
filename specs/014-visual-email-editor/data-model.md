# Phase 1 — Data Model

**Feature**: Phase 7 — Visual Email Editor
**Branch**: `014-visual-email-editor`
**Date**: 2026-05-20

This document derives concrete data shapes from
[spec.md](./spec.md) Key Entities and Requirements. Decisions taken in
[research.md](./research.md) (especially R3, R4, R7, R8, R10) drive the
column choices below.

## Schema delta (migration `000020_visual_editor_and_subscriber_fields`)

### `subscriber_fields` (NEW)

```sql
CREATE TABLE subscriber_fields (
    id            uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid          NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    slug          text          NOT NULL,
    display_name  text          NOT NULL,
    type          text          NOT NULL CHECK (type IN ('text','number','date','boolean','url')),
    default_value text,
    position      integer       NOT NULL DEFAULT 0,
    created_at    timestamptz   NOT NULL DEFAULT now(),
    updated_at    timestamptz   NOT NULL DEFAULT now(),

    UNIQUE (tenant_id, slug),
    CHECK (slug ~ '^[a-z][a-z0-9_]{0,62}$'),
    CHECK (length(display_name) BETWEEN 1 AND 128)
);

CREATE INDEX subscriber_fields_tenant_position_idx
    ON subscriber_fields (tenant_id, position);

ALTER TABLE subscriber_fields ENABLE ROW LEVEL SECURITY;
CREATE POLICY subscriber_fields_tenant ON subscriber_fields
    USING (tenant_id = current_setting('app.tenant_id')::uuid);
```

Notes:

- `slug` is the placeholder key — operators write
  `{{ subscriber.<slug> }}` in campaign/template content.
- The slug regex is intentionally strict: starts with a lowercase
  letter, then lowercase letters / digits / underscores, max 63 chars.
  Mirrors PostgreSQL identifier rules so we can safely embed slugs in
  generated SQL fragments for segmentation later.
- Built-in subscriber fields (`email`, `name`, `first_name`,
  `last_name`, `state`) are **not** rows in this table — they are
  surfaced as pseudo-rows by the query layer (per R7) so a tenant
  cannot delete or rename them.
- `default_value` is plain text; the application layer interprets it
  per `type`.
- RLS policy uses the existing `app.tenant_id` GUC set by the
  tenant-bound transaction adapter — same pattern as every other
  tenant-plane table.

### `templates.body_doc`, `templates.theme` (NEW columns)

```sql
ALTER TABLE templates
    ADD COLUMN body_doc jsonb,
    ADD COLUMN theme    jsonb;
```

- `body_doc` is the structured block document (see "Structured
  document JSON" below). NULL ⇒ legacy raw-HTML row or code-only
  authoring; the editor opens such rows in CodeView (per FR-030).
- `theme` is an explicit theme override (see "Theme JSON" below).
  NULL ⇒ inherit tenant Phase 6 branding defaults at render time
  (per FR-022, FR-024).
- `body_html` and `body_text` (existing columns from migration 000009)
  remain the canonical send-pipeline input — populated by the server
  renderer on every visual save (per R3).

### `campaigns.body_doc`, `campaigns.theme` (NEW columns)

```sql
ALTER TABLE campaigns
    ADD COLUMN body_doc jsonb,
    ADD COLUMN theme    jsonb;
```

Same semantics as the template columns.

### `iam.permissions` (data — new permission string)

The new permission `subscriber_fields:manage` is inserted into the
existing permissions seed; the existing role-grants migration pattern
applies (admin role gets it by default). No schema change.

## Domain entities (Go, `internal/...`)

### `audience.Field` (NEW, `internal/audience/domain/field.go`)

```go
type FieldType string

const (
    FieldTypeText    FieldType = "text"
    FieldTypeNumber  FieldType = "number"
    FieldTypeDate    FieldType = "date"
    FieldTypeBoolean FieldType = "boolean"
    FieldTypeURL     FieldType = "url"
)

type Field struct {
    id           string
    tenantID     string
    slug         string
    displayName  string
    fieldType    FieldType
    defaultValue string  // free-form, interpreted per fieldType
    position     int
    builtIn      bool    // true for the email/name/first_name/last_name/state pseudo-rows
    createdAt    time.Time
    updatedAt    time.Time
}

// NewField is the validating constructor. Rejects invalid slug, empty
// display name, unknown type, and any attempt to set builtIn=true.
func NewField(tenantID, slug, displayName string, t FieldType, defaultValue string, position int) (*Field, error)

// HydrateField — persistence only, NOT a constructor.
func HydrateField(id, tenantID, slug, displayName string, t FieldType, defaultValue string, position int, builtIn bool, createdAt, updatedAt time.Time) *Field
```

Invariants enforced by `NewField`:

- `tenantID != ""`
- `slug` matches `^[a-z][a-z0-9_]{0,62}$`
- `displayName` length in `[1, 128]`
- `fieldType` ∈ {text, number, date, boolean, url}
- `builtIn` is always false on construction; only pseudo-row hydration
  may set it.

### `audience.BuiltInFields` (NEW)

A package-level slice the query layer prepends to the registry rows so
the picker treats built-ins and custom fields uniformly:

| slug         | display_name | type    | builtIn |
|--------------|--------------|---------|---------|
| `email`      | Email        | url *(rendered as text)* | true |
| `first_name` | First name   | text    | true    |
| `last_name`  | Last name    | text    | true    |
| `name`       | Full name    | text    | true    |
| `state`      | State        | text    | true    |

(`email`'s URL-ness is a Phase-3 send-pipeline concern, not a
display-time format.)

### `campaign.VisualDoc` (NEW, `internal/campaign/domain/visualdoc.go`)

The structured block document is a typed Go tree, not opaque JSON. It
is parsed from `body_doc` on save and validated before render.

```go
type VisualDoc struct {
    Version int     // schema version, currently 1
    Nodes   []Node  // top-level blocks in document order
}

type Node interface{ visualNode() }

type Paragraph struct { Children []Inline }
type Heading   struct { Level int; Children []Inline }   // Level ∈ {1,2,3}
type BulletList struct { Items []ListItem }
type OrderedList struct { Items []ListItem }
type ListItem struct { Children []Node }
type Quote   struct { Children []Node }
type Code    struct { Text string }
type Image   struct { MediaRef string; Alt string; Href string }    // MediaRef MUST resolve to a tenant media URL
type Button  struct { Label string; Href string }
type Divider struct{}
type Columns struct { Cols [][]Node }                                // len(Cols) ∈ {2,3,4}; each Cols[i] is the column's blocks
type RawHTML struct { HTML string }                                  // opaque region; renderer passes through after sanitization

type Inline interface{ visualInline() }

type Text     struct { Text string; Marks Marks }
type MergeTag struct { Namespace string; Key string }                // Namespace ∈ {"subscriber","campaign"}

type Marks struct {
    Bold      bool
    Italic    bool
    Underline bool
    Strike    bool
    Color     string         // CSS color or empty
    Link      string         // href or empty
}
```

Invariants enforced by the `Validate(*VisualDoc, ValidateContext)` function:

- `Heading.Level ∈ {1, 2, 3}`.
- `Columns.Cols` has length 2, 3, or 4; each inner slice is itself a
  valid block stream.
- `Image.MediaRef` MUST match the tenant media URL pattern (per
  FR-021); validation fails otherwise.
- `MergeTag.Namespace ∈ {"subscriber", "campaign"}`. For
  `"subscriber"`, `Key` MUST resolve in the tenant's registry
  (built-in or custom). For `"campaign"`, `Key` MUST be in the
  platform allow-list.
- `RawHTML.HTML` is bounded in length and must survive the
  sanitization pass without error.

### `campaign.Theme` (NEW, `internal/campaign/domain/theme.go`)

```go
type Theme struct {
    TextColor       string  // CSS color
    LinkColor       string
    ButtonColor     string
    ButtonTextColor string
    FontFamily      string
    ContainerWidth  int     // pixels
}

// NewTheme validates the colors are CSS-safe and width ∈ [320, 800].
func NewTheme(textColor, linkColor, buttonColor, buttonTextColor, fontFamily string, containerWidth int) (*Theme, error)

// HydrateTheme — persistence only.
func HydrateTheme(...) *Theme

// DefaultsFromBranding builds a Theme from the row's tenant Phase 6
// branding. Used at render time when the row's theme is nil.
func DefaultsFromBranding(b branding.Branding) Theme
```

### Extended `campaign.Template` and `campaign.Campaign`

Both gain:

```go
// BodyDoc returns the structured document; nil for legacy/code-only rows.
func (t *Template) BodyDoc() *VisualDoc

// Theme returns the explicit override; nil ⇒ inherit branding at render time.
func (t *Template) Theme() *Theme
```

and a new validating constructor:

```go
// NewVisualTemplate builds a template authored visually. Renders body_doc
// using the supplied renderer, validates placeholders against the supplied
// fieldset, sanitizes, and returns the populated aggregate ready to persist.
//
// All three pieces of content (body_doc, body_html, body_text) end up on
// the aggregate together — there is no path that produces fewer than three.
func NewVisualTemplate(
    tenantID, name string, kind Kind, subject string,
    doc *VisualDoc, theme *Theme,
    renderer Renderer, fields FieldSet,
) (*Template, error)
```

and an equivalent `NewVisualCampaign(...)`.

`Renderer` and `FieldSet` are *consumer-owned interfaces*
(Constitution VI):

```go
// in internal/campaign/domain/

type Renderer interface {
    Render(doc *VisualDoc, theme Theme) (html string, text string, warnings []string, err error)
}

type FieldSet interface {
    HasSlug(slug string) bool
}
```

The Postgres adapter and the `visualrender` adapter implement these.

### `sending.Substitution` (EXTENDED)

The existing substitutor (`internal/sending/...`) gains support for the
namespaced syntax:

```go
type Substituter struct {
    fields FieldSet   // for validation in the validation path (save) — not used at send time
    // … existing dependencies …
}

// Substitute renders one recipient's view of the campaign by replacing
// every {{ subscriber.<slug> }} and {{ campaign.<name> }} placeholder
// in html and text. Built-in subscriber fields are read directly from
// the Subscriber aggregate; custom fields come from its Attributes
// map; campaign fields are computed from the campaign + recipient
// context (unsubscribe_url, preference_url, …).
func (s *Substituter) Substitute(html, text string, sub Subscriber, ctx CampaignContext) (outHTML, outText string)
```

The pre-existing un-namespaced syntax (if any) stays supported during
the transition (Constitution III — incremental delivery).

## Structured document JSON (`body_doc`)

Serialized form of `VisualDoc`. Stable, versioned, and matched by the
TipTap JSON shape so the frontend can write it directly.

Top-level:

```json
{
  "version": 1,
  "type": "doc",
  "content": [ /* blocks */ ]
}
```

Block examples:

```json
{ "type": "paragraph", "content": [ { "type": "text", "text": "Hello, " }, { "type": "mergeTag", "attrs": { "namespace": "subscriber", "key": "first_name" } }, { "type": "text", "text": "!" } ] }

{ "type": "heading", "attrs": { "level": 2 }, "content": [ { "type": "text", "text": "Welcome" } ] }

{ "type": "columns",
  "attrs": { "count": 2 },
  "content": [
    { "type": "column", "content": [ /* blocks */ ] },
    { "type": "column", "content": [ /* blocks */ ] }
  ] }

{ "type": "image", "attrs": { "mediaRef": "https://media.nvelope.example/tenants/abc/2026/05/logo.png", "alt": "Logo", "href": "" } }

{ "type": "button", "attrs": { "label": "Read more", "href": "https://example.com/post/42" } }

{ "type": "divider" }

{ "type": "rawHtml", "attrs": { "html": "<table>…</table>" } }
```

Inline:

```json
{ "type": "text", "text": "Bold and red", "marks": [ { "type": "bold" }, { "type": "color", "attrs": { "color": "#cc0000" } } ] }

{ "type": "mergeTag", "attrs": { "namespace": "campaign", "key": "unsubscribe_url" } }
```

## Theme JSON (`theme`)

```json
{
  "textColor": "#222222",
  "linkColor": "#0066cc",
  "buttonColor": "#0066cc",
  "buttonTextColor": "#ffffff",
  "fontFamily": "'Inter', sans-serif",
  "containerWidth": 600
}
```

When the column is NULL the API substitutes the resolved branding
defaults at render time; the row keeps NULL until the operator
explicitly pins an override (per R10 + FR-024).

## Lifecycles & state transitions

### Subscriber field

```
[draft picker] --create--> [active]
[active] --update--> [active]   (slug is immutable post-create; only display_name, type, default_value, position editable)
[active] --reorder--> [active]
[active] --delete--> [removed]  (existing subscriber attribute values keep working; merge-tag picker no longer shows the entry; placeholders referencing the deleted slug fail save in new campaigns)
```

Note: deleting a registry entry while a campaign references it is
allowed at the registry level — the campaign keeps sending until edited
because `body_html`/`body_text` are already rendered. **Editing** that
campaign after the delete will fail save with `ErrUnknownSlug` until
the placeholder is removed (per FR-016c). This is intentional —
authors get a clear, named save-time error instead of a silent
substitution gap at send time.

### Visual template / campaign

```
[draft no body_doc]  --POST /templates/{id}/visual or /campaigns/{id}/visual--> [draft with body_doc + rendered html + text]
[draft with body_doc] --PUT visual--> [draft updated]
[draft with body_doc] --opt out "edit as HTML only"--> [body_doc cleared to NULL; html/text kept; future saves go through the existing PUT endpoint]
[legacy NULL body_doc] --opt in "convert to visual"--> [convert html → doc, surface unconvertible regions as RawHTML; user then PUT visuals as normal]
```

## Constraints summary (referenced from FRs)

| Constraint                                                                 | Enforced by                                                                                      |
|----------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------|
| Three-piece storage (FR-013a)                                              | Migration adds `body_doc`/`theme`; `NewVisual*` constructors require renderer to populate all three. |
| Render server-side at save (FR-013b)                                       | Save handler in `internal/api/handlers/{templates,campaigns}.go` does the render before persist.  |
| Email-ready HTML (FR-011, FR-015)                                          | Renderer uses table-based primitives + inline styles; golden tests assert output stability.       |
| No data URLs / script (FR-014, FR-021)                                     | `sanitize.go` deny list; renderer rejects non-media-ref image src.                                |
| Placeholder syntax (FR-016)                                                | `MergeTag` node + send-time `Substituter`.                                                       |
| Registry + built-ins (FR-016a, FR-016b)                                    | `subscriber_fields` table + `BuiltInFields` pseudo-rows + `query.ListFields`.                    |
| Save-time placeholder validation (FR-016c)                                 | `NewVisualTemplate`/`NewVisualCampaign` invoke `FieldSet.HasSlug` for every placeholder; rejects on unknown. |
| Free-form attributes preserved (FR-016e)                                   | No change to subscriber `attributes` JSONB; picker derives from registry only.                   |
| Theming inheritance (FR-022, FR-024)                                       | NULL `theme` ⇒ `Theme.DefaultsFromBranding`; non-NULL ⇒ pinned override.                         |
| Code-only legacy compatibility (FR-030)                                    | NULL `body_doc` ⇒ frontend renders CodeView; visual editor never auto-converts.                  |

## Post-design constitution re-check

- I. Tenant Isolation — PASS. New table carries `tenant_id` from
  migration 1; RLS policy active; existing tenant-bound transaction
  adapter routes every query.
- II. Test-Backed — PASS. Renderer (golden), sanitizer (negative),
  placeholder validation (positive + negative), tenant-isolation
  integration tests all in scope.
- III. Incremental — PASS. Migration adds columns + table; existing
  rows stay valid (NULL columns). Each US ships standalone.
- IV. Security & Consent — PASS. Server-side sanitization +
  save-time validation + RLS on the new table.
- V. Operable & Observable — PASS. Renderer is synchronous CPU work
  in `cmd/api`; no new queue, no worker change beyond extending the
  substitutor's regex.
- VI. Layered Architecture — PASS. Domain types are pure; renderer
  is an adapter; consumer-owned `Renderer` / `FieldSet` interfaces;
  typed errors mapped to HTTP in one place.

Design ready for contracts and quickstart.
