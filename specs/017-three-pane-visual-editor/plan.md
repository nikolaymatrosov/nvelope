# Implementation Plan: Three-Pane Visual Editor — Structure Outline & Block Parameters

**Branch**: `017-three-pane-visual-editor` | **Date**: 2026-05-31 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/017-three-pane-visual-editor/spec.md`

## Summary

Reframe the existing Phase 7 visual editor (feature 014) — today a single TipTap
canvas — as the **center pane of a three-pane editor**. Add a **left panel** that
projects the structured block document as a navigable, reorderable outline, and a
**right panel** that is a **parameters editor** for the currently-selected block,
exposing email-safe style knobs (background, text color, corner radius, padding,
border, alignment, and font family/size/weight/line-height) plus each block's
existing type-specific attributes. Both side panels are **collapsible and
resizable**, and the layout is **remembered per operator**.

The three panes share one block-selection state so the canvas, outline, and
parameters panel always agree on "what is selected." Per-block parameters are
stored as a new optional `style` attribute object on each block inside the
existing `body_doc` JSON — **no schema migration** (the column is already
`jsonb`), **no new send path**, and **no new HTTP endpoint**. They thread through
the same pipeline 014 built: TipTap node attrs → SPA `VisualDoc` → BFF
`@react-email/render` (emit inline CSS) → BFF + Go validators (bounded, email-safe
values) → Go bluemonday sanitizer → persisted `body_html`/`body_text` → Phase 3
send pipeline unchanged.

Three user stories ship as three increments:

- **US1 (P1)** — the right-hand parameters panel: select a block, fine-tune its
  style, see the canvas update live, persist it, render it into the email. This
  is the headline value and includes the per-block `style` model threaded through
  every render/validate/sanitize layer.
- **US2 (P2)** — the left-hand structure outline: a live projection of the doc
  tree with click-to-select/scroll, drag-to-reorder, delete/duplicate, and
  collapsible containers.
- **US3 (P2)** — collapsible/resizable side panels with per-operator layout
  persistence and a graceful narrow-viewport fallback.

The layout shell uses `react-resizable-panels` (MIT, via shadcn's `resizable`
component) for the split/collapse/persist mechanics; everything else is in-house
against the existing TipTap MIT core, consistent with 014's no-Pro / no-extra-dep
stance.

## Technical Context

**Language/Version**: TypeScript 5.9 + React 19 (frontend, the bulk of this
feature) / Go 1.26 (backend, additive block-attr changes only). No new languages.

**Primary Dependencies**:

- **Frontend (new)**: `react-resizable-panels` (MIT) added via the shadcn
  `resizable` component — provides the three-pane split, per-panel collapse,
  imperative collapse/expand, and `autoSaveId` localStorage layout persistence
  (covers FR-020/FR-021/FR-022 without bespoke splitter math). No DnD library is
  added: the structure-outline reorder reuses TipTap's node-move transactions
  (the same primitive the existing in-house `DragHandle` uses) driven by native
  HTML5 drag events.
- **Frontend (existing, reused)**: `@tiptap/react` + StarterKit + the custom
  extensions (Columns, Button, Divider, ImageBlock, MergeTag, RawHTML), the
  in-house `DragHandle`/`SlashCommandMenu`/`BubbleMenu`, shadcn + Radix UI,
  Tailwind v4, lucide-react, react-i18next (the `visualEditor` namespace),
  TanStack Router/Query/Form.
- **BFF (existing, reused)**: `@react-email/components` + `@react-email/render`
  in `frontend/src/server/render/` — extended so each block renderer emits the
  per-block inline `style`. The `validate/` layer gains `style`-attr bounds
  checks. No new Nitro route.
- **Backend (existing, reused)**: the `internal/campaign/domain` VisualDoc types
  and validator, plus the `internal/campaign/visualrender` bluemonday sanitizer —
  extended to carry, validate, and pass through the per-block `style` attrs. No
  new bounded context, no new adapter package.

**Storage**: PostgreSQL, unchanged. Per-block `style` lives inside the existing
`campaigns.body_doc` / `templates.body_doc` `jsonb` columns from migration
000020 — **no new migration**. Panel-layout preference is **client-side only**
(localStorage via `react-resizable-panels` `autoSaveId`); it is UI chrome, not
tenant data, so it adds no table and no server state.

**Testing**:

- **Frontend (`vitest` + Storybook)**: component tests for the three-pane shell
  (collapse/expand/persist, narrow-viewport fallback), the `StructureOutline`
  (projection fidelity for nested columns, click-to-select sync, reorder,
  delete/duplicate, container collapse), and the `BlockParamsPanel` (correct
  control set per block type, live-apply to the selected block only, empty state,
  reset-to-default, constrained inputs). Selection-sync tests assert canvas ↔
  outline ↔ params never desync. Storybook stories per new component per the
  repo's Storybook workflow.
- **BFF (`vitest`)**: render golden tests extended — a styled variant per block
  type asserts the per-block inline CSS is emitted; validator unit tests for the
  `style` bounds (rejects out-of-range radius/padding, non-allow-listed
  font/color formats).
- **Backend (`go test ./...`)**: `visualdoc_validate.go` tests for the new
  `style` attr bounds (defense-in-depth mirror of the TS validator); sanitizer
  tests asserting the bounded inline-style CSS survives the bluemonday pass while
  disallowed properties/values are still stripped; the existing
  visual-save/load round-trip and tenant-isolation tests extended to carry
  `style`.
- **Cross-stack**: the existing drift-catcher pattern (TS reads Go source) is
  extended if a new shared allow-list (e.g. permitted font families) is
  introduced; otherwise the existing golden + validator parity holds the line.

**Target Platform**: Modern desktop browsers for authoring (three panes); the
produced HTML continues to target Gmail (web/mobile), Apple Mail (desktop/iOS),
Outlook (desktop/web). Per-block styles are restricted to inline-CSS primitives
that render reliably in those clients (no CSS-grid-only constructs; table-based
column layout preserved per 014 FR-015).

**Project Type**: Web application — additive changes to the existing React SPA
(the dominant surface), the existing TanStack Start + Nitro BFF render/validate
layer, and the existing Go campaign domain. No new service, no new deployable.

**Performance Goals**: Interactive. A parameter change applies to the canvas in
< 1 s (SC-001) — local TipTap transaction, no round-trip. The structure outline
and panels stay responsive for ≥ 30-block, deeply-nested documents (SC-004). Save
remains the same single BFF→Go round-trip as 014 (well under 500 ms p95 for ≤ ~50
blocks); per-block styles add only a few bytes per block to the rendered HTML.

**Constraints**:

- **No new send path, no new endpoint, no migration.** Per-block style is purely
  additive JSON inside `body_doc`; `cmd/worker` still consumes `body_html` /
  `body_text` unchanged.
- **Server is still the single source of canonical HTML.** The parameters panel
  mutates the structured doc in the browser; the email HTML is still rendered by
  the BFF and sanitized by Go at save time. The browser never produces canonical
  HTML (014 constraint preserved).
- **Email-client safety bounds the controls.** Every parameter control emits only
  values from a curated, client-safe set; the BFF and Go validators reject
  anything outside it, so the sanitizer never has to strip a parameter-editor
  value (SC-007).
- **Single-block selection.** Multi-select editing is out of scope.
- **Per-operator layout is client-side chrome**, not content and not tenant data
  — keeps services stateless (Constitution V) with no new persistence.
- **MIT-only deps.** The one new dependency (`react-resizable-panels`) is MIT and
  enters via shadcn's registry; no TipTap Pro, no paid components.

**Scale/Scope**:

- 0 migrations. 0 new HTTP endpoints. 0 new Go bounded contexts.
- Frontend: ~1 new layout shell (`ThreePaneEditor` / wraps `VisualEmailEditor`),
  ~2 new panel components (`StructureOutline`, `BlockParamsPanel`) + per-block
  param sub-forms, ~1 shared selection hook (`useBlockSelection`), ~1 added
  shadcn `resizable` ui primitive, and extensions to the existing TipTap nodes to
  carry the `style` attr.
- BFF: extend `render/components.tsx` (emit per-block inline style),
  `render/types.ts` (add `style` to block attrs), `validate/blocks.ts` (style
  bounds). Golden fixtures regenerated for styled variants.
- Go: extend `internal/campaign/domain/visualdoc.go` (block `style` struct) +
  `visualdoc_validate.go` (bounds) + `internal/campaign/visualrender/sanitize.go`
  (confirm/allow bounded inline-style CSS).
- i18n: new keys in the existing `visualEditor` namespace (en + ru, parity test
  enforced).

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` v1.1.0.*

