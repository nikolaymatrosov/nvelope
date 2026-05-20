# Phase 0 — Research

**Feature**: Phase 7 — Visual Email Editor
**Branch**: `014-visual-email-editor`
**Date**: 2026-05-20

This document resolves the open technical questions from
[plan.md](./plan.md) and from the clarification session captured in
[spec.md § Clarifications](./spec.md). Decisions here are inputs to
[data-model.md](./data-model.md) and [contracts/](./contracts/).

## R1. Editor stack: TipTap core vs React Email Editor vs Pro-license alternatives

**Decision**: Adopt **TipTap core + StarterKit (MIT)** plus in-house custom
extensions for the email-specific blocks (Columns, Button, Divider, Image,
MergeTag, RawHTML), the drag-handle widget, the slash-command menu, and the
bubble menu. Use the MIT-licensed `@tiptap/suggestion` and
`@tiptap/extension-bubble-menu` packages as building blocks.

**Rationale**:

- TipTap core and the suggestion / bubble-menu / link / image / color
  extensions are MIT-licensed and impose no production cost.
- The Notion-like-editor *template* and the prebuilt `DragHandle` Pro
  extension are gated behind a paid Start-plan subscription per
  [tiptap.dev/docs/ui-components/templates/notion-like-editor](https://tiptap.dev/docs/ui-components/templates/notion-like-editor),
  and we explicitly excluded the paid path in the spec.
- A custom drag handle on top of ProseMirror's node decorations is a
  contained piece of work (~150–300 LOC) and lets us match the visual
  language of the existing workspace (shadcn / Radix).
- `@react-email/editor` is MIT and ships an email-aware extension pack
  with HTML export, but its component surface (built-in bubble menus,
  slash menu) is opinionated and harder to compose with our shadcn
  styling than a from-scratch TipTap setup; its HTML export is also
  client-side, which conflicts with our server-side render decision
  (R3).

**Alternatives considered**:

- **React Email Editor (`@react-email/editor`)**: rejected for the
  reasons above. Worth revisiting in a follow-up phase if the in-house
  block library proves expensive to maintain.
- **TipTap Pro Notion-like template**: rejected — paid Start plan,
  explicitly excluded in [spec.md](./spec.md) Assumptions.
- **Lexical (Meta)**, **Slate**, **Plate**: all viable. TipTap chosen
  because its node-schema model maps cleanly to our serialization
  requirements, its ecosystem of MIT extensions is the largest, and
  ProseMirror's transaction model handles round-tripping
  HTML ↔ document well.

## R2. Code-view editor

**Decision**: **`@uiw/react-codemirror`** + `@codemirror/lang-html` (both
MIT). Used for code-only mode (US4), for the visual-editor's code view,
and as the textarea replacement on legacy raw-HTML rows.

**Rationale**:

- CodeMirror 6 is lighter than Monaco (~150 KB vs ~3 MB gzip), faster
  to load inside our existing route bundle, and the `react-codemirror`
  wrapper is widely-used and stable.
- HTML highlighting is sufficient — we do not need IntelliSense,
  multi-file editing, or a full IDE feature set inside the workspace.

**Alternatives considered**:

- **Monaco** (the user mentioned it): rejected primarily on bundle
  size. Monaco shines in IDE contexts; here we need only an HTML
  textarea with highlighting and reasonable keyboard behavior.
- **Plain `<textarea>`**: rejected because operators authoring raw HTML
  for transactional templates or for opted-out campaigns benefit from
  syntax highlighting and bracket matching.

## R3. Where structured-doc → HTML rendering runs

**Decision (resolved by clarification Q2 and revised 2026-05-20 to
narrow "server" from `cmd/api` to the workspace BFF — see § R4)**:
**Server-side at save time**. The frontend POSTs only the structured
document on save; the *server tier* renders the email-ready HTML and
plain-text alternative, sanitizes, and persists all three pieces
atomically within the same logical save. The server tier is split: the
TanStack Start + Nitro BFF performs the render (§ R4); the Go API
performs validation, sanitization, and persistence. The browser never
produces the canonical HTML. The send worker (`cmd/worker`) does no
rendering and continues to read `body_html` / `body_text` from the row.

**Rationale**:

- Single source of truth for sanitization (Constitution IV). The
  authoritative sanitizer (bluemonday) remains in Go; the BFF's render
  output passes through it before persistence.
- Determinism — no per-browser variance in produced HTML.
- The send hot path stays cheap. Workers don't pull in a JS runtime
  or repeat the render per recipient.
- Aligns with how Phase 6 already sanitizes custom CSS server-side
  (the bluemonday profile is reused).

**Alternatives considered**:

- **Client-side render at save** (the React Email Editor pattern):
  rejected — would require trusting client output and either skipping
  the server sanitizer (unsafe) or running a separate Go renderer to
  re-derive the canonical HTML (defeats the point of client render).
- **Lazy render on send**: rejected — adds a JS runtime or a Go
  renderer dependency to `cmd/worker` and re-runs the same work for
  every recipient.

## R4. Structured-doc → email-HTML renderer in the BFF (react-email)

**Decision (revised 2026-05-20 from "in-house Go renderer" to "BFF +
react-email" — supersedes the previous text; see
[brainstorm-bff-render.md](./brainstorm-bff-render.md) for the
full justification)**: Render in the TanStack Start + Nitro BFF using
`@react-email/components` and `@react-email/render`. The BFF
intercepts the visual save and preview endpoints before the catch-all
proxy to Go, walks the typed VisualDoc tree, renders to email-ready
HTML + plain-text via react-email, and (for save) forwards the
rendered HTML alongside the structured document to the Go API for
validation, sanitization, and persistence. The render is synchronous,
pure CPU (no React server-side hydration cost beyond `renderToString`),
and unit-testable with golden outputs.

**Block-by-block component mapping** (VisualDoc → react-email):

| Block            | react-email component / primitive                                 | Plain-text primitive                                  |
|------------------|-------------------------------------------------------------------|-------------------------------------------------------|
| Paragraph        | `<Text>`                                                          | text + blank line                                      |
| Heading          | `<Heading as="h{1..3}">`                                          | text prefixed with `# ` / `## ` / `### `              |
| Bulleted list    | inline-styled raw `<ul>` + `<li>` (no react-email primitive)      | `- item\n`                                            |
| Numbered list    | inline-styled raw `<ol>` + `<li>`                                 | `1. item\n`                                           |
| Quote            | inline-styled raw `<blockquote>`                                  | `> text`                                              |
| Code             | `<CodeBlock>`                                                     | text                                                  |
| Link mark        | `<Link>`                                                          | text + ` (url)`                                       |
| Image            | `<Img>`                                                           | `[image: alt]`                                        |
| Button           | `<Button>` (renders with Outlook VML fallback)                    | `[ label ] (url)`                                     |
| Divider          | `<Hr>`                                                            | `----`                                                |
| 2/3/4-column row | `<Row>` + N × `<Column>` (MSO conditional comments built-in)      | each column rendered then concatenated with `\n\n`    |
| MergeTag         | literal `{{ subscriber.<slug> }}` / `{{ campaign.<name> }}` text  | same literal                                          |
| RawHTML          | `dangerouslySetInnerHTML` passthrough; sanitized later by Go      | crude HTML-to-text fallback                            |

react-email computes inline styles from the row's theme (resolved from
the explicit override or fetched from `GET /branding` when the
override is null) before emit, so we don't depend on `<style>` blocks
the way Gmail clipping rules sometimes strip them.

