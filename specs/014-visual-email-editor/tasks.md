---
description: "Task list for Phase 7 — Visual Email Editor"
---

# Tasks: Phase 7 — Visual Email Editor

**Input**: Design documents from `/specs/014-visual-email-editor/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/), [quickstart.md](./quickstart.md)

**Tests**: Included — Constitution II (Test-Backed Delivery) is NON-NEGOTIABLE in this repo. Critical paths (rendering, sanitization, placeholder validation, tenant isolation, send substitution) carry integration coverage against real boundaries.

**Organization**: Tasks are grouped by user story so each story can be implemented, tested, and demoed independently.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: parallelizable (different files, no incomplete-task dependencies).
- **[Story]**: which user story this task belongs to (US1, US2, US3, US4, US5).
- Setup, Foundational, and Polish phase tasks carry no story label.

## Path conventions

Web application with Go backend (`internal/`, `cmd/`, `internal/db/migrations/`) and React SPA (`frontend/src/`). All paths in this file are repository-relative.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: get dependencies and shared scaffolding in place so any story can begin.

- [X] T001 Add Go dependency `github.com/microcosm-cc/bluemonday` to `go.mod`; verify `golang.org/x/net/html` is already present (transitive); run `go mod tidy`
- [X] T002 [P] Add frontend dependencies to `frontend/package.json`: `@tiptap/react`, `@tiptap/starter-kit`, `@tiptap/extension-bubble-menu`, `@tiptap/extension-link`, `@tiptap/extension-image`, `@tiptap/extension-color`, `@tiptap/extension-text-style`, `@tiptap/suggestion`, `@uiw/react-codemirror`, `@codemirror/lang-html`; run `pnpm --filter ./frontend install`
- [X] T003 [P] Create empty package directories: `internal/campaign/adapters/visualrender/`, `internal/audience/adapters/` (already exists; verify), `frontend/src/components/visual-editor/`, `frontend/src/components/visual-editor/extensions/`, `frontend/src/components/visual-editor/ui/`, `frontend/src/components/visual-editor/plugins/`, `frontend/src/components/code-editor/`
- [X] T004 [P] Add new permission constant `subscriber_fields:manage` to `internal/iam/domain/permission.go` and to the `Permission` union in `frontend/src/lib/permissions.ts`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: data model, domain types, server-side renderer, and substitutor extensions — everything that MUST exist before any user story can be implemented.

**⚠️ CRITICAL**: No user-story work begins until this phase is complete.

### Migration & seeds

- [X] T005 Write `internal/db/migrations/000020_visual_editor_and_subscriber_fields.up.sql` per [data-model.md § Schema delta](./data-model.md): `subscriber_fields` table with RLS, `templates.body_doc`/`templates.theme` columns, `campaigns.body_doc`/`campaigns.theme` columns
- [X] T006 Write `internal/db/migrations/000020_visual_editor_and_subscriber_fields.down.sql` reversing T005 cleanly (DROP COLUMN, DROP TABLE, DROP POLICY)
- [X] T007 [P] Seed the new `subscriber_fields:manage` permission into the existing roles-and-permissions seed flow (whatever migration / fixture path Phase 2 / 005 uses — check `internal/iam/...` for the existing seed pattern)

### Subscriber-field domain

- [X] T008 [P] Create `internal/audience/domain/field.go`: `FieldType` enum, `Field` aggregate with `id/tenantID/slug/displayName/fieldType/defaultValue/position/builtIn/createdAt/updatedAt`, validating constructor `NewField`, persistence-only `HydrateField`, getters
- [X] T009 [P] Create `internal/audience/domain/builtin_fields.go`: package-level `BuiltInFields []*Field` for `email`, `name`, `first_name`, `last_name`, `state` (constructed via `HydrateField` with `builtIn=true`)
- [X] T010 [P] Create `internal/audience/domain/field_test.go`: invariant tests for `NewField` (empty tenant, bad slug regex, empty/over-long display name, unknown type, builtIn always false on construction)

### Visual-document domain

- [X] T011 [P] Create `internal/campaign/domain/visualdoc.go`: `VisualDoc`, `Node`/`Inline` sealed interfaces (`visualNode()`/`visualInline()`), block types (`Paragraph`, `Heading`, `BulletList`, `OrderedList`, `ListItem`, `Quote`, `Code`, `Image`, `Button`, `Divider`, `Columns`, `RawHTML`), inline types (`Text`, `MergeTag`), `Marks` struct
- [X] T012 [P] Create `internal/campaign/domain/visualdoc_validate.go`: `Validate(*VisualDoc, ValidateContext) error` enforcing heading-level range, `Columns.Cols` length matches `count` (2/3/4), `Image.MediaRef` matches the tenant media URL pattern, `MergeTag.Namespace ∈ {subscriber,campaign}`, mark validity
- [X] T013 [P] Create `internal/campaign/domain/visualdoc_test.go`: positive validation cases + negative cases for every invariant above
- [X] T014 [P] Create `internal/campaign/domain/theme.go`: `Theme` value object, `NewTheme` validating constructor (CSS-color check, `containerWidth ∈ [320,800]`), `HydrateTheme`, `DefaultsFromBranding(branding.Branding) Theme`
- [X] T015 [P] Create `internal/campaign/domain/theme_test.go`: NewTheme positive + negative; DefaultsFromBranding maps Phase 6 branding correctly
- [X] T016 [P] Create `internal/campaign/domain/typed_errors.go` (or extend the existing `errors.go`): `ErrInvalidPlaceholder`, `ErrUnknownSlug`, `ErrUnsupportedNode`, `ErrSanitizationStripped`, `ErrInvalidMediaRef` typed kinds matching the contracts in [tenant-api.md](./contracts/tenant-api.md)
- [X] T017 [P] Create `internal/campaign/domain/renderer.go`: consumer-owned interfaces `Renderer` (renders doc+theme → html/text/warnings) and `FieldSet` (HasSlug)

### Template & Campaign aggregate extensions

- [X] T018 Extend `internal/campaign/domain/template.go`: add `bodyDoc *VisualDoc` and `theme *Theme` fields; add getters `BodyDoc()`, `Theme()`; add new validating constructor `NewVisualTemplate(tenantID, name, kind, subject, doc, theme, renderer Renderer, fields FieldSet) (*Template, error)` that renders, validates placeholders, sanitizes (via renderer warnings), and returns the aggregate with all three pieces populated atomically; update `HydrateTemplate` to accept `bodyDoc` and `theme` (depends on T011, T014, T017)
- [X] T019 Extend `internal/campaign/domain/campaign.go` symmetrically with `NewVisualCampaign` (depends on T018's helpers if any are shared)
- [X] T020 [P] Extend `internal/campaign/domain/template_test.go` and `campaign_test.go` (or add new files): cover `NewVisualTemplate`/`NewVisualCampaign` happy path, unknown-slug rejection, invalid-media-ref rejection, sanitizer warnings surfaced

### Renderer adapter

- [X] T021 Create `internal/campaign/adapters/visualrender/render.go`: walk the `VisualDoc` tree and emit inline-styled, table-based HTML per the [research.md § R4](./research.md) block table; emit a parallel plain-text rendering; expose `Renderer` interface implementation
- [X] T022 [P] Create `internal/campaign/adapters/visualrender/render_golden_test.go`: one canonical doc per block type (paragraph, heading×3, lists, quote, code, link mark, image, button, divider, columns×{2,3,4}, mergeTag, rawHtml) asserted byte-for-byte against golden HTML + plain text fixtures
- [X] T023 Create `internal/campaign/adapters/visualrender/sanitize.go`: bluemonday policy + email-specific deny rules (per [research.md § R5](./research.md)): strip `<script>`, `<style>`, `<iframe>`, `<object>`, `<embed>`, `<form>`, `<input>`, `<link>`, every `on*=`, every disallowed scheme; non-media-ref `<img>` rejected
- [X] T024 [P] Create `internal/campaign/adapters/visualrender/sanitize_test.go`: negative tests for every disallowed construct (must be stripped or refused regardless of placement: inside RawHTML, inside a column, inside a link)
- [X] T025 Create `internal/campaign/adapters/visualrender/placeholders.go`: `ExtractPlaceholders(doc *VisualDoc) []Placeholder`, `ValidatePlaceholders(placeholders, FieldSet) (unknown []Placeholder, err error)`; campaign-namespace placeholders validated against the package-level allow-list
- [X] T026 [P] Create `internal/campaign/adapters/visualrender/placeholders_test.go`: extraction across nested nodes (columns, list items, inline marks) and validation against a `FieldSet` test double

### Send-pipeline substitutor

- [X] T027 Extend the existing send-pipeline substitutor in `internal/sending/domain/substitution.go` (create file if absent) to recognize `{{ subscriber.<slug> }}` and `{{ campaign.<name> }}`; built-in slugs read from the `Subscriber` aggregate, custom slugs from `Subscriber.Attributes`, campaign-namespace from the supplied `CampaignContext` (`unsubscribe_url`, `preference_url`, `archive_url`, `view_in_browser_url`, `tenant_name`, `current_date`)
- [X] T028 [P] Create `internal/sending/domain/substitution_test.go`: subscriber-built-in, subscriber-custom, campaign-namespace, whitespace-tolerant parsing, unknown-slug stays literal at send (validation already happened at save)

### Field-registry adapter and CQRS handlers

- [X] T029 Create `internal/audience/adapters/fields_postgres.go`: Postgres repository for the registry implementing the command/query interfaces (uses the existing tenant-bound `pgx` transaction adapter)
- [X] T030 [P] Create `internal/audience/adapters/fields_postgres_test.go`: integration test that runs against the testcontainer Postgres, covers CRUD + reorder, and asserts tenant-isolation (Constitution I): rows for tenant A are invisible to tenant B even with the application filter omitted
- [X] T031 Create command handlers under `internal/audience/app/command/`: `create_field.go`, `update_field.go`, `delete_field.go`, `reorder_fields.go` — each thin, wrapping the repository under the tenant-bound transaction
- [X] T032 Create query handler `internal/audience/app/query/list_fields.go`: returns `BuiltInFields` prepended to the tenant's registry rows
- [X] T033 Wire new handlers in `internal/audience/app/application.go` (or whatever the existing application-composition file is called for the audience subdomain)

**Checkpoint**: Foundation ready — schema migrated, domain + renderer + substitutor compile and tests pass. User story phases can now begin in priority order or in parallel (US1+US2 first, then US4/US5 in parallel, then US3).

---

## Phase 3: User Story 1 — Author a campaign visually (Priority: P1) 🎯 MVP

**Goal**: An operator can open the campaign editor on a new campaign, author content with paragraphs/headings/lists, multi-column layouts, images from the media library, a button, formatted text via the bubble menu, and merge tags via chips; preview desktop/mobile; save; send a test that arrives in the inbox matching the preview.

**Independent Test**: per [spec.md US1 Independent Test](./spec.md) — author a campaign, send a test, confirm the inbox-rendered email matches the preview in layout, image, button, and styling.

### Backend (HTTP + commands)

- [ ] T034 [US1] Create save command `internal/campaign/app/command/save_visual_campaign.go`: accepts `(tenantID, campaignID, subject, doc, theme)`, loads the campaign, invokes `NewVisualCampaign` with the renderer + field set, persists `body_doc`, `body_html`, `body_text`, `theme` atomically through the existing campaign repository
- [ ] T035 [US1] Create `internal/campaign/app/command/render_preview.go`: accepts an unsaved doc + theme + sample subscriber/campaign data, renders, applies substitution against the sample, returns html/text/warnings — does NOT persist
- [ ] T036 [US1] Extend `internal/api/handlers/campaigns.go` with `PUT /api/v1/t/{slug}/campaigns/{id}/visual` per [tenant-api.md](./contracts/tenant-api.md); error-kind → HTTP status mapping centralized in `internal/api/...` (Constitution VI)
- [ ] T037 [US1] Extend `internal/api/handlers/campaigns.go` with `POST /api/v1/t/{slug}/campaigns/{id}/render-preview`
- [ ] T038 [US1] Create `internal/api/handlers/subscriber_fields.go` with `GET`, `POST`, `PATCH /:id`, `DELETE /:id`, `PATCH /order` per [tenant-api.md](./contracts/tenant-api.md); `subscriber_fields:manage` permission gate on mutating routes
- [ ] T039 [US1] Create `GET /api/v1/t/{slug}/merge-tags` handler in `internal/api/handlers/merge_tags.go`: returns the merged subscriber + campaign-namespace list
- [ ] T040 [P] [US1] API integration tests for `PUT /campaigns/{id}/visual` and `POST /campaigns/{id}/render-preview`: happy path, `unknown_placeholder`, `invalid_doc`, `invalid_media_ref`, `forbidden` (caller without `campaigns:manage`); tests live under `internal/api/handlers/campaigns_test.go` (or the existing pattern for that package)
- [ ] T041 [P] [US1] API integration tests for the subscriber-fields CRUD endpoints (slug conflicts, builtin-slug rejection, reorder validation, tenant-isolation)
- [ ] T042 [P] [US1] API integration test for `GET /merge-tags`

### Frontend (API client + types)

- [ ] T043 [P] [US1] Add type definitions in `frontend/src/lib/api-types.ts`: `VisualDoc`, `VisualBlock` discriminated union, `Theme`, `Field`, `FieldType`, `MergeTagPickerItem`, `RenderWarning`
- [ ] T044 [P] [US1] Extend `frontend/src/lib/api.ts`: `tp(slug).subscriberFields.{list,create,update,delete,reorder}`, `tp(slug).mergeTags.list`, `tp(slug).campaigns.saveVisual`, `tp(slug).campaigns.renderPreview` — keep them inside the existing tenant-scoped `tp(slug, …)` wrapper so `slug` cannot be omitted (Constitution I)
- [ ] T045 [P] [US1] Extend `frontend/src/lib/api.test.ts` with tests for the new client methods (URL shape, error envelope decoding)

### Frontend (visual editor component tree)

- [ ] T046 [US1] Build the `<VisualEmailEditor />` shell in `frontend/src/components/visual-editor/VisualEmailEditor.tsx`: TipTap `useEditor` setup with StarterKit (paragraph, heading levels 1-3, bullet/ordered list, blockquote, code, bold, italic, strike, history), `@tiptap/extension-link`, `@tiptap/extension-color`, `@tiptap/extension-text-style`; exposes controlled `value`/`onChange` over the `VisualDoc` JSON
- [ ] T047 [P] [US1] Implement `frontend/src/components/visual-editor/extensions/Columns.tsx`: TipTap node with `count: 2|3|4` attribute, `Column` child node, custom NodeView rendering a CSS grid in the editor, serializes to the `columns/column` JSON shape (table-based output is the SERVER renderer's job)
- [ ] T048 [P] [US1] Implement `frontend/src/components/visual-editor/extensions/Button.tsx`: TipTap node with `label` and `href` attributes; styled chip in editor
- [ ] T049 [P] [US1] Implement `frontend/src/components/visual-editor/extensions/Divider.tsx`: TipTap node, renders `<hr>` in editor view
- [ ] T050 [P] [US1] Implement `frontend/src/components/visual-editor/extensions/ImageBlock.tsx`: TipTap node with `mediaRef`, `alt`, `href` attrs; renders an editor view that includes "From media library" button — wires to the existing Phase 6 media picker (`<MediaPicker />`)
- [ ] T051 [P] [US1] Implement `frontend/src/components/visual-editor/extensions/MergeTag.tsx`: TipTap inline node with `namespace` and `key` attrs; renders styled pill carrying the display name as text and the raw `{{ … }}` in `title=`; HTML serialization emits the literal placeholder text
- [ ] T052 [US1] Implement `frontend/src/components/visual-editor/ui/DragHandle.tsx`: in-house ProseMirror plugin attaching a hover-revealed handle to the left of every top-level block; supports drag (uses ProseMirror's drop-cursor) and exposes a quick-add affordance that opens the slash-command menu anchored at the block (depends on T046)
- [ ] T053 [P] [US1] Implement `frontend/src/components/visual-editor/ui/SlashCommandMenu.tsx`: uses `@tiptap/suggestion`; lists insertable blocks (paragraph, heading, list, quote, code, image, button, divider, two/three/four columns, merge tag); filters as the operator types
- [ ] T054 [P] [US1] Implement `frontend/src/components/visual-editor/ui/BubbleMenu.tsx`: uses `@tiptap/extension-bubble-menu`; offers bold/italic/link/color + heading-level controls when the selection is inside a heading + "insert merge tag"
- [ ] T055 [P] [US1] Implement `frontend/src/components/visual-editor/ui/MergeTagPicker.tsx`: TanStack Query against `tp(slug).mergeTags.list` under key `["merge-tags", tenantSlug]`; renders grouped list (subscriber/campaign) with type-to-filter
- [ ] T056 [P] [US1] Implement `frontend/src/components/visual-editor/ui/PreviewIframe.tsx`: desktop (600 px) / mobile (375 px) toggle; calls `tp(slug).campaigns.renderPreview` with a sample subscriber and loads the returned HTML into an iframe

### Frontend (route integration)

- [ ] T057 [US1] Extend `frontend/src/routes/t/$slug/campaigns/$id.tsx`: replace the existing HTML-body field with `<VisualEmailEditor />` when the row's `body_doc` is non-null OR when the campaign is new; keep the legacy textarea when `body_doc` is null and the operator opted out; save button calls `campaigns.saveVisual` instead of the old PUT
- [ ] T058 [US1] Create route `frontend/src/routes/t/$slug/settings/fields/index.tsx`: subscriber-field registry CRUD UI (table of fields, create/edit dialog, delete with confirm, drag-to-reorder), gated by `subscriber_fields:manage`

### Frontend tests

- [ ] T059 [P] [US1] Create `frontend/src/components/visual-editor/VisualEmailEditor.test.tsx`: slash command opens; each StarterKit block inserts; Columns block inserts a 2-column row; drag handle reorders; bubble menu toggles bold; merge-tag picker insert produces a chip that serializes to `{{ subscriber.first_name }}` on save
- [ ] T060 [P] [US1] Create `frontend/src/components/visual-editor/ui/MergeTagPicker.test.tsx`: lists built-in + custom + campaign-namespace entries; filters as the operator types; picks an entry and dispatches the insert command
- [ ] T061 [P] [US1] Extend `frontend/src/routes/t/$slug/campaigns/$id.test.tsx`: visual save round-trip (PUT visual, reload, blocks intact); preview iframe loads server-rendered HTML; "send test" path unchanged
- [ ] T062 [P] [US1] Create `frontend/src/routes/t/$slug/settings/fields/index.test.tsx`: create a field, edit, reorder, delete; built-in pseudo-rows shown but not editable/deletable; permission gating hides the page for operators without `subscriber_fields:manage`

**Checkpoint**: US1 is fully functional and demonstrable end-to-end per the [spec.md US1 Independent Test](./spec.md).

---

## Phase 4: User Story 2 — Author and reuse campaign templates visually (Priority: P1)

**Goal**: An operator can author a campaign template in the visual editor and pick it as a starting point when authoring a campaign (the campaign editor opens pre-filled with the template's blocks). Existing raw-HTML templates from before this phase open in the code editor without being silently parsed.

**Independent Test**: per [spec.md US2 Independent Test](./spec.md).

### Backend

- [ ] T063 [US2] Create save command `internal/campaign/app/command/save_visual_template.go`: mirror of `save_visual_campaign` for templates
- [ ] T064 [US2] Extend `internal/api/handlers/templates.go` with `PUT /api/v1/t/{slug}/templates/{id}/visual` per [tenant-api.md](./contracts/tenant-api.md)
- [ ] T065 [US2] Update the existing `GET /templates/{id}` response to include `bodyDoc` and `theme` so the frontend can decide to open the visual or code editor without a second request
- [ ] T066 [P] [US2] API integration tests for visual template save (happy path + every typed error) in `internal/api/handlers/templates_test.go` (or existing equivalent)
- [ ] T067 [P] [US2] Backend test: creating a campaign from a visually-authored template copies the `body_doc` (not just `body_html`) so the campaign editor opens visually

### Frontend

- [ ] T068 [US2] Extend `frontend/src/routes/t/$slug/templates/$id.tsx`: same swap-in pattern as campaigns/$id.tsx — `<VisualEmailEditor />` when `body_doc != null` or new template; legacy code editor otherwise; "kind" radio remains; save uses `templates.saveVisual`
- [ ] T069 [US2] Update the existing "start from template" UX in the campaign editor (`campaigns/$id.tsx` and the template picker component) so picking a template with non-null `body_doc` pre-fills the campaign editor with the template's blocks (campaign's `body_doc` is initialised from the template's, then editable)
- [ ] T070 [P] [US2] Extend `frontend/src/routes/t/$slug/templates/$id.test.tsx`: visual save + reload round-trip; legacy raw-HTML template (mock GET returns `body_doc: null`) opens in CodeView, not in `<VisualEmailEditor />`
- [ ] T071 [P] [US2] Add a vitest assertion in the campaign-editor route test that picking a visual template pre-fills `body_doc` and renders the editor visually
- [ ] T072 [P] [US2] Add a vitest assertion that opening a transactional template still uses the basic/code editor and that `<VisualEmailEditor />` is not mounted

**Checkpoint**: US2 is independently functional and integrates cleanly with US1.

---

## Phase 5: User Story 4 — Code view + raw-HTML migration (Priority: P2)

**Goal**: Advanced operators can switch a visually-authored campaign to a code view, edit the HTML, and return — round-tripped or surfaced as RawHTML blocks. Operators can opt out of the visual editor entirely. Legacy raw-HTML campaigns/templates open in code-only mode by default and only switch to visual on explicit opt-in conversion that preserves unconvertible regions in RawHTML blocks.

**Independent Test**: per [spec.md US4 Independent Test](./spec.md).

### Backend

- [ ] T073 [US4] Create `internal/campaign/adapters/visualrender/convert.go`: best-effort raw-HTML → `VisualDoc` per [research.md § R6](./research.md); conservative heuristics, `RawHTML` fallback for anything ambiguous
- [ ] T074 [P] [US4] Create `internal/campaign/adapters/visualrender/convert_test.go`: each heuristic (`<p>`, `<h1..h6>`, lists, links, images, hr, blockquote, table-2/3/4-cols), each fallback (nested tables, colspan, rowspan, unknown tags), and a round-trip test (convert → render → convert again is stable)
- [ ] T075 [US4] Add HTTP handlers in `internal/api/handlers/templates.go` and `campaigns.go`: `POST /:id/convert-to-visual` and `POST /:id/opt-out-visual` per [tenant-api.md](./contracts/tenant-api.md); convert is non-persisting (returns the candidate doc), opt-out persists `body_doc = NULL`
- [ ] T076 [P] [US4] API integration tests for the four new endpoints (convert template, convert campaign, opt-out template, opt-out campaign)

### Frontend

- [ ] T077 [US4] Implement `frontend/src/components/visual-editor/extensions/RawHTML.tsx`: TipTap node, opaque content view in the editor (renders sanitized HTML inside a labelled container), exposes an "Edit as HTML" affordance that opens a small modal with a CodeMirror editor for the block's HTML
- [ ] T078 [US4] Implement `frontend/src/components/code-editor/CodeView.tsx`: `@uiw/react-codemirror` wrapper with `@codemirror/lang-html`, controlled `value`/`onChange`, used both inline (the editor's full-page code view) and inside the RawHTML modal
- [ ] T079 [US4] Wire a code-view toggle into `<VisualEmailEditor />` chrome: button shows the server-rendered HTML in CodeView; saving from code view sends the edited HTML through the existing PUT (not the visual PUT) and clears `body_doc` IF the operator confirms "edit as HTML only", or keeps `body_doc` and converts back if they save normally
- [ ] T080 [US4] Wire the "Convert to visual editor" affordance into the campaign and template editor routes for legacy rows (visible when `body_doc == null`); calls `convert-to-visual`, surfaces the conversion warnings, opens the returned doc in `<VisualEmailEditor />` so the operator can review before the next save
- [ ] T081 [US4] Wire the "Edit as HTML only" affordance (opt-out) into the editor chrome for visual rows; confirmation modal warning that switching loses the structured document
- [ ] T082 [P] [US4] Vitest tests: code↔visual round-trip preserves edits (either round-tripped or surfaced as a RawHTML block); legacy row (`body_doc: null`) opens in CodeView by default; convert flow shows the warning list and the new doc; opt-out clears `body_doc` and stays sendable
- [ ] T083 [P] [US4] Vitest test: a RawHTML block in the visual editor shows the "Edit as HTML" affordance and round-trips edits made in the modal back into the same RawHTML block on save

**Checkpoint**: US4 is functional independent of US1/US2/US5; combined with US1+US2 it covers the full authoring matrix (visual / mixed / code-only / legacy / converted).

---

## Phase 6: User Story 5 — Image insertion paths (Priority: P2)

**Goal**: Operators can insert images via the media-library picker, drag-and-drop, and clipboard paste; every image in the produced HTML is a tenant media-library reference; deleted assets render as a clear placeholder.

**Independent Test**: per [spec.md US5 Independent Test](./spec.md).

### Frontend

- [ ] T084 [US5] Implement `frontend/src/components/visual-editor/plugins/imageUpload.ts`: ProseMirror plugin handling `drop` and `paste` of image files — calls the existing `api.media.upload` (multipart) under the current tenant slug, shows inline progress, and on success inserts an `ImageBlock` node referencing the new asset's URL; rejects oversize/disallowed types up front using the limits returned by the media endpoint (re-using whatever Phase 6 already exposes)
- [ ] T085 [US5] Update `ImageBlock.tsx` (from T050) to also accept the media-library picker — wire the existing Phase 6 `<MediaPicker />` so picking an asset replaces the block's `mediaRef`
- [ ] T086 [US5] Implement the "no longer available" placeholder in `ImageBlock.tsx`: when the referenced asset returns 404 / not-found from the media endpoint at editor load, render a styled placeholder with a clear message rather than a broken image
- [ ] T087 [P] [US5] Vitest tests for `imageUpload.ts`: drag a fake `File` → calls `api.media.upload` (mocked) → inserts ImageBlock with the returned URL; oversize rejection inline; disallowed type rejection inline; interrupted upload removes the placeholder
- [ ] T088 [P] [US5] Vitest test for the deleted-asset placeholder in `ImageBlock.tsx`

### Backend

- [ ] T089 [P] [US5] Backend integration test: save a visual campaign that contains images uploaded via the picker; assert that every `<img src=…>` in the persisted `body_html` matches the tenant media URL pattern (regex check) — no data URLs, no third-party hotlinks
- [ ] T090 [P] [US5] Backend integration test: save a visual campaign with an `ImageBlock.mediaRef` pointing to a non-media URL; assert the save fails with `invalid_media_ref`

**Checkpoint**: US5 is independently demonstrable and the produced-HTML contract from FR-021 is verified end-to-end.

---

## Phase 7: User Story 3 — Theme defaults from tenant branding (Priority: P2)

**Goal**: The visual editor's default theme derives from Phase 6 tenant branding; the operator can override per campaign/template; tenant-branding changes propagate to unpinned rows; pinned overrides survive branding changes.

**Independent Test**: per [spec.md US3 Independent Test](./spec.md).

### Backend

- [ ] T091 [US3] In the save command (`save_visual_template`, `save_visual_campaign`) accept the `theme` field; persist `null` when the caller passes `null` (inherit-branding state) and the typed `Theme` JSON otherwise; renderer reads the row's theme or invokes `Theme.DefaultsFromBranding` on null
- [ ] T092 [US3] Wire the branding lookup into the renderer call path: the save handler loads the row's tenant branding once and supplies it to `Renderer.Render(doc, theme)` so the renderer can resolve defaults without reaching into the DB itself (keeps the renderer pure)
- [ ] T093 [P] [US3] Backend integration test: save a campaign with `theme: null` → render uses branding defaults; change tenant branding → reopen the campaign (GET) → `body_html` re-renders with the new defaults on next save; save with a pinned theme → change branding → reopen → `body_html` still uses the pinned theme

### Frontend

- [ ] T094 [US3] Implement `frontend/src/components/visual-editor/plugins/theming.ts`: derives the editor's in-canvas style defaults (CSS variables) from the row's tenant branding via the existing branding query (TanStack Query)
- [ ] T095 [US3] Implement the theme controls panel in `<VisualEmailEditor />` chrome: shows the current resolved values, a clear "Using tenant defaults" indicator when `theme == null`, a "Pin a theme override" button that copies the current resolved values into the row's `theme`, and per-property color/font/width controls when an override is pinned
- [ ] T096 [US3] Wire the controls to the save command — theme overrides are part of the `PUT /visual` body
- [ ] T097 [P] [US3] Vitest tests for theme controls: insert a button on a fresh campaign and assert its rendered color matches branding primary; pin an override + change a value + save → preview iframe shows the new value; change branding (mock branding query) → unpinned campaign updates, pinned campaign does not

**Checkpoint**: US3 ships independent of US4/US5; combined with US1+US2 it satisfies the full Phase 7 exit criterion.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: alignment with neighboring phases, observability, audit logging, and final validation.

### Phase 6 alignment (shared registry)

- [ ] T098 [P] Update `internal/audience/domain/subscription_page.go` (and the matching command/query handlers used by Phase 6) so the per-subscription-page "visible profile fields" picker reads from `subscriber_fields` (built-in pseudo-rows + tenant registry rows) — Phase 6 and Phase 7 share one canonical list per FR-016b
- [ ] T099 [P] Update `frontend/src/routes/t/$slug/public-pages/$id.tsx` (the Phase 6 subscription-page editor) to read the visible-field options from `tp(slug).subscriberFields.list` (the same source the merge-tag picker uses); deprecate any inline field-list source
- [ ] T100 [P] Backend integration test (cross-context): create a registry entry, verify it appears in BOTH the `/subscriber-fields` response AND the Phase 6 subscription-page's allowed-fields list; delete it, verify it disappears from both

### Observability & audit

- [ ] T101 [P] Add structured logging fields (`tenant_id`, `actor_id`, `request_id`, `warnings_count`) to all new endpoints in `internal/api/handlers/` per Constitution V; mirror the existing patterns
- [ ] T102 [P] Emit audit events from the new handlers (per [tenant-api.md § Audit events](./contracts/tenant-api.md)): `subscriber_field.{create,update,delete,reorder}`, `template.save_visual`, `campaign.save_visual` — body NOT included
- [ ] T103 [P] Add metrics with the same labels as existing template/campaign save metrics so dashboards split visual vs raw-HTML traffic

### Send-pipeline end-to-end verification

- [ ] T104 [P] Backend integration test in `internal/sending/...`: author a campaign visually with `{{ subscriber.first_name }}` + `{{ campaign.unsubscribe_url }}` placeholders; create two subscribers with different first names; run the send pipeline against them; assert each recipient's rendered `body_html` and `body_text` contain the correctly-substituted values; assert tracking-link rewrite and open-pixel injection from existing Phase 3 still happen on top

### Sanitization end-to-end

- [ ] T105 [P] End-to-end test: POST a `PUT /visual` with a `RawHTML` block containing a `<script>` tag; assert the response includes a `sanitizer_stripped` warning AND the persisted `body_html` contains no `<script>`

### Documentation & runtime config

- [ ] T106 [P] Update `docs/architecture.md` if it documents the editor surface (likely yes — check the existing file); cross-link to [plan.md](./plan.md) and [research.md](./research.md)
- [ ] T107 [P] Update `docs/implementation-plan.md` to mark Phase 7 as in-flight / delivered

### Final validation

- [ ] T108 Run the [quickstart.md](./quickstart.md) end-to-end walkthrough manually against a local stack (`make test-db-clean && make migrate-up && go run ./cmd/api & go run ./cmd/worker & pnpm --filter ./frontend dev`); confirm each user story
- [ ] T109 Run `make test` and `pnpm --filter ./frontend test` — both green
- [ ] T110 Run `pnpm --filter ./frontend typecheck` and `pnpm --filter ./frontend lint` — clean

---

## Dependencies & Execution Order

### Phase dependencies

- **Phase 1 — Setup**: no dependencies; can start immediately.
- **Phase 2 — Foundational**: depends on Phase 1; BLOCKS all user-story phases.
- **Phase 3 (US1, P1)**: depends on Phase 2; the recommended MVP slice.
- **Phase 4 (US2, P1)**: depends on Phase 2; can start in parallel with Phase 3 once Phase 2 is done (different routes/handlers).
- **Phase 5 (US4, P2)**: depends on Phase 2 and on Phase 3 having delivered the visual editor surface (the code-view toggle lives in `<VisualEmailEditor />` chrome). May be developed in parallel with Phase 6 (US5) and Phase 7 (US3) once US1 is integrated.
- **Phase 6 (US5, P2)**: depends on Phase 2 and on Phase 3 (specifically `ImageBlock.tsx` from T050).
- **Phase 7 (US3, P2)**: depends on Phase 2 and on Phase 3 (theme controls live in editor chrome; backend renderer needs the `theme` plumbing from T091/T092 layered on the foundation).
- **Phase 8 — Polish & cross-cutting**: depends on whichever user stories are in scope.

### Within each user-story phase

- Domain types and adapters before API handlers.
- API handlers before frontend wiring.
- Component tests + route tests run after their target compiles; vitest does not need a backend to be up.
- Backend integration tests run against the testcontainer Postgres and require Docker.

### Parallel opportunities

- All Setup tasks marked [P] run in parallel.
- All Foundational tasks marked [P] run in parallel; the renderer (T021) is the only ordering constraint inside Phase 2 (it depends on the domain types it traverses).
- Once Phase 2 is done, US1 (Phase 3) and US2 (Phase 4) can be worked on by two developers in parallel — they touch disjoint routes/handlers and share only the foundational layer.
- US4, US5, and US3 can be worked on in parallel by three developers once US1 has merged the editor shell.
- All `[P]` test tasks run in parallel.

---

## Parallel Example: foundational renderer + frontend setup

```bash
# Once Phase 1 setup is done, kick off Phase 2 in parallel:
Task: "Write migration 000020 — internal/db/migrations/000020_visual_editor_and_subscriber_fields.up.sql"
Task: "Create Field entity — internal/audience/domain/field.go"
Task: "Create VisualDoc types — internal/campaign/domain/visualdoc.go"
Task: "Create Theme value object — internal/campaign/domain/theme.go"
Task: "Configure visual-editor folder structure — frontend/src/components/visual-editor/"
```

## Parallel Example: US1 frontend component tree

```bash
# Inside Phase 3 (US1), once T046 (VisualEmailEditor shell) is done:
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