- **I. Tenant Isolation by Default** — PASS. No new tables and no new
  tenant-scoped data: per-block `style` lives inside the already-tenant-scoped,
  RLS-bound `campaigns`/`templates` rows (`body_doc jsonb`). The existing visual
  save/load tenant-isolation integration tests are extended to carry `style`, so
  isolation coverage tracks the new field. Panel-layout preference is per-browser
  client state, never persisted server-side, so it carries no isolation surface.

- **II. Test-Backed Delivery (NON-NEGOTIABLE)** — PASS. Each increment ships with
  tests: render golden tests gain styled-block variants (byte-for-byte), both
  validators gain `style`-bounds unit tests, the Go sanitizer gains tests that
  the bounded CSS survives while disallowed CSS is stripped, and the SPA panels
  ship component + Storybook tests including the selection-sync invariant. The
  visual save/load round-trip test is extended to prove `style` round-trips
  losslessly (SC-002).

- **III. Incremental, Shippable Phases** — PASS. US1 (params panel) is a
  standalone shippable slice — it works with canvas-click selection and needs
  neither the outline nor the collapsible chrome. US2 (outline) and US3
  (collapse/persist) layer on independently. No speculative scope: multi-select,
  collaborative editing, and server-persisted layout are explicitly excluded
  (YAGNI).

