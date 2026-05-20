# Implementation Plan: Phase 7 — Visual Email Editor

**Branch**: `014-visual-email-editor` | **Date**: 2026-05-20 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/014-visual-email-editor/spec.md`

## Summary

Deliver a visual, block-based email editor embedded in the existing campaign
and campaign-template editors. The editor is built on TipTap core +
StarterKit (MIT) with custom email-aware extensions (Columns, Button,
Divider, Image, MergeTag, RawHTML), a custom drag handle, slash-command
suggestion menu, and bubble menu — no TipTap Pro and no paid templates.
HTML/text rendering and sanitization are **server-side at save time** so the
send pipeline reads pre-rendered `body_html` + `body_text` exactly as today
(no second send path; per FR-013, FR-013b). The structured block document
the editor produced is persisted alongside as `body_doc jsonb` so the editor
reloads losslessly (per FR-013a).

The plan also introduces the **tenant-scoped subscriber custom-field
registry** (`subscriber_fields` table) that did not previously exist on the
platform. It feeds the merge-tag picker, the Phase 6 subscription-page
"visible profile fields" picker, and the send-time placeholder substitutor.
Placeholders use namespaced double-curly syntax — `{{ subscriber.<slug> }}`
and `{{ campaign.<name> }}` — and are validated against the registry on
save (per FR-016, FR-016a–e).

Existing raw-HTML campaigns and templates from before this phase (`body_doc`
is NULL) continue to open in a code-only editor (CodeMirror) and are not
silently rewritten. Operators may explicitly opt in to convert to the
visual editor, with unconvertible regions preserved in a RawHTML block
(per FR-029, FR-030, FR-031).

Five user stories ship as five increments:

- US1, US2 (P1) — visual authoring + visual templates with the structured
  doc → HTML render path and the merge-tag chip picker.
- US3 (P2) — theme defaults derived from Phase 6 branding, per-campaign
  overrides.
- US4 (P2) — code view, RawHTML block round-tripping, opt-out to code-only,
  best-effort raw-HTML → blocks conversion.
- US5 (P2) — image insertion via the Phase 6 media picker, drag-and-drop,
  and paste-to-upload; every image src is a media-library reference.

## Technical Context

**Language/Version**: Go 1.26 (backend) / TypeScript 5.9 + React 19
(frontend). No new languages introduced.

**Primary Dependencies**:

- **Backend (new)**: `golang.org/x/net/html` (parser used by the sanitizer
  pass over rendered HTML and the legacy-HTML → blocks conversion in US4),
  `github.com/microcosm-cc/bluemonday` (HTML sanitization profile already
  used by Phase 6 for custom CSS — reused for visual-editor output).
  No new email-specific library on the server: the renderer is in-house,
  table-based, and emits inline-styled HTML directly.
- **Backend (existing, reused)**: chi router, pgx/v5, River (queue),
  testcontainers-go.
- **Frontend (new)**: `@tiptap/react`, `@tiptap/starter-kit`,
  `@tiptap/extension-bubble-menu`, `@tiptap/extension-link`,
  `@tiptap/extension-image`, `@tiptap/extension-color`,
  `@tiptap/extension-text-style`, `@tiptap/suggestion` (all MIT). Code-view
  editor: `@uiw/react-codemirror` + `@codemirror/lang-html` (MIT). All
  custom blocks (Columns, Button, Divider, MergeTag, RawHTML) and the
  drag-handle widget are implemented in-house against TipTap's MIT core —
  no `@tiptap/extension-drag-handle-pro` and no Notion-template license.
- **Frontend (existing, reused)**: TanStack Start/Router/Query/Form/Table,
  shadcn + Radix UI, Tailwind v4, lucide-react, sonner.

**Storage**: PostgreSQL via the existing tenant-plane schema. Three
additions in a single migration (000020):

1. `templates.body_doc jsonb NULL` and `campaigns.body_doc jsonb NULL` —
   the structured block document. NULL means the row was authored before
   Phase 7 or is in code-only mode.
2. `templates.theme jsonb NULL` and `campaigns.theme jsonb NULL` — explicit
   theme override (per FR-023, FR-024). NULL means "inherit tenant branding
   defaults at render time."
3. `subscriber_fields` table — tenant-scoped registry (`id`, `tenant_id`,
   `slug`, `display_name`, `type`, `default_value`, `position`,
   `created_at`, `updated_at`) with `UNIQUE (tenant_id, slug)` and RLS on
   `tenant_id`.

No new object-storage usage; the visual editor reuses the Phase 6 media
library and S3 prefix scheme as-is.

**Testing**:

- **Backend**: `go test ./...` (existing). New tests:
  - Golden-output tests in `internal/campaign/adapters/visualrender/` —
    one canonical structured doc per block type renders to a stable HTML
    string and a stable plain-text string, asserted byte-for-byte.
  - Renderer security tests — script tags, data URLs, javascript: URLs,
    and on*= handlers are stripped/refused regardless of where in the doc
    they appear.
  - Placeholder-validation tests against the registry (unknown slug fails
    save; known slug + built-in pseudo-row both pass).
  - Tenant-isolation integration tests for `subscriber_fields` CRUD and for
    visual save/load of templates and campaigns (covers Principle I).
  - Send-pipeline integration test: a campaign authored visually with
    placeholders is sent to two recipients with different field values and
    each receives the correctly-substituted HTML and plain text.
- **Frontend**: `vitest` (existing). New tests:
  - Component tests for `<VisualEmailEditor />` — slash command opens,
    insertion of each block type, drag-handle reorder, bubble-menu format,
    merge-tag chip insertion + serialization, image picker invocation,
    drag-and-drop image upload via the existing `api.media.upload` mock.
  - Route tests for template/campaign editor — switching between visual
    and code view preserves content; opening a legacy row (no `body_doc`)
    lands in code-only mode; opt-in conversion surfaces RawHTML blocks for
    unconvertible regions.
  - Component tests for the merge-tag picker — lists the tenant's registry
    rows + built-in pseudo-rows + campaign-level allow-list; respects
    typing-to-filter behavior.

**Target Platform**: Modern desktop browsers for the authoring surfaces.
The produced HTML targets Gmail (web/mobile), Apple Mail (desktop/iOS), and
Outlook (desktop/web). Multi-column layouts use table-based primitives so
Outlook desktop renders them correctly (per FR-015).

**Project Type**: Web application — Go backend extended with new commands,
queries, adapters, and a renderer; React SPA extended with the visual editor
component, a code-view editor, the subscriber-field-registry settings page,
and inline changes to the existing template/campaign editor routes.

**Performance Goals**: Interactive. Save (structured doc → HTML + text +
persist) is synchronous on the API; the renderer is pure CPU work over a
bounded document size and is expected to complete in well under 500 ms p95
for typical campaigns (≤ ~50 blocks). The send hot path is unchanged —
`cmd/worker` reads `body_html` + `body_text` and applies the existing
placeholder substitutor per recipient.

**Constraints**:

- **No TipTap Pro and no Notion-template license.** Every editor capability
  must be implementable on TipTap MIT core + StarterKit, possibly with
  in-house extensions. The custom drag handle is in-house, not the paid
  `@tiptap/extension-drag-handle-pro`.
- **No client-side rendering of email HTML.** The browser shows the visual
  surface and a desktop/mobile preview iframe that loads the *server-rendered*
  HTML — the browser never produces the canonical HTML. This keeps the
  server as the single source of truth for sanitization (Constitution IV)
  and makes the send pipeline trivially correct (Constitution VI).
- **No new send path.** `cmd/worker` continues to consume `body_html` /
  `body_text` exactly as today; only the placeholder substitutor is
  extended to accept the new namespaced syntax and to validate against the
  registry as a hard gate at *save* time, not at send time.
- **Tenant isolation is a data-layer property** for `subscriber_fields` —
  RLS on `tenant_id` plus the existing tenant-bound transaction adapter
  from `internal/db/`. Application-level filtering is defense in depth, not
  the primary control (Constitution I).
- **Backwards compatibility is mandatory.** Pre-Phase-7 rows
  (`body_doc IS NULL`) continue to work; the new endpoint/component path
  is additive. No data migration of legacy HTML into structured documents
  is performed at deploy time; conversion is per-row, opt-in, and
  best-effort (per FR-030, FR-031).

**Scale/Scope**:

- 1 migration (`000020_visual_editor_and_subscriber_fields`).
- ~1 new bounded context inside `internal/audience/` (the field registry):
  `domain/field.go`, `app/command/{create,update,delete,reorder}_field.go`,
  `app/query/list_fields.go`, `adapters/fields_postgres.go`.
- ~1 new package in `internal/campaign/adapters/visualrender/` — the
  structured-doc → HTML + plain-text renderer plus the
  HTML → structured-doc converter (US4 opt-in).
- ~2 extended commands in `internal/campaign/app/command/` —
  `save_visual_template.go` and `save_visual_campaign.go` that accept the
  structured doc, validate placeholders against the registry, render, and
  persist the three pieces atomically.
- ~1 extended substitutor at send time (extends the existing Phase 3 send
  pipeline) supporting the namespaced `{{ subscriber.<slug> }}` and
  `{{ campaign.<name> }}` syntax and a fixed allow-list of campaign
  values.
- ~8 new HTTP endpoints (registry CRUD ×4, visual save for templates ×1,
  visual save for campaigns ×1, sample-data render preview ×1, theme
  read/write folded into the campaign/template PATCH where natural).
- ~1 new SPA route (`t/$slug/settings/fields/index.tsx`) and ~2 inline
  changes to existing routes (`t/$slug/templates/$id.tsx`,
  `t/$slug/campaigns/$id.tsx`) plus the visual editor component tree under
  `frontend/src/components/visual-editor/`.
- ~1 new `Permission` union member on the frontend: `subscriber_fields:manage`
  (the registry CRUD). Campaign and template authoring inherits the
  existing `campaigns:manage` / `templates:manage` permission gating — the
  visual editor does NOT introduce a new authoring-level permission.

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` v1.1.0.*

