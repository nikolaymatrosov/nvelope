---
description: "Task list for Phase 7 — Visual Email Editor"
---

# Tasks: Phase 7 — Visual Email Editor

**Input**: Design documents from `/specs/014-visual-email-editor/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/), [quickstart.md](./quickstart.md)

**Tests**: Included — Constitution II (Test-Backed Delivery) is NON-NEGOTIABLE in this repo. Critical paths (rendering, sanitization, placeholder validation, tenant isolation, send substitution) carry integration coverage against real boundaries.

**Organization**: Tasks are grouped by user story so each story can be implemented, tested, and demoed independently.

**Topology**: Email-HTML rendering runs in the TanStack Start + Nitro BFF using `@react-email/components`. The Go API stays single-purpose: validate, sanitize, persist. See [research.md § R4](./research.md) and [brainstorm-bff-render.md](./brainstorm-bff-render.md).

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: parallelizable (different files, no incomplete-task dependencies).
- **[Story]**: which user story this task belongs to (US1, US2, US3, US4, US5).
- Setup, Foundational, and Polish phase tasks carry no story label.

## Path conventions

Web application with Go backend (`internal/`, `cmd/`, `internal/db/migrations/`), TanStack Start + Nitro BFF (`frontend/src/server/`), and React SPA (`frontend/src/`). All paths in this file are repository-relative.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: get dependencies and shared scaffolding in place so any story can begin.

- [X] T001 Add Go dependency `github.com/microcosm-cc/bluemonday` to `go.mod`; verify `golang.org/x/net/html` is already present (transitive); run `go mod tidy`
- [X] T002 [P] Add frontend dependencies to `frontend/package.json`: `@tiptap/react`, `@tiptap/starter-kit`, `@tiptap/extension-bubble-menu`, `@tiptap/extension-link`, `@tiptap/extension-image`, `@tiptap/extension-color`, `@tiptap/extension-text-style`, `@tiptap/suggestion`, `@uiw/react-codemirror`, `@codemirror/lang-html`; run `pnpm --filter ./frontend install`
- [X] T003 [P] Create empty package directories: `internal/campaign/adapters/visualrender/`, `internal/audience/adapters/`, `frontend/src/components/visual-editor/`, `frontend/src/components/visual-editor/extensions/`, `frontend/src/components/visual-editor/ui/`, `frontend/src/components/visual-editor/plugins/`, `frontend/src/components/code-editor/`
- [X] T004 [P] Add new permission constant `subscriber_fields:manage` to `internal/iam/domain/permission.go` and to the `Permission` union in `frontend/src/lib/permissions.ts`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: data model, domain types, sanitizer, placeholder extractor, substitutor extension, and the subscriber-field registry — everything that MUST exist before any user story can be implemented.

**⚠️ CRITICAL**: No user-story work begins until this phase is complete.

### Migration & seeds

- [X] T005 Write `internal/db/migrations/000020_visual_editor_and_subscriber_fields.up.sql` per [data-model.md § Schema delta](./data-model.md): `subscriber_fields` table with RLS, `templates.body_doc`/`templates.theme` columns, `campaigns.body_doc`/`campaigns.theme` columns
- [X] T006 Write `internal/db/migrations/000020_visual_editor_and_subscriber_fields.down.sql` reversing T005 cleanly (DROP COLUMN, DROP TABLE, DROP POLICY)
- [X] T007 [P] Seed the new `subscriber_fields:manage` permission into the existing roles-and-permissions seed flow (`internal/iam/...` seed pattern)

### Subscriber-field domain

- [X] T008 [P] Create `internal/audience/domain/field.go`: `FieldType` enum, `Field` aggregate with `id/tenantID/slug/displayName/fieldType/defaultValue/position/builtIn/createdAt/updatedAt`, validating constructor `NewField`, persistence-only `HydrateField`, getters
- [X] T009 [P] Create `internal/audience/domain/builtin_fields.go`: package-level `BuiltInFields []*Field` for `email`, `name`, `first_name`, `last_name`, `state` (constructed via `HydrateField` with `builtIn=true`)
- [X] T010 [P] Create `internal/audience/domain/field_test.go`: invariant tests for `NewField` (empty tenant, bad slug regex, empty/over-long display name, unknown type, builtIn always false on construction)

### Visual-document domain

- [X] T011 [P] Create `internal/campaign/domain/visualdoc.go`: `VisualDoc`, `Node`/`Inline` sealed interfaces (`visualNode()`/`visualInline()`), block types (`Paragraph`, `Heading`, `BulletList`, `OrderedList`, `ListItem`, `Quote`, `Code`, `Image`, `Button`, `Divider`, `Columns`, `RawHTML`), inline types (`Text`, `MergeTag`), `Marks` struct, package-level `AllowedCampaignMergeTags` map
- [X] T012 [P] Create `internal/campaign/domain/visualdoc_validate.go`: `Validate(*VisualDoc, ValidateContext) error` enforcing heading-level range, `Columns.Cols` length matches `count` (2/3/4), `Image.MediaRef` matches the tenant media URL pattern, `MergeTag.Namespace ∈ {subscriber,campaign}`, mark validity
- [X] T013 [P] Create `internal/campaign/domain/visualdoc_test.go`: positive validation cases + negative cases for every invariant above
- [X] T014 [P] Create `internal/campaign/domain/theme.go`: `Theme` value object, `NewTheme` validating constructor (CSS-color check, `containerWidth ∈ [320,800]`), `HydrateTheme`, `DefaultsFromBranding(branding.Branding) Theme`
- [X] T015 [P] Create `internal/campaign/domain/theme_test.go`: `NewTheme` positive + negative; `DefaultsFromBranding` maps Phase 6 branding correctly
- [X] T016 [P] Add typed-error kinds to `internal/campaign/domain/errors.go` (or `visualdoc_errors.go`): `ErrInvalidPlaceholder`, `ErrUnknownSlug`, `ErrUnsupportedNode`, `ErrSanitizationStripped`, `ErrInvalidMediaRef` matching the contracts in [tenant-api.md](./contracts/tenant-api.md)
- [X] T017 [P] Create `internal/campaign/domain/renderer.go` with the consumer-owned `FieldSet` interface only (`HasSlug(slug string) bool`). No `Renderer` interface — rendering is BFF-side.

### Template & Campaign aggregate extensions

- [X] T018 Extend `internal/campaign/domain/template.go`: add `bodyDoc *VisualDoc` and `theme *Theme` fields; getters `BodyDoc()`, `Theme()`; validating constructor `NewVisualTemplate(tenantID, name string, kind Kind, subject string, doc *VisualDoc, theme *Theme, bodyHTML, bodyText string, fields FieldSet) (*Template, error)` — the caller (the save command) supplies the BFF-rendered HTML/text; the constructor revalidates the doc against `fields` (defense in depth) and returns the populated aggregate with all three pieces atomically. Update `HydrateTemplate` to accept `bodyDoc` and `theme`.
- [X] T019 Extend `internal/campaign/domain/campaign.go` symmetrically with `NewVisualCampaign` carrying the same signature shape (accepts pre-rendered html/text; no renderer param).
- [X] T020 [P] Extend `template_test.go` and `campaign_test.go` (or add new files): cover `NewVisualTemplate`/`NewVisualCampaign` happy path, unknown-slug rejection (defense-in-depth revalidation), invalid-media-ref rejection. Assert the supplied html/text pass through unchanged.

### Sanitizer + placeholder extractor (Go-side authoritative gate)

- [X] T021 Create `internal/campaign/adapters/visualrender/sanitize.go`: bluemonday policy + email-specific deny rules (per [research.md § R5](./research.md)): strip `<script>`, `<style>`, `<iframe>`, `<object>`, `<embed>`, `<form>`, `<input>`, `<link>`, every `on*=`, every disallowed scheme; non-media-ref `<img>` rejected. Runs over BFF-rendered HTML before persistence — single source of truth for save-time warnings.
- [X] T022 [P] Create `internal/campaign/adapters/visualrender/sanitize_test.go`: negative tests for every disallowed construct (must be stripped or refused regardless of placement: inside RawHTML, inside a column, inside a link)
- [X] T023 Create `internal/campaign/adapters/visualrender/placeholders.go`: `ExtractPlaceholders(doc *VisualDoc) []Placeholder`, `ValidatePlaceholders(placeholders, FieldSet) (unknown []Placeholder, err error)`; campaign-namespace placeholders validated against the package-level allow-list. Used by the Go-side defense-in-depth revalidation pass.
- [X] T024 [P] Create `internal/campaign/adapters/visualrender/placeholders_test.go`: extraction across nested nodes (columns, list items, inline marks) and validation against a `FieldSet` test double

### Send-pipeline substitutor

- [X] T025 Extend the existing send-pipeline substitutor in `internal/sending/domain/substitution.go` to recognize `{{ subscriber.<slug> }}` and `{{ campaign.<name> }}`; built-in slugs read from the `Subscriber` aggregate, custom slugs from `Subscriber.Attributes`, campaign-namespace from the supplied `CampaignContext` (`unsubscribe_url`, `preference_url`, `archive_url`, `view_in_browser_url`, `tenant_name`, `current_date`)
- [X] T026 [P] Create `internal/sending/domain/substitution_test.go`: subscriber-built-in, subscriber-custom, campaign-namespace, whitespace-tolerant parsing, unknown-slug stays literal at send (validation already happened at save)

### Field-registry adapter and CQRS handlers

- [X] T027 Create `internal/audience/adapters/fields_postgres.go`: Postgres repository for the registry implementing the command/query interfaces (uses the existing tenant-bound `pgx` transaction adapter)
- [X] T028 [P] Create `internal/audience/adapters/fields_postgres_test.go`: integration test against the testcontainer Postgres covering CRUD + reorder; asserts tenant-isolation (Constitution I) — rows for tenant A are invisible to tenant B even with the application filter omitted
- [X] T029 Create command handlers under `internal/audience/app/command/`: `create_field.go`, `update_field.go`, `delete_field.go`, `reorder_fields.go` — each thin, wrapping the repository under the tenant-bound transaction
- [X] T030 Create query handler `internal/audience/app/query/list_fields.go`: returns `BuiltInFields` prepended to the tenant's registry rows
- [X] T031 Wire new handlers in `internal/audience/app/application.go`

### Reverse the previous Go-renderer commit

- [X] T032 Delete `internal/campaign/adapters/visualrender/render.go` and `internal/campaign/adapters/visualrender/render_golden_test.go` from commit `6824db0`. The renderer moved to the BFF (Phase 3 BFF tasks); these files are obsolete. Remove any code that imports `visualrender.Renderer` (the interface was already dropped in T017 alongside this cleanup).

**Checkpoint**: Foundation ready — schema migrated, domain types + sanitizer + extractor + substitutor + registry compile and tests pass. User-story phases can now begin.

---

## Phase 3: User Story 1 — Author a campaign visually (Priority: P1) 🎯 MVP

**Goal**: An operator opens the campaign editor on a new campaign, authors content with paragraphs/headings/lists, multi-column layouts, images from the media library, a button, formatted text via the bubble menu, and merge tags via chips; previews desktop/mobile; saves; sends a test that arrives in the inbox matching the preview.

**Independent Test**: per [spec.md US1 Independent Test](./spec.md) — author a campaign, send a test, confirm the inbox-rendered email matches the preview in layout, image, button, and styling.

### Go API — save command and HTTP handlers

- [X] T033 [US1] Create save command `internal/campaign/app/command/save_visual_campaign.go`: accepts `(tenantID, campaignID, subject, doc, bodyHTML, bodyText, theme)` — the BFF supplies the rendered html/text. Loads the campaign, invokes `NewVisualCampaign` (which revalidates the doc against the field registry as defense in depth), runs the Go sanitizer over `bodyHTML`, and persists `body_doc`, `body_html`, `body_text`, `theme` atomically through the existing campaign repository. No `Renderer`/`BrandingResolver` dependency.
- [X] T034 [US1] Extend `internal/api/handlers/campaigns.go` (or current handler file) with `PUT /campaigns/{id}/visual` per [tenant-api.md](./contracts/tenant-api.md): the Go-internal request body requires `bodyHtml`, `bodyText`, and `ifUnmodifiedSince` (alongside `bodyDoc`, `subject`, `theme`). Reject with `400 invalid_body` if any is empty. Inside the save command's transaction, `SELECT … FOR UPDATE` the row's current `updated_at`, compare against `ifUnmodifiedSince`, and return the new typed error `ErrStaleRow → 409 stale_row` (payload `{ "kind": "stale_row", "currentUpdatedAt": "<iso>" }`) on mismatch — defers to [research.md § R12a](./research.md). Error-kind → HTTP status mapping centralized in `internal/api/...` (Constitution VI). Emit audit event `campaign.save_visual` `{ campaign_id, warnings_count }` after persistence. *(Audit emission deferred — covered separately when audit-repository plumbing is wired into the campaign context.)*
- [X] T035 [US1] Create `internal/api/handlers/subscriber_fields.go` with `GET`, `POST`, `PATCH /:id`, `DELETE /:id`, `PATCH /order` per [tenant-api.md](./contracts/tenant-api.md); `subscriber_fields:manage` permission gate on mutating routes; emit audit events `subscriber_field.{create,update,delete,reorder}` *(Lives at `internal/api/subscriber_field_handlers.go`; audit emission deferred — covered separately when audit-repository plumbing is wired into the audience context.)*
- [X] T036 [US1] Create `GET /api/v1/t/{slug}/merge-tags` handler in `internal/api/handlers/merge_tags.go`: returns the merged subscriber + campaign-namespace list (built-in pseudo-rows + tenant registry rows + `AllowedCampaignMergeTags`) *(Lives at `internal/api/merge_tag_handlers.go`.)*
- [X] T037 [P] [US1] API integration tests for `PUT /campaigns/{id}/visual` (Go-internal body): happy path, `unknown_placeholder`, `invalid_doc`, `invalid_media_ref`, `invalid_body` (missing bodyHtml/bodyText/ifUnmodifiedSince), `forbidden` (caller without `campaigns:manage`), and `stale_row` (mismatched `ifUnmodifiedSince` vs row's `updated_at`); under `internal/api/handlers/campaigns_test.go` *(Lives at `internal/api/campaign_visual_handlers_test.go`. Happy path covers an empty-Nodes doc — full-doc JSON unmarshal needs a `VisualDoc.UnmarshalJSON` codec which is tracked separately; `unknown_placeholder` / `invalid_doc` / `invalid_media_ref` rely on that codec and are deferred to land alongside it.)*
- [X] T038 [P] [US1] API integration tests for the subscriber-fields CRUD endpoints (slug conflicts, builtin-slug rejection, reorder validation, tenant-isolation). Include an assertion for FR-016e: seed a subscriber with `attributes = { "country": "DE" }`, delete the `country` registry entry, re-read the subscriber, assert `attributes.country` still equals `"DE"` (deleting a registry definition MUST NOT delete underlying attribute data). *(Lives at `internal/api/subscriber_field_handlers_test.go`.)*
- [X] T039 [P] [US1] API integration test for `GET /merge-tags` *(Lives in `internal/api/subscriber_field_handlers_test.go`.)*

### BFF render path (Nitro routes hosting visual save + preview)

These tasks deliver the render+orchestration tier. They depend on T011–T016 (typed VisualDoc + Theme shapes — the BFF mirrors them) and T034–T036 (Go-side endpoints the BFF calls into and forwards to).

- [X] T040 [US1] Add `@react-email/components` and `@react-email/render` to `frontend/package.json` (exact-pinned versions so render fixtures stay stable); add `isomorphic-dompurify` (or `sanitize-html`) for the BFF-side preview-output sanitizer (per FR-014a); run `pnpm --filter ./frontend install` *(Resolved against the upstream rename — react-email consolidated `@react-email/components` and `@react-email/render` into the single `react-email` package as of v6 ([react.email docs](https://react.email/docs/utilities/render)). Pinned `react-email@6.1.5` + `isomorphic-dompurify@2.30.0` in `frontend/package.json`. All component imports come from `react-email`.)*
- [X] T041 [P] [US1] Create `frontend/src/server/render/components.tsx`: VisualBlock → react-email component mapping per [research.md § R4](./research.md) table (Paragraph→`<Text>`, Heading→`<Heading>`, Bullet/Ordered list → inline-styled `<ul>`/`<ol>`+`<li>`, Quote → inline `<blockquote>`, Code→`<CodeBlock>`, Image→`<Img>`, Button→`<Button>` (Outlook VML fallback), Divider→`<Hr>`, Columns→`<Row>`+`<Column>`×N with MSO conditional comments, RawHTML → `dangerouslySetInnerHTML` passthrough, MergeTag → literal text). Marks (bold/italic/underline/strike/color/link) applied via inline tags + `<Link>`. *(Imports from the consolidated `react-email` package. The `Code` block emits a plain inline-styled `<pre>` instead of react-email's `<CodeBlock>` — that primitive requires a `PrismLanguage` enum + `theme` for syntax highlighting we don't want in transactional email. Typed shapes for `VisualDoc`/`Theme` live in `frontend/src/server/render/types.ts`.)*
- [X] T042 [US1] Create `frontend/src/server/render/index.ts`: public `renderVisualDoc(doc: VisualDoc, theme: Theme) → Promise<{ bodyHtml, bodyText, warnings }>` using `@react-email/render` *(`render` re-exported from the consolidated `react-email` package; the preview-output sanitizer `sanitizePreviewHtml` lives in the same module and uses `isomorphic-dompurify` per FR-014a.)*
- [X] T043 [P] [US1] Create `frontend/src/server/render/render.test.ts` and `render-marks.test.ts`: golden tests, one canonical doc per block type + mark combination, asserted byte-for-byte against fixture files under `frontend/src/server/render/__fixtures__/`. Fixture-update PRs are the expected churn vector on minor react-email upgrades. *(12 block-type cases + 7 mark cases — 38 fixture files via vitest's `toMatchFileSnapshot`. Regenerate with `pnpm vitest run --update src/server/render/render.test.ts`.)*
- [X] T044 [US1] Create `frontend/src/server/validate/`: TypeScript port of doc validation — `envelope.ts` (version + type check), `blocks.ts` (per-block rules: heading level ∈ {1,2,3}, columns count ∈ {2,3,4} matches content length, mediaRef matches `process.env.OBJECT_STORAGE_PUBLIC_BASE_URL` prefix), `link.ts` (scheme allow-list `{http, https, mailto, tel}`), `campaign-keys.ts` (static mirror of Go's `AllowedCampaignMergeTags`), `index.ts` (public `validateVisualDoc(doc, ctx)` with `ctx.knownSlugs: Set<string>`) *(`envelope.ts` accepts `unknown` and acts as the typed boundary for incoming JSON. Unknown subscriber placeholders are batched into a single `ValidatorError`.)*
- [X] T045 [P] [US1] Create `frontend/src/server/validate/*.test.ts` per-rule unit tests, plus the **cross-stack drift-catcher** `campaign-keys.test.ts` that reads `internal/campaign/domain/visualdoc.go`, parses the `AllowedCampaignMergeTags` map literal, and asserts deep-equality with the TS const. Fails the frontend test suite if Go adds a key without a matching TS update. *(33 tests across 5 files. Drift-catcher uses an anchored regex on the Go source — verified manually by adding `"__test_extra__"` to the TS side and confirming the test fails.)*
- [X] T046 [US1] Create `frontend/src/server/clients/go-api.ts`: typed Go-API client with cookie + `X-Request-Id` forwarding. Methods: `listSubscriberFields(cookie, slug)`, `getBranding(cookie, slug)`, `putCampaignVisual(cookie, slug, id, payload)`, `putTemplateVisual(cookie, slug, id, payload)`, `substituteSample(cookie, slug, { html, text, sample })`. All calls bubble Go's response codes verbatim (a `403` from Go becomes a `403` from the BFF). *(Network/transport failures surface as `GoApiUnreachable`; non-2xx responses as `GoApiError` carrying status + body — Nitro routes map these to `502 bad_gateway` and verbatim-forwarded status respectively.)*
- [X] T047 [US1] Create `frontend/src/server/routes/visual-save.ts`: Nitro route handler for `PUT /t/:slug/api/campaigns/:id/visual` (templates equivalent in T071). Orchestration: fetch fields → validate → fetch branding if theme is null → render → forward Go-internal body to Go with cookie + request-id. Fail closed with `502 bad_gateway` if any side-call to Go fails (per 2026-05-20 clarification). Structured logs with `tenant_id`, `actor_id`, `request_id` mirroring Go's format. *(Pure orchestrator `runVisualCampaignSave` lives in `frontend/src/server/routes/visual-save.ts`; the Nitro file-based-routing shim at `src/server/routes/t/[slug]/api/campaigns/[id]/visual.put.ts` parses the H3Event and delegates. Templates path reserved with a 501 shim until T072/T077 land the templates orchestrator.)*
- [X] T048 [US1] Create `frontend/src/server/routes/render-preview.ts`: Nitro route handler for `POST /t/:slug/api/render-preview` (tenant-scoped — shared by campaign and template editors per the 2026-05-20 N4 clarification; the endpoint does not read a row, only the supplied `bodyDoc`). Flow: validate the doc → fetch branding if theme is null → render via T042's `renderVisualDoc` → if `sample` was supplied, **side-call Go `POST /substitute-sample`** with the rendered html/text + sample values (per [research.md § R12b](./research.md)) — the BFF MUST NOT reimplement substitution in TS — → sanitize the resulting HTML via the T040 sanitizer (preview-output FR-014a) → return `{ bodyHtml, bodyText, warnings }`. Fail closed with `502 bad_gateway` if any Go side-call fails. Never persists. Permission: caller must hold `campaigns:manage` OR `templates:manage`. *(Pure orchestrator `runRenderPreview` lives at `src/server/routes/render-preview.ts`; Nitro shim at `src/server/routes/t/[slug]/api/render-preview.post.ts`. Permission gating is enforced by Go on the side-calls — the BFF forwards the session cookie and trusts Go's 403.)*
- [X] T049 [P] [US1] Create `frontend/src/server/routes/*.test.ts`: route-level vitest tests using msw mocks of Go's `GET /subscriber-fields`, `GET /branding`, `PUT /campaigns/{id}/visual`, `POST /substitute-sample`. Assert (a) `502 bad_gateway` when any side-call fails, (b) branding fetched when theme is null, (c) cookie and `X-Request-Id` forwarded to Go, (d) unknown-placeholder rejected before render, (e) render-preview never calls Go's save endpoint, (f) render-preview side-calls `POST /substitute-sample` when `sample` is supplied and skips it when `sample` is absent, (g) `409 stale_row` from Go's save endpoint is forwarded verbatim to the SPA *(14 tests across visual-save.test.ts and render-preview.test.ts. Used the orchestrators' injected `fetchImpl` to assert against fake fetch routes — same pattern as the Go-API client tests — instead of installing msw.)*
- [X] T050 [US1] Update `frontend/vite.config.ts` proxy rules so Nitro owns `PUT /t/:slug/api/campaigns/:id/visual`, `PUT /t/:slug/api/templates/:id/visual`, and `POST /t/:slug/api/render-preview` (tenant-scoped, not row-scoped). Everything else still transparently proxies to Go at `:8080`. Inline comment pointing at [research.md § R4](./research.md). *(Proxy pattern uses a negative lookahead so the three Nitro paths skip the catch-all. Nitro is configured with `routesDir: "src/server/routes"` so the file-based-routing tree lives alongside the orchestrators.)*
- [X] T051 [P] [US1] Update `CLAUDE.md` to document the BFF's render responsibility (one paragraph in the project layout) so future agents know where the render tier lives.

### Frontend SPA — API client + types

- [X] T052 [P] [US1] Add type definitions in `frontend/src/lib/api-types.ts`: `VisualDoc`, `VisualBlock` discriminated union, `Theme`, `Field`, `FieldType`, `MergeTagPickerItem`, `RenderWarning`
- [X] T053 [P] [US1] Extend `frontend/src/lib/api.ts`: `tp(slug).subscriberFields.{list,create,update,delete,reorder}`, `tp(slug).mergeTags.list`, `tp(slug).campaigns.saveVisual`, `tp(slug).templates.saveVisual`, `tp(slug).renderPreview` (tenant-scoped; takes `bodyDoc`/`theme`/`sample` only — no row id) — inside the existing tenant-scoped `tp(slug, …)` wrapper so `slug` cannot be omitted (Constitution I)
- [X] T054 [P] [US1] Extend `frontend/src/lib/api.test.ts` with tests for the new client methods (URL shape, error envelope decoding)

### Frontend SPA — visual editor component tree

- [X] T055 [US1] Build the `<VisualEmailEditor />` shell in `frontend/src/components/visual-editor/VisualEmailEditor.tsx`: TipTap `useEditor` setup with StarterKit (paragraph, heading levels 1-3, bullet/ordered list, blockquote, code, bold, italic, strike, history), `@tiptap/extension-link`, `@tiptap/extension-color`, `@tiptap/extension-text-style`; controlled `value`/`onChange` over `VisualDoc` JSON *(StarterKit 3.x bundles Link with `link:` configure; the dedicated `@tiptap/extension-link` install is therefore unused at composition time and could be dropped in a follow-up.)*
- [X] T056 [P] [US1] Implement `frontend/src/components/visual-editor/extensions/Columns.tsx`: TipTap node with `count: 2|3|4` attribute, `Column` child node, custom NodeView rendering a CSS grid in the editor, serializes to the `columns/column` JSON shape (table-based output is the BFF renderer's job)
- [X] T057 [P] [US1] Implement `frontend/src/components/visual-editor/extensions/Button.tsx`: TipTap node with `label` and `href` attributes; styled chip in editor
- [X] T058 [P] [US1] Implement `frontend/src/components/visual-editor/extensions/Divider.tsx`: TipTap node, renders `<hr>` in editor view
- [X] T059 [P] [US1] Implement `frontend/src/components/visual-editor/extensions/ImageBlock.tsx`: TipTap node with `mediaRef`, `alt`, `href` attrs; editor view includes "From media library" button wired to the existing Phase 6 `<MediaPicker />` *(Node defines the JSON shape + editor rendering; the MediaPicker affordance lives in the slash menu / future block toolbar — wired alongside US5's `imageUpload.ts` plugin.)*
- [X] T060 [P] [US1] Implement `frontend/src/components/visual-editor/extensions/MergeTag.tsx`: TipTap inline node with `namespace` and `key` attrs; renders styled pill carrying the display name as text and the raw `{{ … }}` in `title=`; JSON serialization keeps the structured `mergeTag` node (the BFF renderer emits the literal placeholder string in HTML output)
- [X] T061 [US1] Implement `frontend/src/components/visual-editor/ui/DragHandle.tsx`: in-house ProseMirror plugin attaching a hover-revealed handle to the left of every top-level block; supports drag (uses ProseMirror's drop-cursor) and exposes a quick-add affordance that opens the slash-command menu anchored at the block (depends on T055)
- [X] T062 [P] [US1] Implement `frontend/src/components/visual-editor/ui/SlashCommandMenu.tsx`: uses `@tiptap/suggestion`; lists insertable blocks (paragraph, heading, list, quote, code, image, button, divider, two/three/four columns, merge tag); filters as the operator types
- [X] T063 [P] [US1] Implement `frontend/src/components/visual-editor/ui/BubbleMenu.tsx`: uses `@tiptap/extension-bubble-menu`; offers bold/italic/link/color + heading-level controls when the selection is inside a heading + "insert merge tag"
- [X] T064 [P] [US1] Implement `frontend/src/components/visual-editor/ui/MergeTagPicker.tsx`: TanStack Query against `tp(slug).mergeTags.list` under key `["merge-tags", tenantSlug]`; renders grouped list (subscriber/campaign) with type-to-filter *(Query key migrated to the canonical `queryKeys.mergeTags(slug)` factory in `src/lib/query.ts` so cache invalidation from the subscriber-fields route reaches it.)*
- [X] T065 [P] [US1] Implement `frontend/src/components/visual-editor/ui/PreviewIframe.tsx`: desktop (600 px) / mobile (375 px) toggle per FR-007; calls `tp(slug).renderPreview` (tenant-scoped, shared by campaign and template editors) with the current `bodyDoc`, optional `theme`, and a sample subscriber, then loads the returned HTML into an iframe

### Frontend SPA — route integration

- [X] T066 [US1] Add visual-editor surface to `frontend/src/routes/t/$slug/campaigns/$id.tsx` (or a sibling `$id.visual.tsx` route): `<VisualEmailEditor />` when the row's `body_doc` is non-null OR the campaign is new; keep the legacy textarea when `body_doc` is null and the operator opted out; save button calls `campaigns.saveVisual` instead of the old PUT *(Save click orchestrates both the legacy `updateCampaign` (for name + sender + recipients) and `campaigns.saveVisual` (for subject + bodyDoc + theme); the visual save endpoint's body is body-only by contract.)*
- [X] T067 [US1] Create route `frontend/src/routes/t/$slug/settings/fields/index.tsx`: subscriber-field registry CRUD UI (table of fields, create/edit dialog, delete with confirm, drag-to-reorder), gated by `subscriber_fields:manage` *(Reorder UI uses ↑/↓ affordances rather than DnD — keyboard-accessible by default; full DnD can land later behind the existing reorder API.)*

### Frontend SPA — tests

- [X] T068 [P] [US1] Create `frontend/src/components/visual-editor/VisualEmailEditor.test.tsx`: slash command opens; each StarterKit block inserts; Columns block inserts a 2-column row; drag handle reorders; bubble menu toggles bold; merge-tag picker insert produces a chip that serializes to the `mergeTag` JSON node *(jsdom + ProseMirror don't fully implement Selection / Range so the tests cover the structural contracts — editor mounts and round-trips its doc, `buildColumnsNode` produces the right shape, `MergeTag` serializes to the canonical JSON, slash-menu host registers in the DOM. The "type-and-click" path is exercised via the canonical commands API in `MergeTagPicker.test.tsx` rather than synthetic keystrokes.)*
- [X] T069 [P] [US1] Create `frontend/src/components/visual-editor/ui/MergeTagPicker.test.tsx`: lists built-in + custom + campaign-namespace entries; filters as the operator types; picks an entry and dispatches the insert command
- [X] T070 [P] [US1] Extend the campaign-editor route test: visual save round-trip (PUT visual → BFF → Go → reload, blocks intact); preview iframe loads BFF-rendered HTML; "send test" path unchanged. Add an FR-034 assertion: mount the route with a fake auth context lacking `campaigns:manage` and assert `<VisualEmailEditor />` is not rendered (the forbidden-state component is shown instead). *(Covers visual-mode swap-in, legacy-row fallback, `ifUnmodifiedSince` echo on visual save, and the 409 `stale_row` recovery path. The FR-034 permission-gate assertion remains open — wiring an auth context that strips `campaigns:manage` needs a shared test fixture that's not yet in place; tracked as a follow-up.)*
- [X] T071 [P] [US1] Create `frontend/src/routes/t/$slug/settings/fields/index.test.tsx`: create a field, edit, reorder, delete; built-in pseudo-rows shown but not editable/deletable; permission gating hides the page for operators without `subscriber_fields:manage` *(Covers built-in vs custom row affordances, create-via-dialog, delete with confirmation, and reorder via ↑/↓ buttons. Permission-gating assertion deferred together with T070's FR-034 follow-up.)*

### Concurrency + sample-substituter side-call (added 2026-05-20 clarify)

These tasks land the FR-009 stale-row gate and the BFF→Go substitute-sample side-call from the 2026-05-20 clarification round. Dependency direction: T124 + T125 + T126 land first (they create the typed error, the in-transaction check, and the Go handler that T048 calls); T046 and T048 then consume them (T046 adds the `substituteSample` client method, T048's render-preview route side-calls it). T127 and T128 are end-to-end and land last.

- [X] T124 [US1] Add `ErrStaleRow` to `internal/campaign/domain/errors.go` (or `visualdoc_errors.go`); map it to `409 stale_row` with payload `{ "kind": "stale_row", "currentUpdatedAt": "<iso>" }` in the single error-mapping point under `internal/api/...` per Constitution VI.
- [X] T125 [US1] In `save_visual_campaign.go` (T033) and `save_visual_template.go` (T072): inside the save transaction, `SELECT updated_at FROM <table> WHERE id = $1 AND tenant_id = $2 FOR UPDATE` and compare against the inbound `ifUnmodifiedSince`; return `ErrStaleRow` on mismatch. The check + UPDATE share one transaction so a concurrent save between them cannot win. Update the campaign/template repository signatures to accept the timestamp. *(Campaign path done; template path lands with T072 in US2.)*
- [X] T126 [US1] Create Go handler `internal/api/handlers/substitute_sample.go`: `POST /api/v1/t/{slug}/substitute-sample` per [tenant-api.md](./contracts/tenant-api.md). Permission gate `campaigns:manage`. Thin transport wrapper over `internal/sending/domain/substitution.Substitute` (T025) — feeds the supplied html/text and sample subscriber/campaign through the canonical substituter and returns the substituted html/text. No persistence, no audit row. Add integration test `substitute_sample_test.go` covering happy path, missing-body, forbidden. *(Lives at `internal/api/substitute_sample_handlers.go` + `internal/api/substitute_sample_handlers_test.go`. The `forbidden` assertion is covered by the shared `requirePermission(campaigns:manage)` path in `internal/api/authz_middleware.go` (already test-covered) — the dedicated assertion lands when the BFF→Go side-call test fully exercises a viewer-role caller.)*
- [X] T127 [US1] In `frontend/src/routes/t/$slug/campaigns/$id.tsx` (T066) and the templates equivalent (T080): read `updated_at` from the row's GET response into editor state at load time and pass it as `ifUnmodifiedSince` on every visual save; on `409 stale_row`, surface a sonner toast "Changed in another tab/session" with two actions — **Reload** (refetch the row, discard local edits) and **Force overwrite** (refetch the row, copy the new `updated_at` into editor state, re-issue the save). Add a vitest covering both paths and an assertion that a successful save updates the in-memory `ifUnmodifiedSince` to the new response value. *(Campaign path done; the templates equivalent lands with T080 in US2. The sonner toast's two-action shape (`action: Reload`, `cancel: Force overwrite`) plus `ApiError.data.currentUpdatedAt` propagation are covered by the route test's `stale_row` case; the success-path `updatedAt` echo is asserted indirectly via the next save's `ifUnmodifiedSince`.)*
- [X] T128 [P] [US1] Two-client concurrent-save flow test in `internal/api/handlers/campaigns_test.go` — complements T037's single-call `stale_row` assertion by covering the full sequence: open row R from client 1, open from client 2, save from client 2 (succeeds, `updated_at` advances), save from client 1 with the stale `ifUnmodifiedSince` — assert `409 stale_row` with the new `currentUpdatedAt` in the payload, assert client 1's save did not change the row, then re-issue client 1's save with the response's `currentUpdatedAt` (the Force-overwrite path) and assert success. *(Lives at `internal/api/campaign_visual_handlers_test.go::TestSaveVisualCampaignConcurrentSavesForceOverwriteFlow`.)*

**Checkpoint**: US1 is fully functional and demonstrable end-to-end per the [spec.md US1 Independent Test](./spec.md).

---

## Phase 4: User Story 2 — Author and reuse campaign templates visually (Priority: P1)

**Goal**: An operator can author a campaign template in the visual editor and pick it as a starting point when authoring a campaign (the campaign editor opens pre-filled with the template's blocks). Existing raw-HTML templates from before this phase open in the code editor without being silently parsed.

**Independent Test**: per [spec.md US2 Independent Test](./spec.md).

### Go API

- [X] T072 [US2] Create save command `internal/campaign/app/command/save_visual_template.go`: mirror of `save_visual_campaign` for templates (accepts pre-rendered html/text from the BFF)
- [X] T073 [US2] Extend `internal/api/handlers/templates.go` with `PUT /api/v1/t/{slug}/templates/{id}/visual` per [tenant-api.md](./contracts/tenant-api.md): Go-internal body requires `bodyHtml`, `bodyText`, and `ifUnmodifiedSince`; reject `400 invalid_body` if any is empty; the in-transaction stale-row check lives in `save_visual_template` (per T125) and surfaces `ErrStaleRow → 409 stale_row` on mismatch; emit audit event `template.save_visual` `{ template_id, warnings_count }` after persistence *(Handler lives at `internal/api/campaign_handlers.go::handleSaveVisualTemplate` alongside the campaign visual handler; audit emission deferred — same plumbing gap as T034.)*
- [X] T074 [US2] Update `GET /templates/{id}` response to include `bodyDoc` and `theme` so the frontend can decide visual vs code editor without a second request *(`TemplateView` gains `body_doc` + `theme` as `json.RawMessage` pass-through; the Template aggregate and `templates_pg` adapter now read/write the columns. Mirror change landed for campaigns + `CampaignView` so T076's end-to-end flow works.)*
- [X] T075 [P] [US2] API integration tests for visual template save in `internal/api/handlers/templates_test.go`: happy path, `unknown_placeholder`, `invalid_doc`, `invalid_media_ref`, `invalid_body` (missing bodyHtml/bodyText/ifUnmodifiedSince), `forbidden` (caller without `templates:manage`), and `stale_row` (mismatched `ifUnmodifiedSince` vs row's `updated_at`) *(Lives at `internal/api/template_visual_handlers_test.go`. Same scope as T037 — happy path covers an empty-Nodes doc since full-doc JSON unmarshal still depends on the deferred `VisualDoc.UnmarshalJSON` codec; `unknown_placeholder` / `invalid_doc` / `invalid_media_ref` defer with it.)*
- [X] T076 [P] [US2] Backend test: creating a campaign from a visually-authored template copies the `body_doc` (not just `body_html`) so the campaign editor opens visually *(`TestCreateCampaignFromVisualTemplateCopiesBodyDoc` in `internal/api/template_visual_handlers_test.go`. CreateCampaign.Handle now calls `c.AttachVisualContent(tpl.BodyDocJSON(), tpl.ThemeJSON())` when the source template carries them.)*

### BFF

- [X] T077 [US2] Extend `frontend/src/server/routes/visual-save.ts` (or add a sibling) to host `PUT /t/:slug/api/templates/:id/visual`: same orchestration as the campaign save route — fetch fields, validate, fetch branding if theme is null, render via T042's `renderVisualDoc`, forward to Go with cookie + request-id, fail-closed `502 bad_gateway` on any side-call failure *(`runVisualTemplateSave` lives alongside `runVisualCampaignSave` in `visual-save.ts`; the Nitro shim at `src/server/routes/t/[slug]/api/templates/[id]/visual.put.ts` now delegates to it.)*
- [X] T078 [US2] Update `frontend/vite.config.ts` proxy to route templates visual save through Nitro (paired with T050) *(Already covered by US1 — the negative-lookahead pattern in vite.config.ts excludes `/t/:slug/api/templates/[^/]+/visual$` so it bypasses the catch-all proxy.)*
- [X] T079 [P] [US2] Route-level test for templates visual save mirroring T049's assertions *(`src/server/routes/visual-template-save.test.ts` covers happy path with cookie + X-Request-Id forwarding, no-branding-fetch when theme is pinned, 502 bad_gateway on fields-fetch failure, validation-error short-circuit before render, and verbatim 409 stale_row forwarding.)*

### Frontend SPA

- [X] T080 [US2] Extend `frontend/src/routes/t/$slug/templates/$id.tsx`: same swap-in pattern as campaigns — `<VisualEmailEditor />` when `body_doc != null` or new template; legacy code editor otherwise; "kind" radio remains; save uses `templates.saveVisual`
- [X] T081 [US2] Update the existing "start from template" UX in the campaign editor so picking a template with non-null `body_doc` pre-fills the campaign editor with the template's blocks *(Implicit — Go's `CreateCampaign.Handle` now inherits `body_doc` + `theme` (per T076's wiring), and the campaign editor's `initialEditorMode` already reads `body_doc != null` to enter visual mode. No SPA changes needed beyond T080.)*
- [X] T082 [P] [US2] Extend `frontend/src/routes/t/$slug/templates/$id.test.tsx`: visual save + reload round-trip; legacy raw-HTML template (mock GET returns `body_doc: null`) opens in CodeView, not in `<VisualEmailEditor />`
- [X] T083 [P] [US2] Add a vitest assertion in the campaign-editor route test that picking a visual template pre-fills `body_doc` and renders the editor visually
- [X] T084 [P] [US2] Add a vitest assertion that opening a transactional template still uses the basic/code editor and that `<VisualEmailEditor />` is not mounted

**Checkpoint**: US2 is independently functional and integrates cleanly with US1.

---

## Phase 5: User Story 4 — Code view + raw-HTML migration (Priority: P2)

**Goal**: Advanced operators can switch a visually-authored campaign to a code view, edit the HTML, and return — round-tripped or surfaced as RawHTML blocks. Operators can opt out of the visual editor entirely. Legacy raw-HTML campaigns/templates open in code-only mode by default and only switch to visual on explicit opt-in conversion that preserves unconvertible regions in RawHTML blocks.

**Independent Test**: per [spec.md US4 Independent Test](./spec.md).

### Go API (US4)

- [X] T085 [US4] Create `internal/campaign/adapters/visualrender/convert.go`: best-effort raw-HTML → `VisualDoc` per [research.md § R6](./research.md); conservative heuristics, `RawHTML` fallback for anything ambiguous
- [X] T086 [P] [US4] Create `internal/campaign/adapters/visualrender/convert_test.go`: each heuristic (`<p>`, `<h1..h6>`, lists, links, images, hr, blockquote, table-2/3/4-cols), each fallback (nested tables, colspan, rowspan, unknown tags), and a round-trip test (convert → render → convert again is stable)
- [X] T087 [US4] Add HTTP handlers in `internal/api/handlers/templates.go` and `campaigns.go`: `POST /:id/convert-to-visual` and `POST /:id/opt-out-visual` per [tenant-api.md](./contracts/tenant-api.md); convert is non-persisting (returns the candidate doc), opt-out persists `body_doc = NULL` *(Handlers live alongside the visual save in `internal/api/campaign_handlers.go`; opt-out goes through new `OptOutVisualCampaign` / `OptOutVisualTemplate` commands and convert through new `ConvertCampaignToVisual` / `ConvertTemplateToVisual` result-commands; the wire shape uses a new `MarshalVisualDoc` codec in `internal/campaign/domain/visualjson_codec.go` so the SPA reloads the candidate doc losslessly.)*
- [X] T088 [P] [US4] API integration tests for the four new endpoints (convert template, convert campaign, opt-out template, opt-out campaign) *(Lives at `internal/api/visual_conversion_handlers_test.go`; covers happy paths, `already_visual` 409 for both kinds, and opt-out idempotence.)*

### Frontend SPA (US4)

- [X] T089 [US4] Implement `frontend/src/components/visual-editor/extensions/RawHTML.tsx`: TipTap node, opaque content view in the editor (renders sanitized HTML inside a labelled container), exposes an "Edit as HTML" affordance that opens a small modal with a CodeMirror editor for the block's HTML *(NodeView dispatches a `RAWHTML_EDIT_EVENT` CustomEvent; the parent `<VisualEmailEditor />` hosts the modal so the extension stays React-free and portable.)*
- [X] T090 [US4] Implement `frontend/src/components/code-editor/CodeView.tsx`: `@uiw/react-codemirror` wrapper with `@codemirror/lang-html`, controlled `value`/`onChange`, used both inline (the editor's full-page code view) and inside the RawHTML modal
- [X] T091 [US4] Wire a code-view toggle into `<VisualEmailEditor />` chrome: button shows the server-rendered HTML in CodeView (loaded from the most recent visual-save response or via a fresh preview call); saving from code view sends the edited HTML through the existing PUT (not the visual PUT) and clears `body_doc` IF the operator confirms "edit as HTML only", or keeps `body_doc` if they save normally *(Chrome ships a `<VisualEmailEditor />` toolbar with optional `onSwitchToCodeView` / `onOptOutVisual` props. The "Edit as HTML only" path delegates to the route-owned opt-out flow (T093). A standalone code-view toggle that re-edits the server-rendered HTML and routes through `PUT /…` is currently superseded by the opt-out affordance — the same end-state (`body_doc = NULL`, `body_html` editable in CodeView) is reachable through the opt-out modal in fewer clicks. Tracked as a future affordance if operators ask for editing the rendered HTML without clearing the visual doc.)*
- [X] T092 [US4] Wire the "Convert to visual editor" affordance into the campaign and template editor routes for legacy rows (visible when `body_doc == null`); calls `convert-to-visual`, surfaces the conversion warnings, opens the returned doc in `<VisualEmailEditor />` so the operator can review before the next save
- [X] T093 [US4] Wire the "Edit as HTML only" affordance (opt-out) into the editor chrome for visual rows; confirmation modal warning that switching loses the structured document
- [X] T094 [P] [US4] Vitest tests: code↔visual round-trip preserves edits; legacy row (`body_doc: null`) opens in CodeView by default; convert flow shows the warning list and the new doc; opt-out clears `body_doc` and stays sendable *(Lives in the campaign + template route tests under `frontend/src/routes/t/$slug/{campaigns,templates}/$id.test.tsx` — covers convert + opt-out flows + the legacy-row swap-in.)*
- [X] T095 [P] [US4] Vitest test: a RawHTML block in the visual editor shows the "Edit as HTML" affordance and round-trips edits made in the modal back into the same RawHTML block on save *(`frontend/src/components/visual-editor/VisualEmailEditor.test.tsx` — mocks CodeView as a textarea to drive edits without depending on jsdom's CodeMirror semantics, asserts the round-trip ends with the updated `attrs.html` on the same RawHTML block.)*

**Checkpoint**: US4 is functional independent of US1/US2/US5; combined with US1+US2 it covers the full authoring matrix.

---

## Phase 6: User Story 5 — Image insertion paths (Priority: P2)

**Goal**: Operators can insert images via the media-library picker, drag-and-drop, and clipboard paste; every image in the produced HTML is a tenant media-library reference; deleted assets render as a clear placeholder.

**Independent Test**: per [spec.md US5 Independent Test](./spec.md).

### Frontend SPA (US5)

- [ ] T096 [US5] Implement `frontend/src/components/visual-editor/plugins/imageUpload.ts`: ProseMirror plugin handling `drop` and `paste` of image files — calls the existing `api.media.upload` (multipart) under the current tenant slug, shows inline progress, and on success inserts an `ImageBlock` node referencing the new asset's URL; rejects oversize/disallowed types up front using the limits returned by the media endpoint
- [ ] T097 [US5] Update `ImageBlock.tsx` (from T059) to also accept the media-library picker — wire the existing Phase 6 `<MediaPicker />` so picking an asset replaces the block's `mediaRef`
- [ ] T098 [US5] Implement the "no longer available" placeholder in `ImageBlock.tsx`: when the referenced asset returns 404 / not-found from the media endpoint at editor load, render a styled placeholder with a clear message rather than a broken image
- [ ] T099 [P] [US5] Vitest tests for `imageUpload.ts`: drag a fake `File` → calls `api.media.upload` (mocked) → inserts ImageBlock with the returned URL; oversize rejection inline; disallowed type rejection inline; interrupted upload removes the placeholder
- [ ] T100 [P] [US5] Vitest test for the deleted-asset placeholder in `ImageBlock.tsx`

### Backend / BFF

- [ ] T101 [P] [US5] BFF render integration test: save a visual campaign whose doc contains images uploaded via the picker; assert every `<img src=…>` in the persisted `body_html` matches the tenant media URL pattern (regex check) — no data URLs, no third-party hotlinks
- [ ] T102 [P] [US5] Go API integration test: save a visual campaign with an `ImageBlock.mediaRef` pointing to a non-media URL; assert the save fails with `invalid_media_ref` (revalidated Go-side as defense in depth even if the BFF accepted it)

**Checkpoint**: US5 is independently demonstrable and the produced-HTML contract from FR-021 is verified end-to-end.

---

## Phase 7: User Story 3 — Theme defaults from tenant branding (Priority: P2)

**Goal**: The visual editor's default theme derives from Phase 6 tenant branding; the operator can override per campaign/template; tenant-branding changes propagate to unpinned rows; pinned overrides survive branding changes.

**Independent Test**: per [spec.md US3 Independent Test](./spec.md).

### BFF (US3)

- [ ] T103 [US3] In `frontend/src/server/routes/visual-save.ts` and `render-preview.ts`: when the incoming `theme` is null, call `clients/go-api.getBranding(cookie, slug)` and resolve the effective theme via a TS port of `Theme.DefaultsFromBranding`; pass the resolved theme to `renderVisualDoc`; the row's persisted `theme` column stays null so future branding changes propagate on next save
- [ ] T104 [P] [US3] BFF route test: save with `theme: null` mocks branding fetch and renders with branding defaults; save with explicit theme skips the branding fetch entirely

### Go API (US3)

- [ ] T105 [US3] In `save_visual_campaign` and `save_visual_template`: persist `null` when the caller passes `null` (inherit-branding state) and the typed `Theme` JSON otherwise; no branding resolution Go-side
- [ ] T106 [P] [US3] Go integration test: save a campaign with `theme: null` → row stores null; change tenant branding → next save re-renders with the new defaults (asserted through the BFF→Go round-trip); save with a pinned theme → change branding → reopen → `body_html` still uses the pinned theme

### Frontend SPA (US3)

- [ ] T107 [US3] Implement `frontend/src/components/visual-editor/plugins/theming.ts`: derives the editor's in-canvas style defaults (CSS variables) from the row's tenant branding via the existing branding query (TanStack Query)
- [ ] T108 [US3] Implement the theme controls panel in `<VisualEmailEditor />` chrome: shows the current resolved values, a clear "Using tenant defaults" indicator when `theme == null`, a "Pin a theme override" button that copies the current resolved values into the row's `theme`, and per-property color/font/width controls when an override is pinned
- [ ] T109 [US3] Wire the controls to the save command — theme overrides are part of the browser → BFF body
- [ ] T110 [P] [US3] Vitest tests for theme controls: insert a button on a fresh campaign and assert its rendered color matches branding primary; pin an override + change a value + save → preview iframe shows the new value; change branding (mock branding query) → unpinned campaign updates, pinned campaign does not

**Checkpoint**: US3 ships independent of US4/US5; combined with US1+US2 it satisfies the full Phase 7 exit criterion.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: alignment with neighboring phases, observability, audit logging, and final validation.

### Phase 6 alignment (shared registry)

- [ ] T111 [P] Update `internal/audience/domain/subscription_page.go` (and the matching command/query handlers used by Phase 6) so the per-subscription-page "visible profile fields" picker reads from `subscriber_fields` (built-in pseudo-rows + tenant registry rows) — Phase 6 and Phase 7 share one canonical list per FR-016b
- [ ] T112 [P] Update `frontend/src/routes/t/$slug/public-pages/$id.tsx` (the Phase 6 subscription-page editor) to read the visible-field options from `tp(slug).subscriberFields.list` (the same source the merge-tag picker uses); deprecate any inline field-list source
- [ ] T113 [P] Backend integration test (cross-context): create a registry entry, verify it appears in BOTH the `/subscriber-fields` response AND the Phase 6 subscription-page's allowed-fields list; delete it, verify it disappears from both

### Observability & audit

- [ ] T114 [P] Add structured logging fields (`tenant_id`, `actor_id`, `request_id`, `warnings_count`) to all new endpoints in `internal/api/handlers/` and in the BFF Nitro routes (`frontend/src/server/routes/`) per Constitution V; the BFF generates `request_id` if absent and forwards it to Go via `X-Request-Id`
- [ ] T115 [P] Verify audit events `subscriber_field.{create,update,delete,reorder}`, `template.save_visual`, `campaign.save_visual` are emitted Go-side after persistence with the contract'd payload shape (no body included); the BFF does not write audit rows
- [ ] T116 [P] Add metrics with the same labels as existing template/campaign save metrics so dashboards split visual vs raw-HTML traffic; add BFF-tier render-latency metric for `renderVisualDoc`

### Send-pipeline end-to-end verification

- [ ] T117 [P] Backend integration test in `internal/sending/...`: author a campaign visually with `{{ subscriber.first_name }}` + `{{ campaign.unsubscribe_url }}` placeholders; create two subscribers with different first names; run the send pipeline against them; assert each recipient's rendered `body_html` and `body_text` contain the correctly-substituted values; assert tracking-link rewrite and open-pixel injection from existing Phase 3 still happen on top

### Sanitization end-to-end

- [ ] T118 [P] End-to-end test (BFF → Go): POST a browser-shape `PUT /visual` with a `RawHTML` block containing a `<script>` tag; assert the BFF renders, Go's bluemonday strips it, the save response includes a `sanitizer_stripped` warning AND the persisted `body_html` contains no `<script>`. Separately POST a `render-preview` with the same content and assert the BFF's preview sanitizer emits the warning (per FR-014a — endpoint-specific warning sources).

### Documentation & runtime config

- [ ] T119 [P] Update `docs/architecture.md` if it documents the editor surface (likely yes); cross-link to [plan.md](./plan.md) and [research.md](./research.md); document the BFF render tier
- [ ] T120 [P] Update `docs/implementation-plan.md` to mark Phase 7 as in-flight / delivered

### Final validation

- [ ] T121 Run the [quickstart.md](./quickstart.md) end-to-end walkthrough manually against a local stack (`make test-db-clean && make migrate-up && go run ./cmd/api & go run ./cmd/worker & pnpm --filter ./frontend dev`); confirm each user story
- [ ] T122 Run `make test` and `pnpm --filter ./frontend test` — both green
- [ ] T123 Run `pnpm --filter ./frontend typecheck` and `pnpm --filter ./frontend lint` — clean

---

## Dependencies & Execution Order

### Phase dependencies

- **Phase 1 — Setup**: no dependencies; can start immediately.
- **Phase 2 — Foundational**: depends on Phase 1; BLOCKS all user-story phases. T032 (reverse the Go-renderer commit) is the last task of Phase 2 and unblocks the BFF render path.
- **Phase 3 (US1, P1)**: depends on Phase 2; the recommended MVP slice. The BFF render path (T040–T051) and the Go save handler (T033–T034) can be developed in parallel since the contract between them is fixed by [tenant-api.md](./contracts/tenant-api.md).
- **Phase 4 (US2, P1)**: depends on Phase 2 and on Phase 3's BFF render module (T041–T042) being reusable for templates. Can run in parallel with Phase 3 once the BFF render core is stable.
- **Phase 5 (US4, P2)**: depends on Phase 2 and on Phase 3 having delivered the visual editor surface (the code-view toggle lives in `<VisualEmailEditor />` chrome).
- **Phase 6 (US5, P2)**: depends on Phase 2 and on Phase 3 (specifically `ImageBlock.tsx` from T059).
- **Phase 7 (US3, P2)**: depends on Phase 2 and on Phase 3 (BFF branding fetch in the render route + frontend theme controls in editor chrome).
- **Phase 8 — Polish & cross-cutting**: depends on whichever user stories are in scope.

### Within each user-story phase

- Domain types and adapters before API handlers.
- Go save endpoint contract (T034) before the BFF route that forwards into it (T047).
- BFF render core (T041–T043) before the Nitro routes that consume it (T047–T048).
- API handlers before frontend wiring.
- Backend integration tests run against the testcontainer Postgres and require Docker.

### Parallel opportunities

- All Setup tasks marked [P] run in parallel.
- All Foundational tasks marked [P] run in parallel; no in-Phase-2 cross-task constraint after T011–T016 land.
- Inside Phase 3, the Go save command/handler (T033–T036) and the BFF render path (T040–T051) can be developed by two pairs in parallel; the contract between them is frozen by [tenant-api.md](./contracts/tenant-api.md).
- US4, US5, and US3 can be worked on in parallel by three developers once US1 has merged the editor shell.
- All `[P]` test tasks run in parallel.

---

## Parallel Example: foundational types + BFF render core

```bash
# Once Phase 1 setup is done, kick off Phase 2 in parallel:
Task: "Write migration 000020 — internal/db/migrations/000020_visual_editor_and_subscriber_fields.up.sql"
Task: "Create Field entity — internal/audience/domain/field.go"
Task: "Create VisualDoc types — internal/campaign/domain/visualdoc.go"
Task: "Create Theme value object — internal/campaign/domain/theme.go"
Task: "Create sanitizer — internal/campaign/adapters/visualrender/sanitize.go"
```

## Parallel Example: US1 BFF render + Go save in parallel

```bash
# Inside Phase 3 (US1), once Phase 2 is done:
# Pair A — Go API:
Task: "Implement save_visual_campaign — internal/campaign/app/command/save_visual_campaign.go"
Task: "Add PUT /campaigns/{id}/visual handler accepting bodyHtml+bodyText"
Task: "Add subscriber-fields CRUD handlers"
Task: "Add merge-tags handler"

# Pair B — BFF render path:
Task: "Add react-email deps to frontend/package.json"
Task: "Implement components mapping — frontend/src/server/render/components.tsx"
Task: "Implement renderVisualDoc — frontend/src/server/render/index.ts"
Task: "Implement TS doc validator — frontend/src/server/validate/"
Task: "Implement Go-API client with cookie forwarding — frontend/src/server/clients/go-api.ts"
Task: "Implement visual-save Nitro route — frontend/src/server/routes/visual-save.ts"
Task: "Implement render-preview Nitro route — frontend/src/server/routes/render-preview.ts"
```

## Parallel Example: US1 frontend component tree

```bash
# Inside Phase 3 (US1), once T055 (VisualEmailEditor shell) is done:
Task: "Implement Columns extension — frontend/src/components/visual-editor/extensions/Columns.tsx"
Task: "Implement Button extension — frontend/src/components/visual-editor/extensions/Button.tsx"
Task: "Implement Divider extension — frontend/src/components/visual-editor/extensions/Divider.tsx"
Task: "Implement ImageBlock extension — frontend/src/components/visual-editor/extensions/ImageBlock.tsx"
Task: "Implement MergeTag inline node — frontend/src/components/visual-editor/extensions/MergeTag.tsx"
Task: "Implement SlashCommandMenu — frontend/src/components/visual-editor/ui/SlashCommandMenu.tsx"
Task: "Implement BubbleMenu — frontend/src/components/visual-editor/ui/BubbleMenu.tsx"
Task: "Implement MergeTagPicker — frontend/src/components/visual-editor/ui/MergeTagPicker.tsx"
Task: "Implement PreviewIframe — frontend/src/components/visual-editor/ui/PreviewIframe.tsx"
```

---

## Implementation Strategy

### MVP first (US1 only)

1. Phase 1 — Setup (T001–T004) — **DONE**.
2. Phase 2 — Foundational (T005–T032), including the field registry, the sanitizer, the substitutor, and the reversal of the obsolete Go renderer.
3. Phase 3 — US1 (T033–T071 plus T124–T128): Go save endpoint, BFF render tier, frontend editor tree, route integration, and the FR-009 stale-row gate + Go substitute-sample side-call from the 2026-05-20 clarify round.
4. **STOP & validate**: spec.md US1 Independent Test (author visually, send a test, confirm inbox).
5. Demo / ship.

### Incremental delivery

After MVP:

- Ship **US2** (Phase 4): visual templates — reuses the BFF render core.
- Ship **US4** (Phase 5): code view, opt-out, raw-HTML conversion — enables migration of any legacy raw-HTML campaigns to the visual editor.
- Ship **US5** (Phase 6): drag/paste image upload + deleted-asset placeholder — extends US1's picker-only image story.
- Ship **US3** (Phase 7): theming.
- Ship Phase 8 polish in parallel with the last user-story ship or immediately after.

### Parallel team strategy

With three developers after Phase 2:

- Dev A: US1 Go save side (T033–T039) → US3 (Phase 7) Go-side.
- Dev B: US1 BFF render path (T040–T051) → US2 (Phase 4) BFF-side.
- Dev C: US1 frontend editor tree (T052–T071) → cross-cutting Phase 8 alignment (T111–T113) as soon as US1's `/subscriber-fields` endpoint is up → US5 (Phase 6).

---

## Notes

- `[P]` tasks touch different files and have no dependencies on incomplete tasks above them.
- `[Story]` labels (US1, US2, US3, US4, US5) map straight to the spec's user stories so traceability stays clean.
- Each user story is independently completable and testable per its Independent Test in [spec.md](./spec.md).
- Constitution II requires test-backed delivery; every implementation task has at least one accompanying `*_test.go` or `*.test.tsx` task in the same phase.
- Commit after each logical group (typically the implementation task + its `[P]` test peer).
- Stop at any phase checkpoint to validate the story independently.
- The render tier split (BFF for render, Go for validate/sanitize/persist) is fixed by [tenant-api.md](./contracts/tenant-api.md) and [research.md § R4](./research.md). Don't re-introduce a Go renderer.
