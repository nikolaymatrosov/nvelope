# Phase 0 Research: Three-Pane Visual Editor

**Feature**: 017-three-pane-visual-editor | **Date**: 2026-05-31

This document resolves the open technical questions for wrapping the feature-014
visual editor in a three-pane shell with a per-block parameters editor. Each item
is a decision, its rationale, and the alternatives rejected.

---

## R1 — Three-pane layout primitive (split / collapse / resize / persist)

**Decision**: Add `react-resizable-panels` (MIT) via shadcn's `resizable`
component (`PanelGroup` / `Panel` / `PanelResizeHandle`). Use a horizontal
`PanelGroup` with three panels: left structure (collapsible, `collapsible
collapsedSize={0} minSize`), center canvas (the existing `VisualEmailEditor`),
right parameters (collapsible). Drive collapse/expand imperatively from panel
header buttons via the `ImperativePanelHandle` ref. Persist the whole layout
(collapsed state + sizes) with the library's built-in `autoSaveId` (writes to
`localStorage`).

**Rationale**:

- Covers FR-020 (independent collapse + re-expand affordance), FR-021 (remember
  layout per operator — `autoSaveId` is exactly per-browser/per-operator), and
  gives resizable widths for free.
- shadcn is already the project's component system (`frontend/components.json`
  present), so the primitive enters through the established registry path; the
  underlying lib is MIT, consistent with 014's "MIT-only, no Pro" constraint.
- Imperative collapse handles let panel-header chevrons and a keyboard shortcut
  both drive the same state without prop-drilling.

**Alternatives considered**:

- *Bespoke CSS grid + a hand-written drag splitter*: rejected — splitter math,
  min/max clamping, collapse animation, and persistence are exactly what the
  library solves; reinventing it is avoidable complexity (Constitution III YAGNI
  cuts the other way here — the library is the simpler whole).
- *`allotment`*: viable and MIT, but `react-resizable-panels` is the one shadcn
  wraps, so it aligns with the project's existing UI conventions and typing.
- *CSS-only collapse with no resize*: rejected — FR-021 requires remembering
  widths if resizable, and operators on wide screens will want to widen the
  params panel; the library cost is already paid once we need persistence.

---

## R2 — Shared block-selection model across the three panes

**Decision**: A single `useBlockSelection(editor)` hook owns the selected
block, identified by its **ProseMirror document position** (`pos`) plus a derived
`{ type, attrs, depth }` snapshot. Wiring:

- **Canvas → selection**: subscribe to TipTap's `selectionUpdate` (and `update`)
  events; resolve `editor.state.selection.$from` up to the nearest *block* node
  (skipping inline/text) and record its `pos`. A click in a block sets the
  selection there.
- **Outline → canvas**: clicking an outline entry calls
  `editor.chain().setNodeSelection(pos).scrollIntoView().run()` (using
  `NodeSelection.create` for atom/leaf blocks like image/divider/button, and a
  text selection at the block start for textual blocks), then focuses the editor.
- **Params → block**: a parameter change dispatches a transaction that calls
  `setNodeAttrs(pos, nextAttrs)` (a small command), keeping the doc the single
  source of truth and re-emitting `onChange`.
- **Highlight**: the selected `pos` drives a `ve-selected` decoration on the
  canvas and an `aria-current` highlight on the matching outline entry.

The hook returns `{ selectedPos, selectedNode, selectBlock(pos), clear() }`. Pos
is remapped through transaction `mapping` on every doc change so it survives
edits/reorders; if the mapped pos no longer resolves to a block (e.g. the block
was deleted), selection clears → params panel shows its empty state (edge case in
spec).

**Rationale**: ProseMirror position is the canonical, edit-stable handle for "a
node in the doc"; deriving everything from it guarantees the three panes can never
disagree (FR-002, SC-004 "zero desync"). Mutating attrs through a command keeps
the structured doc authoritative and reuses 014's controlled-component
`value`/`onChange` contract untouched.

**Alternatives considered**:

- *Synthetic block IDs added to every node's attrs*: rejected — pollutes the wire
  schema, must be generated/migrated, and `Date.now()`/random ID generation is
  awkward; positions already exist and are free.