**Rationale**:

- react-email is the most battle-tested email-rendering library in the
  JS ecosystem. Outlook VML fallbacks for buttons, MSO conditional
  comments around column tables, pixel-vs-percent sizing, and the
  inline-style invariant are all encoded in the library — we get them
  for free instead of porting each quirk into Go.
- The repository already runs a Node tier next to the Go API: TanStack
  Start + Nitro is the SPA host. Giving it the render responsibility
  costs no new operational surface beyond what already exists.
- The browser still does not produce the canonical HTML — the *server*
  in "server-side at save time" is the BFF, satisfying the R3 intent
  (Constitution IV).
- The Go API stays single-purpose: validate, sanitize, persist. No JS
  runtime in `cmd/api`.

**Implementation surface**:

- `frontend/src/server/render/` — render module (VisualBlock →
  react-email component mapping)
- `frontend/src/server/validate/` — TypeScript port of the doc
  validator with a drift-catcher test that reads
  `internal/campaign/domain/visualdoc.go`'s
  `AllowedCampaignMergeTags` map literal to keep the BFF and Go
  campaign-namespace allow-lists in sync
- `frontend/src/server/routes/visual-save.ts` and
  `frontend/src/server/routes/render-preview.ts` — Nitro route
  handlers that intercept the visual-editor endpoints before the
  catch-all proxy to Go