- **I. Tenant Isolation by Default** — PASS.
  - `subscriber_fields` carries `tenant_id` from the first schema version,
    has `UNIQUE (tenant_id, slug)`, and is RLS-bound by the existing
    tenant-plane transaction wiring.
  - `templates.body_doc` / `campaigns.body_doc` live on rows that already
    carry `tenant_id`; the new columns inherit RLS automatically.
  - Visual editor saves go through the same tenant-bound repository pattern
    as the existing template/campaign repositories.
  - Test coverage: a dedicated integration test asserts that
    `subscriber_fields` rows belonging to one tenant are invisible to
    another even when the application-level filter is omitted (the same
    pattern Phase 1–6 already use).

- **II. Test-Backed Delivery** — PASS.
  - Renderer has golden-output tests per block type with byte-for-byte
    assertions on emitted HTML and plain text.
  - Sanitization has dedicated negative tests for each disallowed
    construct (script, data URLs, javascript: URLs, on*= handlers).
  - Placeholder validation is covered with integration tests across the
    save-and-send path (unknown slug rejected at save; known slug
    substituted correctly per recipient at send).
  - Frontend routes ship with colocated `*.test.tsx` covering primary
    flows, empty states, error states, and the visual ↔ code-view
    round-trip.

- **III. Incremental, Shippable Phases** — PASS.
  - The five user stories are independently demonstrable. The order of
    delivery is US1 → US2 → (US4, US5 in parallel) → US3, and each can
    ship without the next.
  - No speculative scope: TipTap Pro is excluded; collaboration is
    excluded; multi-language content is excluded; custom-extension API
    for partners is excluded.
  - Pre-Phase-7 rows continue to work — no big-bang migration of legacy
    HTML, and code-only mode remains a first-class authoring choice.