- **IV. Security & Consent by Design** — PASS. Canonical HTML is still rendered
  server-side (BFF) and sanitized server-side (Go bluemonday) at save; the
  parameters panel only mutates the structured doc client-side. Parameter values
  are bounded by validators on both server tiers before persistence, so no
  operator input widens the sanitizer's attack surface. Permission gating is
  unchanged — the panels live inside the editor and inherit `campaigns:manage` /
  `templates:manage` (FR-023); operators without them never see the editor.

- **V. Operable & Observable Services** — PASS. All new code is stateless: the
  panels are client React state; per-block style is pure data on the doc;
  layout preference is browser localStorage. No new queue, service, or
  long-running work. The save path's structured logging (`tenant_id`,
  `actor_id`, `request_id`) is unchanged because the endpoint is unchanged.

- **VI. Layered Architecture & Domain Integrity** — PASS. The per-block `style`
  is a value object on the existing `VisualDoc`/`Block` domain types
  (`internal/campaign/domain`), validated by the domain validator with typed
  errors mapped to HTTP in the single existing mapping point. No transport or DB
  concern enters the domain; the BFF render and Go sanitizer remain adapters. No
  new DI, no global state. The frontend selection/panel state is presentation
  concern kept in components/hooks, not leaked into the wire types.

**Result**: PASS — no violations, Complexity Tracking not required.

*Post-design re-check after Phase 1*: see the foot of [data-model.md](./data-model.md).
The design adds one optional value object (`BlockStyle`) to existing aggregates,
zero schema changes, zero endpoints, and one MIT UI dependency. Still PASS.

## Project Structure

### Documentation (this feature)