1. Phase 1 — Setup (T001–T004).
2. Phase 2 — Foundational (T005–T033), including the field registry, the renderer, and the sanitizer.
3. Phase 3 — US1 (T034–T062).
4. **STOP & validate**: spec.md US1 Independent Test (author visually, send a test, confirm inbox).
5. Demo / ship.

### Incremental delivery

After MVP:

- Ship **US2** (Phase 4): visual templates.
- Ship **US4** (Phase 5): code view, opt-out, raw-HTML conversion — enables migration of any legacy raw-HTML campaigns to the visual editor.
- Ship **US5** (Phase 6): drag/paste image upload + deleted-asset placeholder — extends US1's picker-only image story.
- Ship **US3** (Phase 7): theming.
- Ship Phase 8 polish in parallel with the last user-story ship or immediately after.

### Parallel team strategy

With three developers after Phase 2:

- Dev A: US1 (Phase 3) → US3 (Phase 7).
- Dev B: US2 (Phase 4) → US4 (Phase 5).
- Dev C: cross-cutting Phase 8 alignment (T098–T100) as soon as US1's `/subscriber-fields` endpoint is up → US5 (Phase 6).

---

## Notes

- `[P]` tasks touch different files and have no dependencies on incomplete tasks above them.
- `[Story]` labels (US1, US2, US3, US4, US5) map straight to the spec's user stories so traceability stays clean.
- Each user story is independently completable and testable per its Independent Test in [spec.md](./spec.md).
- Constitution II requires test-backed delivery; every implementation task has at least one accompanying `*_test.go` or `*.test.tsx` task in the same phase.
- Commit after each logical group (typically the implementation task + its `[P]` test peer).
- Stop at any phase checkpoint to validate the story independently.
- Avoid: vague tasks, cross-story dependencies that break independence, and any task that touches the file another in-flight task is editing.