- *Index path (`[2,0,1]` into content)*: rejected — brittle under concurrent
  transactions; ProseMirror's `mapping` already solves position remapping
  correctly.

---

## R3 — Per-block `style` model and which blocks carry it

**Decision**: Introduce one optional value object `BlockStyle` attached as a
`style` attr on styleable blocks. Fields are a flat, email-safe subset:

```
BlockStyle = {
  backgroundColor?: string   // #RGB/#RRGGBB only
  color?: string             // text/foreground; #RGB/#RRGGBB only
  fontFamily?: string        // from a curated allow-list
  fontSize?: number          // px, 8–72
  fontWeight?: 400 | 700     // normal / bold (email-safe)
  lineHeight?: number        // unitless, 1.0–3.0
  textAlign?: "left" | "center" | "right"
  paddingTop?, paddingRight?, paddingBottom?, paddingLeft?: number  // px, 0–64
  borderRadius?: number      // px, 0–48
  borderWidth?: number       // px, 0–8
  borderStyle?: "solid" | "dashed" | "dotted"
  borderColor?: string       // #RGB/#RRGGBB only
}
```

Each block type exposes only the **subset that is meaningful and email-safe** for
it (the matrix lives in [data-model.md](./data-model.md)):

- **button**: backgroundColor, color, fontFamily, fontSize, fontWeight,
  borderRadius, padding\*, plus existing label/href.
- **image**: borderRadius, width (existing-style), alignment, plus existing
  alt/href. (No background/font.)
- **paragraph/heading/blockquote/list**: color, fontFamily, fontSize, fontWeight,
  lineHeight, textAlign, padding\* (as cell padding).
- **divider**: borderColor (line color), borderWidth (thickness), padding\*
  (spacing above/below).
- **columns (container) / column**: backgroundColor, padding\*, borderRadius,
  border\*, plus existing count / column widths + gap.