- Go-side: the renderer (`internal/campaign/adapters/visualrender/render.go`)
  is removed; the sanitizer (`sanitize.go`) and placeholder extractor
  (`placeholders.go`) remain — they support the Go-side revalidation
  and sanitization pass

**Alternatives considered**:

- **In-house Go renderer** (the previous decision): rejected because
  porting react-email's email-client quirks (Outlook VML, MSO
  conditional comments) into Go is a maintenance burden we can avoid;
  we shipped this approach in T021 and Phase 2 but reversed before
  US1 lands.
- **MJML server-side** (`mjml-go`): rejected because it imposes its
  own block vocabulary that doesn't match the in-editor block model
  one-to-one; we'd end up maintaining a translation layer between our
  blocks and MJML's components, plus the MJML transformation itself.
- **Spawning a separate Node `cmd/render-worker` service**: rejected
  because the BFF already provides the JS runtime; standing up an
  additional service is unnecessary operational cost.
- **Embedding a JS runtime in the Go API (`goja` / `v8go`)**: rejected
  because react-email isn't designed for that environment, React SSR
  semantics in `goja` are brittle, and we'd lose the operational
  observability the BFF already has.

## R5. HTML sanitization profile

**Decision**: Reuse the Phase 6 bluemonday profile as the base, with the
following email-specific *additions* applied to the output of the
renderer **and** to any RawHTML block:

- Strip every `<script>`, `<style>` (we ship inline styles only), `<iframe>`,
  `<object>`, `<embed>`, `<form>`, `<input>`, `<link>` element.
- Strip every `on*=` attribute regardless of element.
- Strip every `href`/`src` whose scheme is `javascript:`, `vbscript:`,
  `data:`, or `file:`. Allow only `http:`, `https:`, `mailto:`, `tel:`,
  and the relative paths the renderer itself produced for media-library
  references.
- Strip every `<img>` whose `src` does not match the tenant's
  media-library URL pattern *unless* the operator explicitly placed it
  via code-view edit (in which case it's still subject to scheme
  filtering above and is logged as a warning at save).

Sanitization is the last step before persistence; if it stripped
anything beyond whitespace and benign whitespace-normalization, the API
includes a `warnings: []` array in the save response so the editor can
surface a clear, non-blocking notice (per FR-014 / Constitution IV).

**Rationale**: bluemonday is already a dependency, has a battle-tested
allow-list model, and the email-specific deny rules above are a small,
auditable surface. Mirroring the Phase 6 profile keeps a single mental
model.

**Alternatives considered**: writing a from-scratch sanitizer (rejected —
high security risk for negligible benefit) or shipping a stricter
DOMPurify on the frontend only (rejected — does not protect against a
malicious or malformed client).

## R6. Best-effort raw-HTML → structured-doc conversion (US4 opt-in)

