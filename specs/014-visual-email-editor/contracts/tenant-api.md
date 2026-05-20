# Phase 1 — HTTP Contracts

**Feature**: Phase 7 — Visual Email Editor
**Branch**: `014-visual-email-editor`
**Date**: 2026-05-20

All endpoints are tenant-plane (mounted under `/api/v1/t/{slug}/…`) and
require an authenticated session with the relevant permission. Errors
follow the existing platform error envelope; new typed kinds are listed
under each endpoint.

---

## Subscriber field registry

### `GET /api/v1/t/{slug}/subscriber-fields`

**Permission**: any tenant member (read).

**Purpose**: list the tenant's subscriber field registry, prepended with
the built-in pseudo-rows so consumers can render one uniform picker.

**Response** `200 OK`:

```json
{
  "fields": [
    { "id": "builtin:first_name", "slug": "first_name", "displayName": "First name", "type": "text",  "builtIn": true,  "defaultValue": "", "position": 0 },
    { "id": "builtin:last_name",  "slug": "last_name",  "displayName": "Last name",  "type": "text",  "builtIn": true,  "defaultValue": "", "position": 1 },
    { "id": "builtin:email",      "slug": "email",      "displayName": "Email",      "type": "url",   "builtIn": true,  "defaultValue": "", "position": 2 },
    { "id": "builtin:name",       "slug": "name",       "displayName": "Full name",  "type": "text",  "builtIn": true,  "defaultValue": "", "position": 3 },
    { "id": "builtin:state",      "slug": "state",      "displayName": "State",      "type": "text",  "builtIn": true,  "defaultValue": "", "position": 4 },
    { "id": "f_01h…",             "slug": "country",    "displayName": "Country",    "type": "text",  "builtIn": false, "defaultValue": "", "position": 0 },
    { "id": "f_01h…",             "slug": "plan_tier",  "displayName": "Plan tier",  "type": "text",  "builtIn": false, "defaultValue": "", "position": 1 }
  ]
}
```

### `POST /api/v1/t/{slug}/subscriber-fields`

**Permission**: `subscriber_fields:manage`.

**Body**:

```json
{ "slug": "country", "displayName": "Country", "type": "text", "defaultValue": "" }
```

**Response** `201 Created`: the created `Field` object.

**Errors**:

- `400 invalid_slug` — slug fails the regex.
- `400 invalid_display_name` — display name out of `[1,128]`.
- `400 invalid_type` — type not in the allowed set.
- `409 slug_taken` — `(tenant_id, slug)` already exists.
- `409 builtin_slug_reserved` — slug collides with a built-in pseudo-row
  (`email`, `name`, `first_name`, `last_name`, `state`).

### `PATCH /api/v1/t/{slug}/subscriber-fields/{id}`

**Permission**: `subscriber_fields:manage`.

**Body** (any subset, slug is immutable post-create):

```json
{ "displayName": "Country / region", "type": "text", "defaultValue": "US" }
```

**Response** `200 OK`: the updated `Field`.

**Errors**: same `400` codes as POST; `404 not_found`.

### `DELETE /api/v1/t/{slug}/subscriber-fields/{id}`

**Permission**: `subscriber_fields:manage`.

**Response** `204 No Content`.

**Errors**:

- `404 not_found`.
- `409 builtin_field` — attempt to delete a built-in pseudo-row.

### `PATCH /api/v1/t/{slug}/subscriber-fields/order`

**Permission**: `subscriber_fields:manage`.

**Body**:

```json
{ "order": ["f_01h…country", "f_01h…plan_tier", "f_01h…signup_source"] }
```

**Response** `200 OK`: `{ "fields": [ /* registry in new order */ ] }`.

**Errors**:

- `400 incomplete_order` — the supplied list does not cover every
  non-built-in field id exactly once.

---

## Merge-tag picker

### `GET /api/v1/t/{slug}/merge-tags`

**Permission**: any tenant member who can read the campaign/template
they're authoring (the endpoint itself just checks tenant membership).

**Purpose**: feed the editor's merge-tag picker with one merged list —
subscriber fields (built-in + custom) plus the platform's campaign-level
allow-list.

**Response** `200 OK`:

```json
{
  "subscriber": [
    { "slug": "first_name", "displayName": "First name", "type": "text",  "builtIn": true  },
    { "slug": "email",      "displayName": "Email",      "type": "url",   "builtIn": true  },
    { "slug": "country",    "displayName": "Country",    "type": "text",  "builtIn": false }
  ],
  "campaign": [
    { "key": "unsubscribe_url",      "displayName": "Unsubscribe URL" },
    { "key": "preference_url",       "displayName": "Preference URL" },
    { "key": "archive_url",          "displayName": "Archive URL" },
    { "key": "view_in_browser_url",  "displayName": "View in browser URL" },
    { "key": "tenant_name",          "displayName": "Tenant name" },
    { "key": "current_date",         "displayName": "Current date" }
  ]
}
```

