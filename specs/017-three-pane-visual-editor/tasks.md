---

description: "Task list for Three-Pane Visual Editor — Structure Outline & Block Parameters"
---

# Tasks: Three-Pane Visual Editor — Structure Outline & Block Parameters

**Input**: Design documents from `/specs/017-three-pane-visual-editor/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: INCLUDED — the constitution makes test-backed delivery non-negotiable
(Principle II) and plan.md specifies tests per increment (render goldens,
validator bounds, sanitizer survival, selection-sync, lossless round-trip).

**Organization**: Tasks are grouped by user story (US1 → US3) so each story is
independently implementable, testable, and shippable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: US1 / US2 / US3 (Setup/Foundational/Polish carry no story label)
- All paths are repository-root-relative (frontend under `frontend/`, backend under `internal/`)

## Context (from plan.md / research.md)

This wraps the existing feature-014 visual editor (`frontend/src/components/visual-editor/`)
in a three-pane shell. **No DB migration, no new HTTP endpoint, no new send path.**
Per-block style is an optional `BlockStyle` value object inside the existing
`body_doc jsonb`, threaded through the 014 pipeline: TipTap attrs → SPA `VisualDoc`
→ BFF react-email render → BFF + Go validators → Go bluemonday sanitizer →
persisted HTML/text. New dependency: `react-resizable-panels` (MIT, via shadcn).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add the one new dependency and i18n scaffolding.

- [X] T001 [P] Add the shadcn `resizable` UI primitive via `cd frontend && pnpm dlx shadcn@latest add resizable` — creates `frontend/src/components/ui/resizable.tsx` and adds the `react-resizable-panels` dependency to `frontend/package.json`
- [X] T002 [P] Add baseline panel/outline/params key sections (`structure.*`, `params.*`, `panel.*`) to `frontend/src/locales/en/visualEditor.json` and `frontend/src/locales/ru/visualEditor.json`, keeping en/ru namespace parity

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared selection model + three-pane shell scaffold that US1, US2, and US3 all build on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T003 Refactor `frontend/src/components/visual-editor/VisualEmailEditor.tsx` to expose its TipTap editor instance to a parent via a new optional `onEditorReady?(editor: Editor) => void` prop, without changing existing behavior or the `value`/`onChange` contract
- [X] T004 Create the shared selection hook in `frontend/src/components/visual-editor/hooks/useBlockSelection.ts` — owns `selectedPos` (mapping-stable across transactions), a derived `selectedNode` snapshot, `selectBlock(pos)` (sets node/text selection + `scrollIntoView`), and `clear()`; clears selection when the mapped pos no longer resolves to a block (depends on T003)
- [X] T005 [P] Test `useBlockSelection` in `frontend/src/components/visual-editor/hooks/useBlockSelection.test.ts` — selection updates from canvas clicks, `selectedPos` remaps after edits/reorders, and selection clears when the selected block is deleted
- [X] T006 Create the minimal three-pane shell `frontend/src/components/visual-editor/ThreePaneEditor.tsx` — a horizontal `ResizablePanelGroup` (left | center | right) that wraps `VisualEmailEditor`, owns `useBlockSelection`, and renders `left`/`right` slot props (collapse/resize/persistence are added in US3); add three-pane layout rules to `frontend/src/components/visual-editor/visual-editor.css` (depends on T004)
- [X] T007 Add the selected-block canvas affordance: a `ve-selected` decoration driven by `useBlockSelection.selectedPos` plus click-to-select wiring, in `frontend/src/components/visual-editor/VisualEmailEditor.tsx` and `visual-editor.css` (depends on T004)

**Checkpoint**: A static three-pane shell renders with the existing canvas in the center and a working single-selection model shared across panes.

---

## Phase 3: User Story 1 - Fine-tune a selected block's appearance (Priority: P1) 🎯 MVP

**Goal**: Select a block, fine-tune its email-safe style in the right panel, see the canvas update live, persist it, and render it into the delivered email.

**Independent Test**: Open the campaign editor, select a button, change its background color / corner radius / font weight in the right panel, confirm the canvas updates live, save, send a test, and confirm the inbox button reflects the change. (Works with canvas-click selection — needs neither the outline nor collapsible chrome.)

### Tests for User Story 1 ⚠️ (write before / alongside implementation)

- [X] T008 [P] [US1] Go validator tests for `BlockStyle` bounds and per-type applicability in `internal/campaign/domain/visualdoc_style_test.go` — assert valid styles pass, out-of-range/malformed/wrong-type-for-block styles return the `invalid_style` typed error
- [X] T009 [P] [US1] Go sanitizer tests in `internal/campaign/visualrender/sanitize_test.go` — assert every `BlockStyle`-derived inline-CSS property survives the bluemonday pass while a hostile property (`position`, `behavior`, `expression(...)`) is stripped (extend existing test file)
- [X] T010 [P] [US1] BFF validator tests in `frontend/src/server/validate/blocks.test.ts` — `style` bounds + per-type applicability + `invalid_style` rejection
- [X] T011 [P] [US1] BFF render golden tests in `frontend/src/server/render/render.test.ts` — one styled variant per styleable block asserts the per-block inline CSS is emitted (theme base + style override; container styles on the table cell)

### Implementation for User Story 1 — Backend (Go)

- [X] T012 [P] [US1] Add the `BlockStyle` struct and a `Style *BlockStyle` field to the styleable block attr structs (paragraph, heading, blockquote, lists, button, image, divider, columns, column) in `internal/campaign/domain/visualdoc.go`
- [X] T013 [US1] Add the `ErrInvalidStyle` typed error (`apperr.NewIncorrectInput("invalid_style", …)`) in `internal/campaign/domain/visualdoc_errors.go` (depends on T012)
- [X] T014 [US1] Validate `BlockStyle` bounds (color regex, numeric ranges, enum literals, font allow-list) and per-block-type applicability in `internal/campaign/domain/visualdoc_validate.go`, returning `ErrInvalidStyle` (depends on T012, T013)
- [X] T015 [US1] Extend the Go sanitizer in `internal/campaign/visualrender/sanitize.go` to allow exactly the `BlockStyle`-producible inline-CSS properties (explicit `AllowAttrs("style")` + per-property `AllowStyles`/matchers per research R5) while still stripping everything else (depends on T012)

### Implementation for User Story 1 — BFF (TanStack/Nitro render + validate)

- [X] T016 [P] [US1] Add the `BlockStyle` type and an optional `style` field on the styleable block attrs in `frontend/src/server/render/types.ts`
- [X] T017 [US1] Add `style` bounds + per-type applicability checks in `frontend/src/server/validate/blocks.ts` (mirrors the Go validator; emits `invalid_style`) (depends on T016)
- [X] T018 [US1] Add a `mapBlockStyle(style): React.CSSProperties` helper and merge `style` over the theme defaults in every styleable block renderer in `frontend/src/server/render/components.tsx`, applying container styles (bg/padding/border/radius) to the table-cell/wrapper so Outlook honors them (depends on T016)
- [X] T019 [US1] Add styled-variant fixtures per styleable block under `frontend/src/server/render/__fixtures__/` (`*-styled.html` / `*-styled.txt`) to back the T011 golden assertions (depends on T018)

### Implementation for User Story 1 — Shared font allow-list (cross-stack)

- [X] T020 [P] [US1] Define the curated font allow-list once: `frontend/src/server/validate/fonts.ts` (TS const + email-safe fallback stacks) and an `AllowedFontFamilies` map in `internal/campaign/domain/visualdoc_validate.go`; add a drift-catcher `frontend/src/server/validate/fonts.test.ts` that parses the Go source and asserts deep equality (mirrors 014's `campaign-keys.test.ts` pattern)

### Implementation for User Story 1 — Frontend SPA editor

- [X] T021 [P] [US1] Add `BlockStyle` and an optional `style` field on the styleable block attrs in `frontend/src/lib/api-types.ts`
- [X] T022 [US1] Create the shared style-attr helper `frontend/src/components/visual-editor/extensions/styleAttr.ts` (`addAttributes`/parse/renderHTML round-trip for `style`) and apply it to the StarterKit nodes (paragraph, heading, lists, blockquote) via the extension config in `VisualEmailEditor.tsx` (depends on T021)
- [X] T023 [P] [US1] Extend the custom node extensions to carry the `style` attr (parse/render round-trip): `frontend/src/components/visual-editor/extensions/Button.tsx`, `Columns.tsx`, `Divider.tsx`, `ImageBlock.tsx` (depends on T021)
- [X] T024 [US1] Create the shared constrained controls `frontend/src/components/visual-editor/panels/params/StyleControls.tsx` — color pickers, bounded numeric steppers/sliders (radius/padding/size/border), font dropdown (from the allow-list), weight + alignment toggles (no free-form entry)
- [X] T025 [P] [US1] Create the per-block-type param forms in `frontend/src/components/visual-editor/panels/params/`: `ButtonParams.tsx`, `ImageParams.tsx`, `TextParams.tsx`, `DividerParams.tsx`, `ColumnsParams.tsx` (each exposes its applicable subset per the data-model matrix; depends on T024)
- [X] T026 [US1] Create `frontend/src/components/visual-editor/panels/BlockParamsPanel.tsx` — dispatches to the per-type form by selected node type, shows a neutral empty state when nothing is selected, offers per-field and whole-block reset-to-default, and applies changes to the selected block via a `setNodeAttrs(pos, …)` transaction (depends on T004, T024, T025)
- [X] T027 [US1] Mount `BlockParamsPanel` in the `ThreePaneEditor` right region (depends on T006, T026)
- [X] T028 [P] [US1] Component tests in `frontend/src/components/visual-editor/panels/BlockParamsPanel.test.tsx` — correct control set per block type, live-apply to the selected block only (no other block changes), empty state, reset-to-default, constrained inputs
- [X] T029 [P] [US1] Storybook stories + story tests for `BlockParamsPanel` and `StyleControls` in `frontend/src/components/visual-editor/panels/BlockParamsPanel.stories.tsx` (preview-stories + run-story-tests per the Storybook workflow)

### Implementation for User Story 1 — Persistence & integration

- [X] T030 [US1] Extend the existing visual save/load round-trip + tenant-isolation test (in `internal/api/`, e.g. the visual-save test) to carry per-block `style` and assert it round-trips losslessly and stays tenant-isolated (depends on T012, T014)
- [X] T031 [US1] Swap `VisualEmailEditor` → `ThreePaneEditor` at the mount points `frontend/src/routes/t/$slug/campaigns/$id.tsx` and `frontend/src/routes/t/$slug/templates/$id.tsx` (forwarding the same props; the canvas stays inside) (depends on T027)

**Checkpoint**: US1 is fully functional — select any block, fine-tune its email-safe parameters, save, and the delivered email reflects them. This is the shippable MVP.

---

## Phase 4: User Story 2 - Navigate the document via the structure outline (Priority: P2)

**Goal**: A left-panel outline mirroring the document tree, with click-to-select/scroll, drag-to-reorder, delete/duplicate, and collapsible containers — all in sync with the canvas and params panel.

**Independent Test**: Build a campaign with a 3-column layout holding nested blocks, open the left panel, confirm the outline mirrors the hierarchy, click a nested entry and confirm the canvas selects it, then reorder and delete a block from the outline and confirm the canvas + output reflect the change.

### Tests for User Story 2 ⚠️

- [X] T032 [P] [US2] Component tests in `frontend/src/components/visual-editor/panels/StructureOutline.test.tsx` — projection fidelity for nested columns, label derivation per type, click-to-select sync (outline ⇄ canvas ⇄ params), reorder via DnD, invalid-target rejection, delete/duplicate, container collapse

### Implementation for User Story 2

- [X] T033 [US2] Create `frontend/src/components/visual-editor/panels/StructureOutline.tsx` — derives an indented tree live from the controlled `VisualDoc` (one entry per block with its `pos`), labels each entry by type + a content-derived label, and highlights the selected entry via `useBlockSelection` (depends on T004, T006)
- [X] T034 [US2] Wire click-to-select: clicking an entry calls `useBlockSelection.selectBlock(pos)` (selects + scrolls the canvas + loads params) in `StructureOutline.tsx` (depends on T033)
- [X] T035 [US2] Implement reorder via native HTML5 drag-and-drop on outline rows → a TipTap node-move transaction at the computed positions (the same move primitive the canvas `DragHandle` uses), rejecting invalid targets (e.g. a columns container dropped into its own column) before dispatch, in `StructureOutline.tsx` (depends on T033)
- [X] T036 [US2] Add per-entry delete and duplicate editor commands in `StructureOutline.tsx` (depends on T033)
- [X] T037 [US2] Add container collapse/expand (local view-only state) for `columns`/`column` entries in `StructureOutline.tsx` (depends on T033)
- [X] T038 [US2] Mount `StructureOutline` in the `ThreePaneEditor` left region (depends on T006, T033)
- [X] T039 [P] [US2] Storybook stories + story tests for `StructureOutline` in `frontend/src/components/visual-editor/panels/StructureOutline.stories.tsx`

**Checkpoint**: US1 + US2 both work independently — the outline navigates and edits structure while staying in sync with the canvas and params panel.

---

## Phase 5: User Story 3 - Collapse and restore the side panels (Priority: P2)

**Goal**: Independently collapsible/resizable side panels with per-operator layout persistence and a graceful narrow-viewport fallback.

**Independent Test**: Collapse each panel and confirm the canvas widens; expand both; reload and confirm the layout is remembered; shrink the viewport and confirm the canvas stays usable.

### Tests for User Story 3 ⚠️

- [X] T040 [P] [US3] Component tests in `frontend/src/components/visual-editor/ThreePaneEditor.test.tsx` — collapse/expand each panel reflows the canvas, re-expand restores prior width with content intact, layout persists across remount (`autoSaveId`), and at ≤ 1024 px the panels collapse by default / open as overlays with no canvas horizontal overflow

### Implementation for User Story 3

- [X] T041 [US3] Enhance `frontend/src/components/visual-editor/ThreePaneEditor.tsx` — make the left/right panels `collapsible` with imperative collapse/expand handles, add always-available collapse + re-expand affordances, and persist collapsed state + panel widths via `react-resizable-panels` `autoSaveId="ve-three-pane"` (depends on T006)
- [X] T042 [US3] Add the narrow-viewport (≤ ~1024 px) responsive path in `ThreePaneEditor.tsx` — default the side panels to collapsed and present them as overlays using the existing shadcn `sheet` primitive, keeping the canvas above its minimum usable width (depends on T041)
- [X] T043 [P] [US3] Storybook stories + story tests for `ThreePaneEditor` (expanded, one collapsed, both collapsed, narrow-viewport overlay) in `frontend/src/components/visual-editor/ThreePaneEditor.stories.tsx`

**Checkpoint**: All three stories are independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Finalize i18n, run the full verification bundle, validate the quickstart.

- [X] T044 [P] Finalize all new en/ru strings in `frontend/src/locales/{en,ru}/visualEditor.json` and confirm `frontend/src/i18n/catalog-parity.test.ts` passes
- [X] T045 Run the verification bundle and fix failures: `go test ./...`; `cd frontend && pnpm test && pnpm lint && pnpm typecheck`
- [X] T046 [P] Walk the [quickstart.md](./quickstart.md) manual checklist (US1/US2/US3) and confirm the spec acceptance scenarios + "what did NOT change" sanity checks (no migration, no new endpoint, worker untouched, pre-017 rows render unchanged)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies — start immediately.
- **Foundational (Phase 2)**: depends on Setup (needs the `resizable` primitive); **BLOCKS all user stories**.
- **User Stories (Phase 3–5)**: all depend on Foundational. US1 is the MVP; US2 and US3 each layer on the same shell and selection hook but are independently testable.
- **Polish (Phase 6)**: depends on the desired user stories being complete.

### User Story Dependencies

- **US1 (P1)**: starts after Foundational. Self-contained (canvas-click selection + params + style pipeline). No dependency on US2/US3.
- **US2 (P2)**: starts after Foundational. Reuses `useBlockSelection` (T004) and the shell (T006); independently testable.
- **US3 (P2)**: starts after Foundational. Enhances the shell (T006); independently testable. Does not depend on US1/US2 content.

### Within User Story 1

- Backend chain: T012 → T013 → T014; T012 → T015. Tests T008/T009 validate them.
- BFF chain: T016 → T017; T016 → T018 → T019. Tests T010/T011 validate them.
- Frontend chain: T021 → {T022, T023}; T024 → T025 → T026 → T027; T027 → T031.
- T030 (round-trip) depends on T012/T014.

### Parallel Opportunities

- **Setup**: T001 and T002 run in parallel.
- **Foundational**: T005 runs parallel to T006/T007 once T004 lands.
- **US1 across stacks**: the Go track (T012–T015 + tests T008/T009), the BFF track (T016–T019 + tests T010/T011), the font allow-list (T020), and the frontend-types start (T021) are largely independent files and can proceed in parallel; they converge at the panel (T026) and round-trip (T030).
- **[P] within US1**: T008, T009, T010, T011 (tests, different files); T012/T016/T020/T021 (type additions, different files); T023, T025, T028, T029.
- **US2 vs US3**: once Foundational is done, US2 and US3 can be built in parallel by different developers (different files: `StructureOutline.*` vs `ThreePaneEditor.*`).

---

## Parallel Example: User Story 1 kickoff

```bash
# Cross-stack type additions + test scaffolds in parallel (different files):
Task: "T012 Add BlockStyle struct in internal/campaign/domain/visualdoc.go"
Task: "T016 Add BlockStyle type in frontend/src/server/render/types.ts"
Task: "T020 Define shared font allow-list (TS + Go) + drift-catcher"
Task: "T021 Add BlockStyle to frontend/src/lib/api-types.ts"
Task: "T008 Go validator style-bounds tests"
Task: "T010 BFF validator style-bounds tests"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1 Setup → 2. Phase 2 Foundational → 3. Phase 3 US1 → **STOP & validate**: select a block, restyle it, save, send a test, confirm the inbox reflects it. Ship.

### Incremental Delivery

1. Setup + Foundational → shell + selection ready.
2. US1 → params panel + style pipeline → ship MVP.
3. US2 → structure outline → ship.
4. US3 → collapsible/resizable/persistent panels → ship.
5. Polish → i18n parity + full verification + quickstart walk.

---

## Notes

- `[P]` = different files, no dependency on an incomplete task.
- `[Story]` label maps each task to its user story for traceability.
- Per-block `style` requires touching matched layers (TS types ↔ Go domain ↔ both validators ↔ renderer ↔ sanitizer); keep them in sync — the golden tests and the font drift-catcher are the guardrails.
- No DB migration, no new HTTP endpoint, no `cmd/worker` change. If a task implies any of those, re-read plan.md — it is out of scope.
- Commit after each task or logical group; stop at any checkpoint to validate a story independently.