**Decision**: Convert with `golang.org/x/net/html` in
`internal/campaign/adapters/visualrender/convert.go`. The converter is
**deliberately conservative**: nodes that map cleanly to our block
vocabulary (`<p>`, `<h1..h6>`, `<ul>`, `<ol>`, `<li>`, `<a>`, `<img>`,
`<hr>`, `<blockquote>`, recognized `<table>` shapes that look like
column layouts) become typed blocks; everything else is preserved
verbatim inside a `RawHTML` block (per FR-031 / spec User Story 4 ACs).

**Heuristics for table → Columns**:

- A `<table>` whose immediate `<tr>` has 2, 3, or 4 `<td>` children,
  recursively containing only convertible nodes, becomes a `Columns`
  block.
- Anything else (nested tables, `colspan`, `rowspan`, unusual cell
  counts) collapses to a single `RawHTML` block to avoid lossy
  reflowing.

**Rationale**: pre-existing HTML is unpredictable. A conservative
converter that round-trips intact at worst and structures cleanly at
best is dramatically safer than an aggressive one that silently
restructures content.

**Alternatives considered**: a Go port of the `turndown`/`html-to-tiptap`
JS heuristics (rejected — large surface, marginal value over what the
above heuristics already cover for our block vocabulary).

## R7. Tenant subscriber custom-field registry (resolved by clarification Q4-a)

**Decision**: New tenant-scoped table `subscriber_fields` with columns
`(id, tenant_id, slug, display_name, type, default_value, position,
created_at, updated_at)`. `UNIQUE (tenant_id, slug)`. RLS on
`tenant_id` via the existing tenant-bound transaction adapter. Built-in
fields (`email`, `name`, `first_name`, `last_name`, `state`) are
surfaced as pseudo-rows by `query.ListFields` so the merge-tag picker
and the Phase 6 subscription-page field picker treat them uniformly.

**Field types**: `text` | `number` | `date` | `boolean` | `url`. `type`
governs how the send-time substitutor formats the value (e.g. dates
become `YYYY-MM-DD` by default; URLs become bare URLs in plain-text and
linkified in HTML).