- **IV. Security & Consent by Design** — PASS.
  - Server-side sanitization is authoritative (bluemonday profile +
    additional email-specific deny rules). The frontend renderer is
    advisory; the canonical HTML is always the one the server emits.
  - Placeholder substitution is server-side at send time only — the
    editor never executes or evaluates `{{ ... }}` against real
    subscriber data.
  - Registry CRUD is gated by `subscriber_fields:manage` and audited
    through the existing audit-log path used by other tenant-plane
    mutations.
  - Image URLs in the produced HTML are required to be tenant-scoped
    media-library references; any other src in a visual doc is rejected
    at render time (per FR-021).

- **V. Operable & Observable Services** — PASS.
  - All new code is stateless. The renderer is pure CPU work inside the
    `cmd/api` request lifecycle; no new queue, no new long-running work.
  - The send pipeline in `cmd/worker` is unchanged — it continues to
    consume `body_html` + `body_text` from the row. The only worker-side
    change is extending the existing placeholder substitutor's regex /
    parser to recognize the namespaced syntax.
  - Structured logging covers the new endpoints with the standard
    `tenant_id`, `actor_id`, `request_id` fields; metrics tag the new
    save endpoints with the same labels as existing template/campaign
    save metrics.

- **VI. Layered Architecture & Domain Integrity** — PASS.
  - The new field registry lives in `internal/audience/`
    (`domain/field.go` with a validating constructor and the
    "persistence only" hydration helper; `app/command/...` and
    `app/query/...` for CQRS; `adapters/fields_postgres.go` for the
    Postgres repo that implements the command/query-owned interfaces).
  - The visual-document type and the renderer are split correctly: the
    document type lives in `internal/campaign/domain/visualdoc.go` (pure,
    no transport, no DB); the renderer lives in
    `internal/campaign/adapters/visualrender/` because it depends on the
    `golang.org/x/net/html` and `bluemonday` adapters.
  - Errors crossing domain boundaries carry typed kinds
    (`ErrInvalidPlaceholder`, `ErrUnknownSlug`, `ErrUnsupportedNode`,
    `ErrSanitizationStripped`) and are mapped to HTTP status codes in
    one place (`internal/api/...`), consistent with the existing pattern.
  - No new DI framework, no global state — composition stays in
    `cmd/api/main.go`.