---

## Visual save for templates

### `PUT /api/v1/t/{slug}/templates/{id}/visual`

**Permission**: `templates:manage`.

**Purpose**: persist a visual-editor template. The body carries the
**structured document only**; the server renders to HTML and plain
text, sanitizes, validates placeholders against the registry, and
persists all three pieces atomically (per FR-013b).

**Body**:

```json
{
  "name":    "Welcome series — week 1",
  "kind":    "campaign",
  "subject": "Welcome, {{ subscriber.first_name }}",
  "bodyDoc": { "version": 1, "type": "doc", "content": [ /* blocks */ ] },
  "theme":   { "textColor": "#222222", "linkColor": "#0066cc", "buttonColor": "#0066cc", "buttonTextColor": "#ffffff", "fontFamily": "'Inter', sans-serif", "containerWidth": 600 }
}
```

`theme` may be omitted or null ⇒ row's theme stays NULL ⇒ render
inherits tenant branding (per FR-022).

**Response** `200 OK`:

```json
{
  "id": "t_01h…",
  "name": "Welcome series — week 1",
  "kind": "campaign",
  "subject": "Welcome, {{ subscriber.first_name }}",
  "bodyHtml": "<table role=\"presentation\" …>…</table>",
  "bodyText": "Welcome, {{ subscriber.first_name }}\n\n…",
  "bodyDoc":  { "version": 1, "type": "doc", "content": [/* unchanged */] },
  "theme":    { /* echoed or null */ },
  "warnings": [
    { "kind": "sanitizer_stripped", "detail": "Removed <script> tag in raw HTML block at content[3]" }
  ],
  "updatedAt": "2026-05-20T12:34:56Z"
}
```

**Errors** (typed kinds map to HTTP status in one place per
Constitution VI):

- `400 invalid_doc` — JSON does not parse to a valid `VisualDoc`
  (unknown node type, bad column count, missing required attrs).
- `400 unknown_placeholder` — payload includes:
  `{ "kind": "unknown_placeholder", "placeholders": ["subscriber.first_naem"] }`
  — operator typo'd a slug not in the registry.
- `400 invalid_media_ref` — an image block's `mediaRef` is not a
  tenant media URL.
- `400 invalid_subject` — empty or > 998 chars.
- `403 forbidden` — caller lacks `templates:manage`.
- `404 not_found` — template id does not belong to the tenant.
- `422 sanitization_blocked` — the sanitizer would have stripped so
  much from the rendered output that the result is empty or
  ill-formed (rare; typically caused by a RawHTML block that is
  entirely `<script>`).

### `PUT /api/v1/t/{slug}/templates/{id}` (existing, unchanged)

Still accepts a raw-HTML body for code-only authoring; clears
`body_doc` to NULL when called.

---

## Visual save for campaigns

### `PUT /api/v1/t/{slug}/campaigns/{id}/visual`

**Permission**: `campaigns:manage`.

**Body**:

```json
{
  "subject": "Subject line, may include {{ subscriber.first_name }}",
  "bodyDoc": { "version": 1, "type": "doc", "content": [ /* blocks */ ] },
  "theme":   { /* optional override */ }
}
```

**Response**: same shape as the templates response above.

**Errors**: same set of typed kinds.

### `PUT /api/v1/t/{slug}/campaigns/{id}` (existing, unchanged)

Still accepts a raw-HTML body for code-only authoring.

---

## Render preview

### `POST /api/v1/t/{slug}/campaigns/{id}/render-preview`

**Permission**: `campaigns:manage` (caller is the campaign author).

**Purpose**: render the supplied (unsaved) structured document on the
server for the editor's desktop/mobile preview iframe. Optionally
substitutes a caller-supplied sample subscriber so the operator sees
placeholders resolved with realistic values.

**Body**:

```json
{
  "bodyDoc": { /* unsaved doc */ },
  "theme":   { /* optional override */ },
  "sample":  {
    "subscriber": { "first_name": "Sam", "last_name": "Rivers", "email": "sam@example.test", "country": "GB" },
    "campaign":   { "unsubscribe_url": "https://example.test/u/abc", "preference_url": "https://example.test/p/abc", "view_in_browser_url": "…", "tenant_name": "Acme", "current_date": "2026-05-20" }
  }
}
```

`sample` is optional; when absent, placeholders are rendered as their
literal `{{ … }}` strings for an "unsubstituted" preview.