- **codeBlock / rawHtml / mergeTag**: no style controls (rawHtml directs to "edit
  as HTML"; codeBlock is verbatim).

Absent fields mean "inherit theme/default" (FR-019). The panel distinguishes
"explicitly set" from "inherited" and offers per-field and whole-block reset.

**Rationale**: A single flat value object keeps the wire schema, both validators,
the renderer mapping, and the Go struct symmetric and easy to keep in sync (the
014 architecture's defining discipline). Restricting to a small, well-understood
property set is what makes email-client safety *enforceable* (R5) rather than
aspirational. Per-type subsets prevent the "generic dump of irrelevant controls"
edge case.

**Alternatives considered**:

- *Free-form CSS string per block*: rejected outright — un-validatable,
  un-bounded, defeats SC-007, and hands the sanitizer an open-ended problem.
- *Deeply nested style object (typography{}, box{}, border{})*: rejected — more
  ceremony than value at this size; a flat object maps 1:1 to controls and CSS.
- *Per-block theme overrides reusing the existing `Theme` value object*: rejected
  — `Theme` is document-scoped (FR-014 forbids per-block changes touching it) and
  models different concepts (link color, container width).

---

## R4 — Mapping `BlockStyle` to email-ready HTML in the BFF renderer

**Decision**: Extend `frontend/src/server/render/components.tsx` so each styleable
block renderer merges `block.attrs.style` into the inline `style` it already
emits, with **theme value as the base and per-block style as the override**
(e.g. `style={{ color: theme.textColor, ...mapBlockStyle(block.attrs.style) }}`).
A pure helper `mapBlockStyle(s: BlockStyle): React.CSSProperties` translates the
flat value object to react-email-compatible inline CSS. Container styles
(background/padding/border on columns/sections) are applied to the table cell /
`<Column>`/`<Container>` wrapper so they survive Outlook (014 FR-015 table
discipline). Buttons map `backgroundColor`/`borderRadius`/`padding` onto
react-email's `<Button>` style prop.

**Rationale**: The renderer is already the single place block→HTML mapping lives;
extending it keeps one source of truth and lets the existing golden-test harness
prove the styled output byte-for-byte. Theme-as-base/style-as-override gives the
inherit-vs-explicit semantics (FR-019) for free at render time.

**Alternatives considered**:

- *Client-side style application only (CSS vars in the canvas, nothing in the
  rendered HTML)*: rejected — violates the spec's core requirement that params
  reach the delivered email (FR-017/FR-018) and the 014 "browser never produces
  canonical HTML" rule.
- *A `<style>` block with per-block classes*: rejected — email clients
  (notably Gmail/Outlook) strip or unreliably honor `<style>`; inline CSS is the
  email-safe path and is what the sanitizer already permits.

---

## R5 — Sanitizer policy for bounded inline-style CSS

**Decision**: Keep the Go bluemonday sanitizer as the authoritative save-time
gate and make the **inline-style allow-list explicit** for the `BlockStyle`
property set. Today `sanitize.go` calls `p.AllowStyling()` (which by itself
re-allows `class`, not the `style` attribute); the renderer's existing inline
styles survive only because react-email output is otherwise clean. To make
per-block style robust and intentional, add an explicit CSS-property allow-list
via bluemonday's style policies — permit exactly the properties `mapBlockStyle`
can emit (`background-color`, `color`, `font-family`, `font-size`,
`font-weight`, `line-height`, `text-align`, `padding`/`padding-*`,
`border-radius`, `border-width`, `border-style`, `border-color`,
`border`) with value regexes (hex color, `Npx`, enum keywords). Anything else in
an inline `style` is stripped, and the existing `sanitizer_stripped` warning still
fires. Because the BFF + Go validators already reject out-of-bounds values
(R6), the sanitizer is defense-in-depth, not the primary bound — so SC-007
(0 stripped warnings from the param editor) holds in normal operation.

**Action item flagged for implementation**: verify bluemonday's exact style-policy
API surface in the pinned version and confirm whether `AllowStyling()` +
`AllowAttrs("style")` + per-property `AllowStyles`/`MatchingHandler` is the right
combination; add a sanitizer test that a `BlockStyle`-derived inline style
survives intact while a hostile property (`position`, `behavior`,
`expression(...)`) is stripped.

**Rationale**: An explicit property allow-list turns "email-safe styling" into a
mechanically-enforced contract (Constitution IV — security is structural). It also
documents, in one place, exactly which CSS the platform will ever emit, which the
TS validator and Go validator mirror.

**Alternatives considered**:

- *Trust the validators and leave the sanitizer permissive on `style`*: rejected
  — the sanitizer is the authoritative gate per 014; relying on upstream
  validation alone weakens defense-in-depth.
- *Strip all inline style and re-apply server-side from a parsed model*: rejected
  — the renderer already emits inline style as its primary mechanism; this would
  re-architect 014's render path for no gain.

---

## R6 — Where parameter values are validated (bounds enforcement)

**Decision**: Three layers, same values:

1. **Controls** constrain at input time (color picker, numeric stepper/slider
   with min/max, font dropdown, alignment toggle) — no free-form entry (FR-015).
2. **BFF validator** (`validate/blocks.ts`) checks `style` bounds on save for fast
   typed feedback before render.
3. **Go validator** (`visualdoc_validate.go`) re-checks the same bounds as the
   authoritative pass (defense-in-depth, mirrors 014's two-tier validation), with
   typed errors (`ErrInvalidStyle` kind) mapped to HTTP in the existing single
   mapping point.

The numeric/enum bounds are defined once in [data-model.md](./data-model.md) and
mirrored across the TS and Go validators; if a shared allow-list (e.g. permitted
font families) is introduced, it gets the same source-of-truth drift-catcher test
pattern 014 already uses for campaign-merge-tag keys.

**Rationale**: Mirrors the exact two-tier validation discipline 014 established;
constrained controls mean the bounds are rarely hit, but the server stays
authoritative.

**Alternatives considered**: client-only validation (rejected — server must be
authoritative); single-layer server validation without constrained controls
(rejected — worse UX, and FR-015 explicitly requires constrained controls).

---

## R7 — Structure-outline projection and reorder mechanism

**Decision**: The `StructureOutline` derives its tree **live from the controlled
`value: VisualDoc`** (a pure projection, recomputed on change) — each entry stores
the block's `pos` for selection. Labels are derived per type (heading/paragraph
text excerpt, button label, image alt, "Divider", "2 columns", etc.). Reorder uses
**native HTML5 drag-and-drop** on outline rows; on drop, compute source/target
`pos` and run a TipTap transaction that moves the node (the same node-move
primitive the in-house canvas `DragHandle` already performs), with invalid targets
(e.g. dropping a columns container into its own column) rejected before dispatch.
Delete/duplicate are small editor commands at the entry's `pos`. Container entries
collapse/expand via local component state (not persisted — purely a view aid).

**Rationale**: Reusing the doc as the projection source guarantees outline⇄canvas
fidelity with zero extra state. Reusing the editor's node-move transaction keeps a
single reorder implementation (no second drag library), consistent with 014's
no-extra-dep stance. Native HTML5 DnD is sufficient for a vertical tree and avoids
pulling in a DnD framework.

**Alternatives considered**:

- *`@dnd-kit` / `react-dnd`*: rejected — a whole DnD framework for one vertical
  tree is disproportionate; native DnD + existing move commands suffice.
- *Maintaining a parallel outline data structure*: rejected — invites desync,
  which SC-004 explicitly forbids.

---

## R8 — Panel-layout persistence scope and storage

**Decision**: Persist collapsed/expanded state and panel widths in **browser
localStorage** via `react-resizable-panels`' `autoSaveId` (e.g.
`autoSaveId="ve-three-pane"`). Scope is per-browser-profile, which the spec frames
as "per operator." No server persistence, no new table, no migration.

**Rationale**: Layout preference is UI chrome, not content and not tenant data;
storing it client-side keeps every service stateless (Constitution V) and adds no
backend surface. It matches the spec's assumption ("remembered per operator for
the editor as a whole, not per campaign/template, and not part of the sent
content"). 014 deferred *content* autosave/localStorage; this is unrelated — it
persists a UI preference, not document content, so there is no conflict with that
deferral.

**Alternatives considered**:

- *Server-side per-user preference (new table/endpoint)*: rejected as speculative
  scope (Constitution III) — cross-device sync of a panel layout is not a stated
  requirement; localStorage is the proportionate choice.
- *No persistence (reset each open)*: rejected — FR-021 explicitly requires
  remembering the layout.

---

## R9 — Narrow-viewport behavior

**Decision**: Below a breakpoint (~1024 px, SC-006), the side panels default to
collapsed and, when opened, render as **overlays** over the canvas (via the
existing shadcn `sheet` primitive on small screens) rather than as a third
in-flow column. Above the breakpoint they are in-flow resizable panels. The
canvas never drops below its minimum usable width.

**Rationale**: Satisfies FR-022/SC-006 without a separate mobile editor; reuses
the already-present `sheet` component for the overlay presentation so no new
primitive is needed for the responsive path.

**Alternatives considered**: three squeezed columns (rejected — the explicit
anti-goal of FR-022); a dedicated mobile layout (rejected — out of scope, desktop
authoring is the stated target).

---

## Summary of decisions

| # | Topic | Decision |
|---|-------|----------|
| R1 | Layout primitive | `react-resizable-panels` via shadcn `resizable`; `autoSaveId` persistence |
| R2 | Selection model | Single `useBlockSelection` hook keyed on ProseMirror `pos`, mapping-stable |
| R3 | Style model | One flat `BlockStyle` value object; per-type subset; absent = inherit |
| R4 | Render mapping | BFF renderer merges `style` over theme as inline CSS; container styles on table cell |
| R5 | Sanitizer | Explicit bluemonday inline-style property allow-list (defense-in-depth) |
| R6 | Validation | Constrained controls + BFF validator + authoritative Go validator (same bounds) |
| R7 | Outline | Live projection of `VisualDoc`; reorder via existing node-move transaction + native DnD |
| R8 | Layout persistence | Client-side localStorage (`autoSaveId`); no server state |
| R9 | Narrow viewport | Collapse-by-default + `sheet` overlay below ~1024 px |

All Technical Context unknowns are resolved; no `NEEDS CLARIFICATION` remains.
