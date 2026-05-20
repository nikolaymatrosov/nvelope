# Phase 1 — HTTP Contracts

**Feature**: Phase 7 — Visual Email Editor
**Branch**: `014-visual-email-editor`
**Date**: 2026-05-20

All endpoints are tenant-plane (mounted under `/t/{slug}/api/…` in the
actual route tree; the path prefix shown below uses `/api/v1/t/{slug}/`
as documentation shorthand) and require an authenticated session with
the relevant permission. Errors follow the existing platform error
envelope; new typed kinds are listed under each endpoint.

**Hosting tier note** (revised 2026-05-20 — see
[../brainstorm-bff-render.md](../brainstorm-bff-render.md) and
[../research.md § R4](../research.md)): the three visual-editor
endpoints — `PUT /campaigns/{id}/visual`, `PUT /templates/{id}/visual`,
and `POST /render-preview` (tenant-scoped, shared by both editors per
the 2026-05-20 N4 clarification) — are **hosted by the
TanStack Start + Nitro BFF**, not by `cmd/api`. The BFF intercepts
those paths before the catch-all proxy, renders the structured document
to email-ready HTML via `@react-email/components`, and (for the save
endpoints) forwards the rendered HTML + plain-text alongside the
document to the Go API for validation, sanitization, and persistence.
Every other endpoint below is Go-hosted and the BFF transparently
proxies to it. The browser sees a single uniform URL space and
authenticates via the same session cookie regardless of which tier
ultimately serves the request.

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

**Hosted by**: BFF (Nitro). Intercepted before the catch-all proxy to
Go. The BFF renders, then forwards to the Go-internal endpoint of the
same path with an augmented body (see § Internal BFF→Go body shape
below).

**Permission**: `templates:manage`. The BFF reads the session cookie,
forwards it to Go on the side-call to `GET /subscriber-fields` (for
placeholder validation) and on the eventual save call. Go enforces the
permission.

**Purpose**: persist a visual-editor template. The browser body carries
the **structured document only**; the BFF renders to HTML and plain
text, then the Go API revalidates placeholders against the registry,
sanitizes the rendered HTML, and persists all three pieces atomically
(per FR-013b).

**Browser → BFF body**:

```json
{
  "name":              "Welcome series — week 1",
  "kind":              "campaign",
  "subject":           "Welcome, {{ subscriber.first_name }}",
  "bodyDoc":           { "version": 1, "type": "doc", "content": [ /* blocks */ ] },
  "theme":             { "textColor": "#222222", "linkColor": "#0066cc", "buttonColor": "#0066cc", "buttonTextColor": "#ffffff", "fontFamily": "'Inter', sans-serif", "containerWidth": 600 },
  "ifUnmodifiedSince": "2026-05-20T12:34:56.123456Z"
}
```

`theme` may be omitted or null ⇒ row's theme stays NULL ⇒ render
inherits tenant branding (per FR-022). When `theme` is null the BFF
fetches `GET /branding` from Go (cookie-forwarded) to resolve the
effective theme for rendering only; the persisted theme column stays
NULL so future branding changes propagate to the row on next save.

`ifUnmodifiedSince` is the row's `updated_at` at the time the editor
loaded it (per FR-009). The SPA reads it from the row's GET response
and echoes it back on every save. After `409 stale_row` the SPA
re-fetches and copies the new `updated_at` into the next save (the
"Force overwrite" affordance does this).

**Internal BFF → Go body** (server-internal shape, not exposed to the
browser):

```json
{
  "name":              "Welcome series — week 1",
  "kind":              "campaign",
  "subject":           "Welcome, {{ subscriber.first_name }}",
  "bodyDoc":           { "version": 1, "type": "doc", "content": [ /* blocks */ ] },
  "bodyHtml":          "<table role=\"presentation\" …>…</table>",
  "bodyText":          "Welcome, {{ subscriber.first_name }}\n…",
  "theme":             { /* echoed or null */ },
  "ifUnmodifiedSince": "2026-05-20T12:34:56.123456Z"
}
```

Go rejects this internal-shape request with `400 invalid_body` if
either `bodyHtml` or `bodyText` is empty — the BFF is the only
legitimate caller and must always supply both. `ifUnmodifiedSince` is
mandatory; if absent Go also returns `400 invalid_body` (the BFF MUST
forward what the SPA sent).

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
- `400 invalid_body` — missing `bodyHtml`, `bodyText`, or
  `ifUnmodifiedSince` on the BFF→Go internal body.
- `403 forbidden` — caller lacks `templates:manage`.
- `404 not_found` — template id does not belong to the tenant.
- `409 stale_row` — `ifUnmodifiedSince` does not match the row's
  current `updated_at`; the template was changed in another
  tab/session since the editor loaded it (per FR-009). Response
  payload: `{ "kind": "stale_row", "currentUpdatedAt":
  "2026-05-20T12:35:01.987654Z" }` so the SPA can show the "Reload /
  Force overwrite" affordance.
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

**Hosted by**: BFF (Nitro). Same orchestration shape as the template
save endpoint above.

**Permission**: `campaigns:manage`.