**Rationale**: gives the picker a real list, makes save-time
placeholder validation possible (Constitution IV — catch typos at save
not at send), aligns the editor with Phase 6 subscription pages so
there's one canonical list per tenant, and stays compatible with the
existing free-form `attributes` JSONB on subscribers (uncategorised
keys keep working but don't appear in the picker, per FR-016e).

**Alternatives considered**:

- **Derive picker contents from observed `attributes` keys**: rejected
  — picker contents would change with the data, no typing, no
  validation.
- **Hard-code a fixed placeholder set**: rejected — too restrictive
  for an ESP.
- **Phase 6 per-subscription-page "visible profile fields" as the
  de-facto list**: rejected — it's per-page configuration, not a
  registry, and would force Phase 7 to walk all subscription pages to
  derive a union, with no schema enforcement.

## R8. Placeholder syntax and substitution timing (resolved by Q3)

**Decision**: Namespaced double-curly tags:

- `{{ subscriber.<slug> }}` — looks up `<slug>` in the tenant's
  `subscriber_fields` registry (including built-in pseudo-rows).
  Whitespace inside the braces is allowed and ignored.
- `{{ campaign.<name> }}` — looks up `<name>` in a fixed,
  platform-maintained allow-list (`unsubscribe_url`,
  `preference_url`, `archive_url`, `view_in_browser_url`,
  `tenant_name`, `current_date`).

**When substituted**: at send time, server-side, per recipient, in the
existing Phase 3 send pipeline (`internal/sending/`). The editor
preserves placeholders verbatim through save → reload. The
`POST /campaigns/{id}/render-preview` endpoint substitutes against a
caller-supplied sample subscriber for editor preview only.

**When validated**: at *save* time. The save handler walks the rendered
HTML and the structured doc, extracts every placeholder, and rejects
the save with `ErrUnknownSlug` if any placeholder references a slug not
in the registry. This prevents shipping a campaign whose substitution
would silently break at send.

**Rationale**: ESP-conventional syntax operators recognize; namespacing
prevents accidental collisions between subscriber fields and campaign
values; save-time validation surfaces errors at the right moment;
preserving placeholders verbatim keeps the editor's contract simple.

**Alternatives considered**: Go `html/template` syntax (rejected — too
developer-facing for the operator audience); flat un-namespaced
`{{first_name}}` (rejected — risk of collision with campaign-level
values).

## R9. Merge-tag chip UX (resolved by Q4-b)

**Decision**: Implement merge tags as a TipTap inline node
(`MergeTag`) rendered as a styled pill with the field's `display_name`
visible and the raw tag visible on hover (title attribute). On
serialization the node emits the literal `{{ subscriber.<slug> }}` /
`{{ campaign.<name> }}` placeholder so the renderer + send pipeline
see plain text.

**Picker entry points**:

- A "merge tag" button in the bubble menu (when the selection is in a
  text-accepting block).
- A slash-command entry "Insert merge tag …" that opens the same
  picker.

**Picker source**: `GET /api/v1/t/{slug}/merge-tags` — returns the
union of (a) `subscriber_fields` rows including built-in pseudo-rows
and (b) the platform's campaign-level allow-list. Cached client-side
with TanStack Query under a stable query key
`["merge-tags", tenantSlug]`.

**Round-trip from code view**: when the operator types raw
`{{ subscriber.<slug> }}` in code view, the parser on return to the
visual surface recognizes the literal and replaces it with a
`MergeTag` node carrying the slug.

## R10. Theming (resolved by US3 + FR-022..025)

**Decision**: A `Theme` value object with fields
`{ textColor, linkColor, buttonColor, buttonTextColor, fontFamily, containerWidth }`.
Stored as JSON in `campaigns.theme` / `templates.theme`. When the
column is NULL, the renderer derives defaults from the row's tenant
Phase 6 branding (`tenants_branding.primary_color`, etc.) at render
time. The frontend reflects this distinction with a small "using
tenant defaults" indicator and a single button to "pin a theme
override" that copies the current resolved values into the row.

**Rationale**: keeps the override decision explicit and visible;
inheritance behavior matches FR-024 (tenant changes propagate to
unpinned rows on next save/preview).

## R11. Permission gating

**Decision**: One new permission string —
`subscriber_fields:manage` — gating registry CRUD. The visual editor
itself, theme overrides, and code-view authoring inherit the existing
`campaigns:manage` and `templates:manage` permissions from Phase 3 UI
(007). The merge-tag picker is readable by any operator who can edit
the campaign or template they're authoring; the registry editing UI
under `/settings/fields` requires `subscriber_fields:manage`.

**Rationale**: minimal addition consistent with the Phase 1–6 pattern;
authoring a campaign should not be more locked-down than it is today
just because the editor changed.

## R12. Audit logging

**Decision**: Audit events emitted by registry CRUD
(`subscriber_field.create|update|delete|reorder`) and by visual save
(`campaign.save_visual`, `template.save_visual`). The visual-save
audit entry includes the `warnings: []` summary from sanitization but
NOT the full body — the body lives on the row already and audit log
size is bounded.

## R13. Open question — HTML file upload

**Decision**: **Deferred** to a follow-up PR. The spec already covers
"edit as HTML only" (FR-029) via the code editor, which accepts paste.
File-upload is purely a convenience and does not block any user story.

**Why deferred**: not on the critical path, no acceptance scenario
depends on it, and adding it now would couple a small file-upload
endpoint to Phase 7 unnecessarily. Captured as a TODO in
[quickstart.md](./quickstart.md).

## Summary of resolved unknowns

Every NEEDS CLARIFICATION raised by the plan template is resolved
above. No outstanding research items remain. Phase 1 may proceed.