```text
specs/017-three-pane-visual-editor/
├── plan.md              # This file (/speckit-plan output)
├── research.md          # Phase 0 — layout primitive, selection model, style model, sanitizer policy, persistence
├── data-model.md        # Phase 1 — BlockStyle value object, per-block param matrix, layout-preference entity
├── quickstart.md        # Phase 1 — run, verify, manual-test instructions
├── contracts/           # Phase 1 — extended VisualDoc JSON schema + UI contract for the three panes
│   ├── visual-doc-style-schema.md
│   └── ui-contract.md
├── checklists/
│   └── requirements.md  # /speckit-specify output
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
frontend/
├── components.json                                  # EXTENDED — register shadcn `resizable`
├── src/
│   ├── components/
│   │   ├── ui/
│   │   │   └── resizable.tsx                         # NEW — shadcn wrapper over react-resizable-panels
│   │   └── visual-editor/
│   │       ├── VisualEmailEditor.tsx                # EXTENDED — expose editor instance + selection to the shell; keep canvas concerns
│   │       ├── ThreePaneEditor.tsx                  # NEW — layout shell: left | canvas | right, collapse/resize/persist (US3)
│   │       ├── hooks/
│   │       │   └── useBlockSelection.ts             # NEW — single selection state shared by all three panes (FR-002)
│   │       ├── panels/
│   │       │   ├── StructureOutline.tsx             # NEW — left panel: doc-tree projection, select/reorder/delete/duplicate/collapse (US2)
│   │       │   ├── StructureOutline.stories.tsx     # NEW
│   │       │   ├── BlockParamsPanel.tsx             # NEW — right panel: dispatches to per-type param form (US1)
│   │       │   ├── BlockParamsPanel.stories.tsx     # NEW
│   │       │   └── params/
│   │       │       ├── StyleControls.tsx            # NEW — shared bg/color/radius/padding/border/align/font controls
│   │       │       ├── ButtonParams.tsx             # NEW — button-specific (label/href + style)
│   │       │       ├── ImageParams.tsx              # NEW — image-specific (width/alt/href + style)
│   │       │       ├── TextParams.tsx               # NEW — paragraph/heading/list/quote (font + spacing + align)
│   │       │       ├── DividerParams.tsx            # NEW — thickness/color/spacing
│   │       │       └── ColumnsParams.tsx            # NEW — column widths/gap + container style
│   │       ├── extensions/                          # EXTENDED — each node gains the `style` attr (parse/render/commands)
│   │       │   ├── Button.tsx                       # EXTENDED
│   │       │   ├── Columns.tsx                      # EXTENDED
│   │       │   ├── Divider.tsx                      # EXTENDED
│   │       │   └── ImageBlock.tsx                   # EXTENDED
│   │       ├── extensions/styleAttr.ts              # NEW — shared addAttributes() helper for the `style` attr on StarterKit nodes
│   │       └── visual-editor.css                    # EXTENDED — three-pane layout + selected-block affordance
│   ├── lib/
│   │   └── api-types.ts                             # EXTENDED — BlockStyle + per-block attrs gain optional `style`
│   ├── locales/{en,ru}/visualEditor.json            # EXTENDED — outline/params/panel strings (parity-tested)
│   └── server/
│       ├── render/
│       │   ├── types.ts                             # EXTENDED — block attrs gain optional `style`
│       │   ├── components.tsx                       # EXTENDED — each block renderer emits per-block inline style
│       │   └── __fixtures__/                        # EXTENDED — styled variant fixtures per styleable block
│       └── validate/
│           └── blocks.ts                            # EXTENDED — style-attr bounds (range/format/allow-list)

internal/
└── campaign/
    ├── domain/
    │   ├── visualdoc.go                             # EXTENDED — BlockStyle struct on styleable blocks
    │   └── visualdoc_validate.go                    # EXTENDED — style bounds (defense-in-depth mirror of TS)
    └── visualrender/
        └── sanitize.go                              # EXTENDED/CONFIRMED — bounded inline-style CSS allow-list survives bluemonday
```

**Structure Decision**: Web application, extending three existing surfaces in
place — the React SPA visual-editor component tree (the dominant change), the BFF
render/validate layer, and the Go campaign domain. The new three-pane shell wraps
the existing `VisualEmailEditor` rather than replacing it: the canvas component is
refactored to expose its TipTap editor instance and selection to a shared
`useBlockSelection` hook, which the `StructureOutline` (left) and
`BlockParamsPanel` (right) both consume so all three panes stay in sync. Per-block
style is a new optional value object on the existing `VisualDoc`/`Block` domain
types — no new aggregate, no migration, no endpoint. The only new external
dependency is the MIT `react-resizable-panels` (via shadcn `resizable`) for the
collapse/resize/persist mechanics.

## Complexity Tracking

> No constitution violations to justify. Section intentionally empty.