**Browser → BFF body**:

```json
{
  "subject":           "Subject line, may include {{ subscriber.first_name }}",
  "bodyDoc":           { "version": 1, "type": "doc", "content": [ /* blocks */ ] },
  "theme":             { /* optional override */ },
  "ifUnmodifiedSince": "2026-05-20T12:34:56.123456Z"
}
```

**Internal BFF → Go body** adds the rendered `bodyHtml` and `bodyText`
fields and forwards `ifUnmodifiedSince` (same shape as the templates
internal body above).

**Response**: same shape as the templates response above.

**Errors**: same set of typed kinds as the templates endpoint
(including `409 stale_row` per FR-009). New BFF-emitted code:

- `502 bad_gateway` — BFF cannot reach Go for the
  `GET /subscriber-fields` or `GET /branding` side-call required to
  validate or render. Fail-closed semantics per the 2026-05-20
  clarification.

### `PUT /api/v1/t/{slug}/campaigns/{id}` (existing, unchanged)

Still accepts a raw-HTML body for code-only authoring.

---

## Render preview (shared by campaign + template editors)

### `POST /api/v1/t/{slug}/render-preview`

**Hosted by**: BFF (Nitro). Tenant-scoped, **not** row-scoped — the
endpoint accepts a `bodyDoc` directly and never reads a campaign or
template row, so one route serves both editors per the 2026-05-20
N4 clarification. The render step never reaches Go. When the caller
supplies a `sample` object, the BFF side-calls Go's
`POST /substitute-sample` endpoint (see below) for placeholder
resolution — the BFF MUST NOT reimplement substitution rules in
TypeScript (FR-016 / [research.md § R12b](../research.md)). Flow:
validate the doc → fetch `GET /branding` from Go if `theme` is null
→ render the doc with `@react-email/components` → if `sample` is
supplied, POST `{ html, text, sampleSubscriber, sampleCampaign }` to
Go's `POST /substitute-sample` and use the response — finally
sanitize the resulting HTML (BFF preview-only sanitizer) and return.
The BFF runs its own sanitization pass over the previewed HTML
(preview-only — never persisted) and emits its own warnings for
content that would be stripped (per FR-014a).

**Permission**: any tenant member who can edit campaigns or templates
— effectively `campaigns:manage` OR `templates:manage`. The BFF
forwards the cookie and Go applies the gate (either permission
grants access).

**Purpose**: render the supplied (unsaved) structured document on the
server for the editor's desktop/mobile preview iframe (600 px / 375 px
per FR-007). Optionally substitutes a caller-supplied sample
subscriber so the operator sees placeholders resolved with realistic
values.

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
`unknown_placeholder`, `invalid_media_ref`). New BFF-emitted code:

- `502 bad_gateway` — BFF cannot reach Go for the
  `GET /subscriber-fields`, `GET /branding`, or
  `POST /substitute-sample` side-call. Fail-closed.

---

## Sample-data placeholder substitution (Go-side helper for the BFF)

### `POST /api/v1/t/{slug}/substitute-sample`

**Hosted by**: Go (`cmd/api`). Reached only by the BFF's
render-preview route; not exposed to the SPA directly. The route is
tenant-scoped and reuses the same session-cookie authentication as
every other tenant-plane endpoint.

**Permission**: `campaigns:manage` OR `templates:manage` (the BFF
forwards the cookie of the user authoring the preview; Go enforces
the same gate that `POST /render-preview` applies — either permission
grants access since the endpoint is shared by both editors).

**Purpose**: resolve `{{ subscriber.<slug> }}` and `{{ campaign.<name> }}`
placeholders in already-rendered HTML/text by feeding the supplied
sample values through the canonical send-pipeline substituter
(`internal/sending/domain/substitution.go`). Single substituter
implementation; preview matches inbox.

**Body**:

```json
{
  "html": "<table …>… {{ subscriber.first_name }} …</table>",
  "text": "… {{ subscriber.first_name }} …",
  "sample": {
    "subscriber": { "first_name": "Sam", "last_name": "Rivers", "email": "sam@example.test", "country": "GB" },
    "campaign":   { "unsubscribe_url": "https://example.test/u/abc", "preference_url": "https://example.test/p/abc", "archive_url": "…", "view_in_browser_url": "…", "tenant_name": "Acme", "current_date": "2026-05-20" }
  }
}
```

**Response** `200 OK`:

```json
{
  "html": "<table …>… Sam …</table>",
  "text": "… Sam …"
}
```

This endpoint **does not persist anything** and does not write audit
rows. It is a pure transformation. Unknown subscriber slugs in
`sample` are ignored on this path (the doc itself was already
validated by the BFF before render; this endpoint only resolves
known placeholders against the supplied sample values).

**Errors**:

- `400 invalid_body` — `html`/`text` missing or `sample` malformed.
- `403 forbidden` — caller lacks `campaigns:manage`.

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

All audit events are emitted by the Go API after persistence — the
BFF does not write audit rows. `warnings_count` reflects the count of
items stripped by Go's bluemonday pass (per the 2026-05-20
clarification). Visual-save events do not include the body; the row
already holds it.