**Response** `200 OK`:

```json
{
  "bodyHtml": "<!doctype html><html>…",
  "bodyText": "…",
  "warnings": []
}
```

This endpoint **does not persist anything**.

**Errors**: same as the save endpoints (`invalid_doc`,
`unknown_placeholder`, `invalid_media_ref`).

---

## Opt-in: convert legacy raw-HTML to visual

### `POST /api/v1/t/{slug}/templates/{id}/convert-to-visual`

**Permission**: `templates:manage`.

**Purpose**: best-effort conversion of a legacy raw-HTML template into
a `VisualDoc`. Unconvertible regions land in `RawHTML` blocks (per
FR-031). The endpoint does not persist — it returns the converted
document so the operator can review in the editor before saving with
the visual `PUT`.

**Body**: empty.

**Response** `200 OK`:

```json
{
  "bodyDoc": { /* converted document */ },
  "warnings": [
    { "kind": "rawhtml_block", "detail": "<table> with rowspan was preserved verbatim", "path": "content[5]" }
  ]
}
```

**Errors**:

- `409 already_visual` — template already has a non-NULL `body_doc`.
- `400 unconvertible_html` — the existing HTML cannot even be parsed.

### `POST /api/v1/t/{slug}/campaigns/{id}/convert-to-visual`

Mirror of the template endpoint.

### `POST /api/v1/t/{slug}/templates/{id}/opt-out-visual` and `/campaigns/{id}/opt-out-visual`

**Permission**: `templates:manage` / `campaigns:manage`.

**Purpose**: opt out of the visual editor on a row (per FR-029). Clears
`body_doc` to NULL; `body_html` and `body_text` stay intact so the
campaign remains sendable as a code-only campaign.

**Response** `200 OK`: the updated row.

---

## Structured-document JSON schema (informal)

Authoritative grammar:

```
Doc        := { version:int, type:"doc", content:Block[] }
Block      := Paragraph | Heading | BulletList | OrderedList | Quote | Code | Image | Button | Divider | Columns | RawHtml
Paragraph  := { type:"paragraph", content:Inline[] }
Heading    := { type:"heading", attrs:{ level: 1|2|3 }, content:Inline[] }
BulletList := { type:"bulletList",  content:ListItem[] }
OrderedList:= { type:"orderedList", content:ListItem[] }
ListItem   := { type:"listItem", content:Block[] }
Quote      := { type:"blockquote", content:Block[] }
Code       := { type:"codeBlock", content:[{type:"text",text:string}] }
Image      := { type:"image",  attrs:{ mediaRef:string, alt:string, href:string } }
Button     := { type:"button", attrs:{ label:string, href:string } }
Divider    := { type:"divider" }
Columns    := { type:"columns", attrs:{ count: 2|3|4 }, content: Column[] }   // exactly `count` columns
Column     := { type:"column",  content:Block[] }
RawHtml    := { type:"rawHtml", attrs:{ html:string } }

Inline     := Text | MergeTag
Text       := { type:"text", text:string, marks?: Mark[] }
MergeTag   := { type:"mergeTag", attrs:{ namespace: "subscriber"|"campaign", key:string } }

Mark       := { type:"bold" }
            | { type:"italic" }
            | { type:"underline" }
            | { type:"strike" }
            | { type:"color", attrs:{ color:string } }
            | { type:"link",  attrs:{ href:string } }
```

Validation rules beyond the grammar (enforced by
`internal/campaign/domain/visualdoc.Validate`):

- `Columns.content` length equals `Columns.attrs.count` exactly.
- `Image.attrs.mediaRef` matches the tenant media URL pattern.
- `MergeTag.attrs.key` for namespace `"subscriber"` MUST resolve in
  the tenant registry (built-in or custom).
- `MergeTag.attrs.key` for namespace `"campaign"` MUST be in the
  platform allow-list.
- `Mark.color.color` is a CSS-safe color string (`#RRGGBB`,
  `#RRGGBBAA`, `rgb(…)`, named).
- `Mark.link.href` scheme ∈ {`http`,`https`,`mailto`,`tel`} and is
  non-empty.

---

## Audit events (additive)

Existing audit log gains five new event kinds, emitted by the handlers
above:

- `subscriber_field.create` — `{ id, slug, type }`.
- `subscriber_field.update` — `{ id, before, after }` with diffed fields.
- `subscriber_field.delete` — `{ id, slug }`.
- `subscriber_field.reorder` — `{ orderBefore[], orderAfter[] }`.
- `template.save_visual` — `{ template_id, warnings_count }`.
- `campaign.save_visual` — `{ campaign_id, warnings_count }`.

(Visual-save events do not include the body; the row already holds it.)
