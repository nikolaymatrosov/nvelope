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

- **Backend (new)**: `golang.org/x/net/html` (parser used by the
  legacy-HTML → blocks conversion in US4),
  `github.com/microcosm-cc/bluemonday` (HTML sanitization profile already
  used by Phase 6 for custom CSS — reused for the Go-side sanitizer pass
  over the BFF-rendered HTML before persistence). The Go API does **not**
  host the email-HTML renderer; rendering moved to the BFF per
  [research.md § R4](./research.md) (revised 2026-05-20).
- **Backend (existing, reused)**: chi router, pgx/v5, River (queue),
  testcontainers-go.
- **BFF (new)**: `@react-email/components` + `@react-email/render`
  (MIT) — used by the Nitro server routes that intercept the visual
  save and preview endpoints and produce the canonical email HTML +
  plain-text. A small TypeScript HTML sanitizer (`isomorphic-dompurify`
  or `sanitize-html`) for the BFF-side preview-output cleanup (preview
  warnings come from this; save warnings come from Go's bluemonday).
- **BFF (existing, reused)**: TanStack Start + Nitro (already the SPA
  host; today proxies `/api/*` and `/t/{slug}/api/*` to Go at `:8080`);
  gains two new server routes for visual save and render-preview.
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

- **Backend (Go)**: `go test ./...` (existing). New tests:
  - Sanitizer security tests in
    `internal/campaign/adapters/visualrender/sanitize_test.go` — script
    tags, data URLs, javascript:/vbscript: URLs, and on*= handlers are
    stripped/refused regardless of where in the BFF-supplied HTML they
    appear. The Go-side sanitizer is the authoritative gate before
    persistence (FR-014, FR-014a).
  - Doc-revalidation tests — the Go save handler re-validates the doc
    (defense in depth) and re-runs placeholder extraction against the
    registry; tests cover the case where the BFF and Go validators
    might drift (which should never happen but is recovered if so).
  - Tenant-isolation integration tests for `subscriber_fields` CRUD and
    for visual save/load of templates and campaigns (covers Principle I).
  - Send-pipeline integration test: a campaign authored visually with
    placeholders is sent to two recipients with different field values
    and each receives the correctly-substituted HTML and plain text.
  - **REMOVED** vs prior plan: the byte-for-byte renderer golden tests
    move from Go to TypeScript (now run against react-email's output).
- **BFF (new TypeScript test surface)**: `vitest` (existing harness).
  New tests:
  - Render golden tests in `frontend/src/server/render/` — one canonical
    doc per block type plus every mark combination, asserted
    byte-for-byte against fixture files. react-email + react-email
    versions are exact-pinned so fixture stability is achievable;
    fixture-update PRs are the expected churn vector on minor upgrades.
  - Validator unit tests in `frontend/src/server/validate/` — envelope,
    block shape, columns count, mediaRef host, link scheme, namespace,
    slug membership, campaign-key allow-list, RawHTML size.
  - **Cross-stack drift-catcher** in
    `frontend/src/server/validate/campaign-keys.test.ts` — reads
    `internal/campaign/domain/visualdoc.go`, parses the
    `AllowedCampaignMergeTags` map literal, asserts deep equality with
    the TS const. Fails the frontend test suite if Go adds a key
    without a matching TS update.
  - Route-level tests in `frontend/src/server/routes/` — msw mocks for
    Go's `GET /subscriber-fields`, `GET /branding`, and
    `PUT /campaigns/{id}/visual`. Assert the BFF (a) fails closed with
    `502 bad_gateway` when Go is unreachable, (b) fetches branding when
    `theme` is null, (c) forwards rendered html+text and the session
    cookie to Go.
- **Frontend**: `vitest` (existing). New tests:
  - Component tests for `<VisualEmailEditor />` — slash command opens,
    insertion of each block type, drag-handle reorder, bubble-menu
    format, merge-tag chip insertion + serialization, image picker
    invocation, drag-and-drop image upload via the existing
    `api.media.upload` mock.
  - Route tests for template/campaign editor — switching between visual
    and code view preserves content; opening a legacy row (no
    `body_doc`) lands in code-only mode; opt-in conversion surfaces
    RawHTML blocks for unconvertible regions.
  - Component tests for the merge-tag picker — lists the tenant's
    registry rows + built-in pseudo-rows + campaign-level allow-list;
    respects typing-to-filter behavior.

**Target Platform**: Modern desktop browsers for the authoring surfaces.
The produced HTML targets Gmail (web/mobile), Apple Mail (desktop/iOS), and
Outlook (desktop/web). Multi-column layouts use table-based primitives so
Outlook desktop renders them correctly (per FR-015).

**Project Type**: Web application — Go backend extended with new
commands, queries, adapters, and a sanitization pass over BFF-rendered
HTML; TanStack Start + Nitro BFF gains two server routes that host
visual save + render-preview using react-email; React SPA extended
with the visual editor component, a code-view editor, the
subscriber-field-registry settings page, and inline changes to the
existing template/campaign editor routes.

**Performance Goals**: Interactive. Save (structured doc → render →
validate → sanitize → persist) traverses BFF then Go; the BFF render
via react-email plus Go's bluemonday pass is expected to complete in
well under 500 ms p95 end-to-end for typical campaigns (≤ ~50 blocks).
The send hot path is unchanged — `cmd/worker` reads `body_html` +
`body_text` and applies the existing placeholder substitutor per
recipient.

**Constraints**:

- **No TipTap Pro and no Notion-template license.** Every editor capability
  must be implementable on TipTap MIT core + StarterKit, possibly with
  in-house extensions. The custom drag handle is in-house, not the paid
  `@tiptap/extension-drag-handle-pro`.
- **No client-side rendering of email HTML.** The browser shows the
  visual surface and a desktop/mobile preview iframe that loads the
  *server-rendered* HTML. "Server" here means the BFF (Nitro) for the
  render step and Go for the validate/sanitize/persist step; the
  browser never produces the canonical HTML. This keeps the server
  tier as the single source of truth for sanitization (Constitution
  IV) and makes the send pipeline trivially correct (Constitution VI).
- **BFF failure modes are fail-closed.** When the BFF cannot reach Go
  for a required side-call (subscriber-fields fetch or branding
  fetch), the save returns `502 bad_gateway` and the operator retries.
  No silent fallback to platform defaults, no partial state (FR
  clarification 2026-05-20).
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
- ~1 trimmed package in `internal/campaign/adapters/visualrender/` —
  the bluemonday sanitizer (`sanitize.go`), the placeholder extractor
  (`placeholders.go`) used by Go's revalidation pass, and the
  HTML → structured-doc converter (US4 opt-in). The structured-doc →
  HTML renderer moved out of Go and into the BFF (per
  [research.md § R4](./research.md)).
- ~2 extended commands in `internal/campaign/app/command/` —
  `save_visual_template.go` and `save_visual_campaign.go` that accept
  the structured doc *plus the BFF-rendered html and text*, re-validate
  the doc against the registry, sanitize, and persist the three pieces
  atomically.
- ~1 new BFF render surface under `frontend/src/server/` —
  `render/` (react-email rendering), `validate/` (TS doc validator
  with a Go-source drift-catcher), `clients/go-api.ts` (cookie-
  forwarding HTTP client), and two Nitro routes for visual save and
  render-preview.
- ~1 extended substitutor at send time (extends the existing Phase 3 send
  pipeline) supporting the namespaced `{{ subscriber.<slug> }}` and
  `{{ campaign.<name> }}` syntax and a fixed allow-list of campaign
  values.
- ~8 new HTTP endpoints. The split: registry CRUD ×4 + merge-tags GET
  ×1 are Go-hosted; visual save for templates ×1 and campaigns ×1 are
  BFF-hosted (forward to Go after render); the render-preview endpoint
  ×1 is BFF-only (never reaches Go). Theme read/write folds into the
  campaign/template PATCH where natural.
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
    assertions on emitted HTML and plain text. The goldens now live in
    `frontend/src/server/render/` (TypeScript / `vitest`) since the
    renderer moved to the BFF; react-email is exact-pinned so fixtures
    stay stable across builds.
  - Sanitization has dedicated negative tests for each disallowed
    construct (script, data URLs, javascript: URLs, on*= handlers) on
    the Go side, which remains the authoritative sanitizer before
    persistence.
  - Cross-stack drift-catcher test in
    `frontend/src/server/validate/campaign-keys.test.ts` reads
    `internal/campaign/domain/visualdoc.go` and asserts the campaign-
    namespace allow-list stays in sync between Go and TypeScript.
  - Placeholder validation is covered with integration tests across the
    save-and-send path (unknown slug rejected at save; known slug
    substituted correctly per recipient at send).
  - Frontend routes ship with colocated `*.test.tsx` covering primary
    flows, empty states, error states, and the visual ↔ code-view
    round-trip. BFF Nitro routes ship with msw-mocked route-level
    tests covering fail-closed behavior, branding-fetch on null theme,
    and cookie forwarding to Go.

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
  - Server-side sanitization is authoritative. The Go API's
    bluemonday pass runs over the BFF-rendered HTML before
    persistence; it remains the single source of truth for what
    reaches the database. The BFF emits its own sanitizer pass over
    preview-only HTML so the preview iframe is never asked to render
    `<script>` even transiently.
  - Both server tiers run the doc validator (BFF for fast feedback,
    Go for authoritative re-check before persist) — defense in
    depth.
  - Placeholder substitution is server-side at send time only — the
    editor never executes or evaluates `{{ ... }}` against real
    subscriber data. The BFF's render-preview endpoint substitutes
    sample data only when the caller explicitly supplies it.
  - Registry CRUD is gated by `subscriber_fields:manage` and audited
    through the existing audit-log path used by other tenant-plane
    mutations.
  - Image URLs in the produced HTML are required to be tenant-scoped
    media-library references; both BFF and Go enforce this check
    against the same `ObjectStoragePublicBaseURL` env var (per FR-021).
  - BFF authentication: the user's session cookie is forwarded on
    every BFF→Go call; there is no service account or impersonation
    path.

- **V. Operable & Observable Services** — PASS (with explicit
  trade-off below).
  - All new code is stateless. Render is pure CPU work inside the BFF
    request lifecycle; sanitization + persistence is pure CPU work
    inside `cmd/api`'s request lifecycle; no new queue, no new
    long-running work.
  - The send pipeline in `cmd/worker` is unchanged — it continues to
    consume `body_html` + `body_text` from the row. The only
    worker-side change is extending the existing placeholder
    substitutor's regex / parser to recognize the namespaced syntax.
  - **Accepted trade-off**: the TanStack Start + Nitro BFF becomes
    load-bearing for visual saves and previews. It was already
    load-bearing as the SPA host (browser bootstrap, static assets);
    granting it the render responsibility does not add a new
    deployable service or a new operational alert surface — the
    existing health check + deploy pipeline already cover it.
  - Structured logging covers the new endpoints on both tiers with
    the standard `tenant_id`, `actor_id`, `request_id` fields. The BFF
    generates `request_id` if absent and propagates it via
    `X-Request-Id` header to Go, so one user trace correlates across
    BFF and Go logs.
  - Audit events (`campaign.save_visual`, `template.save_visual`) are
    emitted Go-side after persistence with the original payload shape
    `{ id, warnings_count }`. The BFF does not write audit rows.

- **VI. Layered Architecture & Domain Integrity** — PASS.
  - The new field registry lives in `internal/audience/`
    (`domain/field.go` with a validating constructor and the
    "persistence only" hydration helper; `app/command/...` and
    `app/query/...` for CQRS; `adapters/fields_postgres.go` for the
    Postgres repo that implements the command/query-owned interfaces).
  - The visual-document type lives in
    `internal/campaign/domain/visualdoc.go` (pure, no transport, no
    DB). The sanitizer and placeholder extractor live in
    `internal/campaign/adapters/visualrender/` because they depend on
    the `golang.org/x/net/html` and `bluemonday` adapters. The
    renderer no longer exists in Go — it moved to the BFF
    (`frontend/src/server/render/`).
  - Errors crossing domain boundaries carry typed kinds
    (`ErrInvalidPlaceholder`, `ErrUnknownSlug`, `ErrUnsupportedNode`,
    `ErrSanitizationStripped`) and are mapped to HTTP status codes in
    one place (`internal/api/...`), consistent with the existing pattern.
  - No new DI framework, no global state — Go composition stays in
    `cmd/api/main.go`; BFF composition is the existing TanStack Start
    + Nitro entry point.

**Result**: PASS — no violations, Complexity Tracking not required.

*Post-design re-check after Phase 1*: see the bottom of [data-model.md](./data-model.md)
and [contracts/](./contracts/) — design stays within the dependency
rule, introduces one new bounded context (`subscriber_fields` in
`internal/audience`), reuses the existing campaign/template aggregates
with two new typed fields, and adds one adapter package on the Go
side (sanitizer + placeholder extractor) plus a server-side render
surface on the BFF. Still PASS.

*Post-clarification re-check 2026-05-20 (BFF + react-email)*: the
render step relocated from Go to the BFF; see
[brainstorm-bff-render.md](./brainstorm-bff-render.md) for the
delta. All six constitutional gates still PASS — see the updated
notes inline above (II, IV, V, VI).

*Post-clarification re-check 2026-05-20 (autosave / concurrency /
substituter side-call / FR-002 wording)*: four further clarifications
landed (see [spec.md § Clarifications](./spec.md)):

1. **Autosave is deferred to Phase 7.1.** FR-008/SC-008 are out of
   scope for Phase 7. The editor's only persistence-loss guard in
   Phase 7 is the "unsaved changes" navigate-away prompt — a small
   piece of state inside the editor route. No new plumbing, no
   localStorage write paths, no Phase 7 work in the offline / session-
   expired recovery edge cases (they all defer with FR-008). This
   removes a planned-but-uncoded scope item; no constitutional impact.

2. **FR-009 multi-tab conflict uses optimistic concurrency on the
   row's `updated_at`.** Save bodies (browser→BFF and BFF→Go) carry
   `ifUnmodifiedSince: <ISO timestamp>`; Go's save handler compares
   against the row's current `updated_at` inside the same write
   transaction and returns the new typed `ErrStaleRow → 409
   stale_row`. No schema migration (per [research.md § R12a](./research.md)).
   Constitution II coverage extends to a stale-row integration test.
   Constitution VI coverage: the new error kind maps to HTTP in the
   single error-mapping point.

3. **Sample-data preview substitution side-calls Go.** The BFF's
   render-preview route POSTs to a new Go endpoint
   `POST /substitute-sample` instead of reimplementing substitution
   in TypeScript (per [research.md § R12b](./research.md)). Plan
   replaces the "TS reimplementation" entry under § R4
   Implementation surface; the new Go handler is one thin transport
   wrapper over `internal/sending/domain/substitution.go`. No
   constitutional impact — strengthens Principle VI by keeping
   business logic in exactly one place.

4. **FR-002 wording: "link" is a mark, not a block.** Pure spec
   wording; no plan-level impact (the renderer mapping table in R4
   already treats Link as a mark).

All six constitutional gates remain PASS after these amendments.
Tasks.md and contracts/tenant-api.md and data-model.md are updated
inline; research.md gains R12a and R12b.

*Post-clarification re-check 2026-05-20 (round 3 — preview endpoint
scope + analyze cleanups)*: one further clarification landed (N4)
plus a small inline-fix batch from /speckit-analyze round 2:

1. **Render-preview endpoint is tenant-scoped, not row-scoped.** The
   former `POST /campaigns/{id}/render-preview` is renamed to
   `POST /render-preview` and shared by both the campaign editor and
   the template editor (per [spec.md § Clarifications](./spec.md)).
   The endpoint's body was already doc-only (`bodyDoc`, `theme`,
   `sample`); the row id was never read. One Nitro route, one
   golden-fixture set, one access gate (`campaigns:manage` OR
   `templates:manage`). Templates inherit FR-007 preview without
   any new endpoint or task. Plan's "~8 new HTTP endpoints" count
   stays accurate: the rename does not add an endpoint.
2. **Inline cleanups from /speckit-analyze round 2**: T073
   (templates handler) mirrors T034's `ifUnmodifiedSince` body
   requirement; T075 covers `stale_row`; T038 asserts subscriber
   `attributes` survive `delete_field` (FR-016e); T070 asserts
   `<VisualEmailEditor />` is hidden for users without
   `campaigns:manage` (FR-034); T128 vs T037 boundary clarified;
   the addendum-intro dependency direction is flipped (T048
   depends on T126, not vice versa); FR-007 pins concrete preview
   widths (600 px desktop / 375 px mobile).

All six constitutional gates remain PASS — the rename strengthens
Principle V (one operational surface for preview) and Principle VI
(no row-scoped path for a row-agnostic operation).

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
│   │       ├── save_visual_template.go  # NEW — accept BFF-rendered html+text, revalidate doc, sanitize, persist three pieces
│   │       └── save_visual_campaign.go  # NEW — same shape as save_visual_template
│   │                                    # NOTE: render_preview lives in the BFF, not Go
│   └── adapters/
│       └── visualrender/
│           ├── sanitize.go              # NEW — bluemonday-based + email-specific deny rules; runs over the BFF-rendered HTML before persist
│           ├── convert.go               # NEW — best-effort raw-HTML → VisualDoc (US4 opt-in)
│           └── placeholders.go          # NEW — extract + validate `{{ … }}` placeholders against the registry (Go-side defense-in-depth pass)
├── sending/
│   └── domain/
│       └── substitution.go              # EXTENDED — recognize `{{ subscriber.<slug> }}` and `{{ campaign.<name> }}`
├── api/
│   └── handlers/
│       ├── subscriber_fields.go         # NEW — GET, POST, PATCH, DELETE, PATCH reorder
│       ├── templates.go                 # EXTENDED — PUT /templates/{id}/visual
│       └── campaigns.go                 # EXTENDED — PUT /campaigns/{id}/visual (BFF-hosted; Go owns the validate+persist tail); render-preview is BFF-only and is NOT mounted on the Go side
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
│   ├── lib/
│   │   ├── api.ts                               # EXTENDED — new endpoints: subscriberFields.* , templates.saveVisual, campaigns.saveVisual, campaigns.renderPreview
│   │   ├── api-types.ts                         # EXTENDED — VisualDoc, Theme, Field, FieldType, MergeTagPickerItem
│   │   └── permissions.ts                       # EXTENDED — `subscriber_fields:manage`
│   └── server/                                  # NEW — Nitro server-side surface (TanStack Start + Nitro BFF)
│       ├── render/
│       │   ├── index.ts                         # NEW — public `renderVisualDoc(doc, theme) → { html, text, warnings }`
│       │   ├── components.tsx                   # NEW — VisualBlock → react-email component mapping
│       │   ├── render.test.ts                   # NEW — golden tests for every block type + mark combination
│       │   └── render-marks.test.ts             # NEW — bold/italic/underline/strike/color/link combinations
│       ├── validate/
│       │   ├── index.ts                         # NEW — public `validateVisualDoc(doc, ctx)`
│       │   ├── envelope.ts                      # NEW — version + type check
│       │   ├── blocks.ts                        # NEW — per-block-type rule enforcement
│       │   ├── link.ts                          # NEW — scheme allow-list
│       │   ├── campaign-keys.ts                 # NEW — static mirror of Go's AllowedCampaignMergeTags
│       │   ├── campaign-keys.test.ts            # NEW — drift-catcher: parses Go source + asserts deep-equal
│       │   └── *.test.ts                        # NEW — per-rule unit tests
│       ├── clients/
│       │   └── go-api.ts                        # NEW — typed Go-API client with cookie + X-Request-Id forwarding
│       └── routes/
│           ├── visual-save.ts                   # NEW — Nitro route: PUT /t/:slug/api/campaigns/:id/visual (+ templates equivalent)
│           ├── render-preview.ts                # NEW — Nitro route: POST /t/:slug/api/render-preview (tenant-scoped, shared by campaign + template editors)
│           └── *.test.ts                        # NEW — route-level tests with msw mocks of Go's subscriber-fields/branding/save endpoints
```

**Structure Decision**: Web application — extend the existing Go
services (`cmd/api`, `cmd/worker`), the existing TanStack Start + Nitro
BFF, and the existing React SPA. The visual editor is a new component
tree inside `frontend/src/components/visual-editor/` embedded into the
existing template and campaign editor routes; it is not a separate
app. The backend gains one new bounded context (the
`subscriber_fields` registry inside `internal/audience/`) and a
slimmed-down adapter package (the sanitizer + placeholder extractor
inside `internal/campaign/adapters/visualrender/`); the existing
`Template` and `Campaign` aggregates are extended with two new typed
fields (`bodyDoc` and `theme`) and matching validating constructors.
The BFF gains a new server surface under `frontend/src/server/`
(render, validate, routes, clients) that intercepts the visual save
and preview endpoints before the catch-all proxy to Go and uses
`@react-email/components` to produce the canonical email HTML. The
send pipeline in `cmd/worker` is **unchanged** except for extending
its placeholder substitutor to recognize the namespaced syntax — this
preserves Constitution V (no new queue or stateful service) and
Principle VI (the worker keeps depending on `body_html` + `body_text`
only).

## Complexity Tracking

> No constitution violations to justify. Section intentionally empty.