**Result**: PASS — no violations, Complexity Tracking not required.

*Post-design re-check after Phase 1*: see the bottom of [data-model.md](./data-model.md)
and [contracts/](./contracts/) — design stays within the dependency rule,
introduces one new bounded context (`subscriber_fields` in
`internal/audience`), reuses the existing campaign/template aggregates
with two new typed fields, and adds one adapter package (the renderer).
Still PASS.

## Project Structure

### Documentation (this feature)

```text
specs/014-visual-email-editor/
├── plan.md              # This file
├── research.md          # Phase 0 output — tech selection, sanitization profile, conversion strategy
├── data-model.md        # Phase 1 output — registry table + body_doc/theme columns + entity changes
├── quickstart.md        # Phase 1 output — run, verify, manual-test instructions
├── contracts/           # Phase 1 output — HTTP endpoints + structured-doc JSON schema
└── tasks.md             # Phase 2 output (/speckit-tasks command — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── audience/
│   ├── domain/
│   │   └── field.go                     # NEW — Field entity (slug, display_name, type)
│   ├── app/
│   │   ├── command/
│   │   │   ├── create_field.go          # NEW
│   │   │   ├── update_field.go          # NEW
│   │   │   ├── delete_field.go          # NEW
│   │   │   └── reorder_fields.go        # NEW
│   │   ├── query/
│   │   │   └── list_fields.go           # NEW — returns registry + built-in pseudo-rows
│   │   └── application.go               # EXTENDED — wire new handlers
│   └── adapters/
│       └── fields_postgres.go           # NEW — repo implementing the command/query interfaces
├── campaign/
│   ├── domain/
│   │   ├── visualdoc.go                 # NEW — VisualDoc + Block types (paragraph, heading, columns, image, button, mergetag, rawhtml, …)
│   │   ├── theme.go                     # NEW — Theme value object (colors, fonts, container width)
│   │   ├── template.go                  # EXTENDED — accept body_doc + theme; new constructor NewVisualTemplate
│   │   └── campaign.go                  # EXTENDED — accept body_doc + theme; new constructor NewVisualCampaign
│   ├── app/
│   │   └── command/
│   │       ├── save_visual_template.go  # NEW — validate placeholders, render, persist three pieces
│   │       ├── save_visual_campaign.go  # NEW
│   │       └── render_preview.go        # NEW — server renders a doc with sample subscriber data
│   └── adapters/
│       └── visualrender/
│           ├── render.go                # NEW — VisualDoc → email-ready HTML + plain text (in-house, table-based)
│           ├── sanitize.go              # NEW — bluemonday-based + email-specific deny rules
│           ├── convert.go               # NEW — best-effort raw-HTML → VisualDoc (US4 opt-in)
│           └── placeholders.go          # NEW — extract + validate `{{ … }}` placeholders against the registry
├── sending/
│   └── domain/
│       └── substitution.go              # EXTENDED — recognize `{{ subscriber.<slug> }}` and `{{ campaign.<name> }}`
├── api/
│   └── handlers/
│       ├── subscriber_fields.go         # NEW — GET, POST, PATCH, DELETE, PATCH reorder
│       ├── templates.go                 # EXTENDED — PUT /templates/{id}/visual
│       └── campaigns.go                 # EXTENDED — PUT /campaigns/{id}/visual, POST /campaigns/{id}/render-preview
└── db/
    └── migrations/
        ├── 000020_visual_editor_and_subscriber_fields.up.sql    # NEW
        └── 000020_visual_editor_and_subscriber_fields.down.sql  # NEW

frontend/
├── src/
│   ├── components/
│   │   ├── visual-editor/                       # NEW — TipTap-based editor
│   │   │   ├── VisualEmailEditor.tsx
│   │   │   ├── extensions/
│   │   │   │   ├── Columns.tsx                  # 2/3/4-column block, serializes to table-based HTML
│   │   │   │   ├── Button.tsx
│   │   │   │   ├── Divider.tsx
│   │   │   │   ├── ImageBlock.tsx               # integrates with MediaPicker
│   │   │   │   ├── MergeTag.tsx                 # chip serializing to `{{ subscriber.<slug> }}`
│   │   │   │   └── RawHTML.tsx                  # opaque raw-HTML region
│   │   │   ├── ui/
│   │   │   │   ├── DragHandle.tsx               # in-house, MIT-only
│   │   │   │   ├── SlashCommandMenu.tsx
│   │   │   │   ├── BubbleMenu.tsx
│   │   │   │   ├── MergeTagPicker.tsx           # reads /subscriber-fields + campaign-level allow-list
│   │   │   │   └── PreviewIframe.tsx            # desktop/mobile widths
│   │   │   ├── plugins/
│   │   │   │   ├── theming.ts                   # derives defaults from Phase 6 branding
│   │   │   │   └── imageUpload.ts               # drag/paste → api.media.upload → inserts reference
│   │   │   └── theme.ts
│   │   └── code-editor/
│   │       └── CodeView.tsx                     # NEW — @uiw/react-codemirror wrapper for code-only mode
│   ├── routes/
│   │   └── t/$slug/
│   │       ├── settings/
│   │       │   └── fields/
│   │       │       ├── index.tsx                # NEW — subscriber_fields CRUD
│   │       │       └── index.test.tsx
│   │       ├── templates/
│   │       │   ├── $id.tsx                      # EXTENDED — VisualEmailEditor swap-in
│   │       │   └── $id.test.tsx                 # EXTENDED — covers visual ↔ code-view round-trip
│   │       └── campaigns/
│   │           ├── $id.tsx                      # EXTENDED — VisualEmailEditor swap-in
│   │           └── $id.test.tsx                 # EXTENDED
│   └── lib/
│       ├── api.ts                               # EXTENDED — new endpoints: subscriberFields.* , templates.saveVisual, campaigns.saveVisual, campaigns.renderPreview
│       ├── api-types.ts                         # EXTENDED — VisualDoc, Theme, Field, FieldType, MergeTagPickerItem
│       └── permissions.ts                       # EXTENDED — `subscriber_fields:manage`
```

**Structure Decision**: Web application — extend the existing Go services
(`cmd/api`, `cmd/worker`) and the existing React SPA. The visual editor is
a new component tree inside `frontend/src/components/visual-editor/`
embedded into the existing template and campaign editor routes; it is not
a separate app. The backend gains one new bounded context (the
`subscriber_fields` registry inside `internal/audience/`) and one new
adapter package (the structured-doc renderer inside
`internal/campaign/adapters/visualrender/`); the existing `Template` and
`Campaign` aggregates are extended with two new typed fields (`bodyDoc`
and `theme`) and matching validating constructors. The send pipeline in
`cmd/worker` is **unchanged** except for extending its placeholder
substitutor to recognize the namespaced syntax — this preserves
Constitution V (no new queue or stateful service) and Principle VI (the
worker keeps depending on `body_html` + `body_text` only).

## Complexity Tracking

> No constitution violations to justify. Section intentionally empty.
