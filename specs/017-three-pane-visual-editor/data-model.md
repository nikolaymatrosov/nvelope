# Phase 1 Data Model: Three-Pane Visual Editor

**Feature**: 017-three-pane-visual-editor | **Date**: 2026-05-31

This feature adds **no new tables and no migration**. It adds one optional value
object (`BlockStyle`) to existing blocks inside the `body_doc jsonb` document, and
one client-only preference (panel layout). Everything else here is editor state.

---

## 1. `BlockStyle` (new value object on existing blocks)

A flat, email-safe set of styling attributes. Stored as an optional `style` key on
a styleable block's `attrs` inside the existing `VisualDoc` (`campaigns.body_doc` /
`templates.body_doc`). Absent or absent-field ⇒ "inherit theme/default" (FR-019).

### Fields, types, and bounds

| Field | Type | Bound / allowed values | Maps to CSS |
|-------|------|------------------------|-------------|
| `backgroundColor` | string | `#RGB` or `#RRGGBB` | `background-color` |
| `color` | string | `#RGB` or `#RRGGBB` | `color` |
| `fontFamily` | string | member of the curated font allow-list | `font-family` (with email-safe fallback stack) |
| `fontSize` | number | integer px, 8–72 | `font-size` |
| `fontWeight` | enum | `400` \| `700` | `font-weight` |
| `lineHeight` | number | 1.0–3.0 (unitless, 1 decimal) | `line-height` |
| `textAlign` | enum | `left` \| `center` \| `right` | `text-align` |
| `paddingTop` | number | integer px, 0–64 | `padding-top` |
| `paddingRight` | number | integer px, 0–64 | `padding-right` |
| `paddingBottom` | number | integer px, 0–64 | `padding-bottom` |
| `paddingLeft` | number | integer px, 0–64 | `padding-left` |
| `borderRadius` | number | integer px, 0–48 | `border-radius` |
| `borderWidth` | number | integer px, 0–8 | `border-width` |
| `borderStyle` | enum | `solid` \| `dashed` \| `dotted` | `border-style` |
| `borderColor` | string | `#RGB` or `#RRGGBB` | `border-color` |

### Validation rules (mirrored: controls → BFF `validate/blocks.ts` → Go `visualdoc_validate.go`)

- Color fields MUST match `^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`.
- Numeric fields MUST be within their inclusive bounds and integral (except
  `lineHeight`, one decimal place).
- Enum fields MUST be one of the listed literals.
- `fontFamily` MUST be a member of the shared font allow-list (single source of
  truth; drift-catcher test if duplicated TS/Go per the 014 pattern).
- An out-of-bounds, malformed, or unknown property ⇒ typed error
  (`ErrInvalidStyle` / kind `invalid_style`), mapped to HTTP in the single
  existing mapping point; the save fails with a named reason (FR-024).
- A field MAY be omitted; omission is valid and means "inherit."

### Per-block-type applicability matrix

Only the meaningful, email-safe subset is exposed per type (FR-012 — no irrelevant
controls). `✓` = exposed in the params panel and honored by the renderer.

| Block | bg | color | font\* | lineHeight | textAlign | padding\* | borderRadius | border\* | Existing type-specific attrs |
|-------|----|-------|--------|-----------|-----------|-----------|--------------|----------|------------------------------|
| paragraph | – | ✓ | ✓ | ✓ | ✓ | ✓ | – | – | — |
| heading | – | ✓ | ✓ | ✓ | ✓ | ✓ | – | – | `level` (1–3) |
| blockquote | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | — |
| bulletList / orderedList | – | ✓ | ✓ | ✓ | – | ✓ | – | – | — |
| button | ✓ | ✓ | ✓ | – | – | ✓ | ✓ | ✓ | `label`, `href` |
| image | – | – | – | – | ✓ (align) | – | ✓ | ✓ | `mediaRef`, `alt`, `href`, width |
| divider | – | – | – | – | – | ✓ (spacing) | – | ✓ (line color/width/style) | — |
| columns (container) | ✓ | – | – | – | – | ✓ | ✓ | ✓ | `count` (2–4), column widths, gap |
| column | ✓ | – | – | – | ✓ (valign) | ✓ | ✓ | ✓ | — |
| codeBlock | – | – | – | – | – | – | – | – | verbatim text (no style) |
| rawHtml | – | – | – | – | – | – | – | – | `html` (opaque; "edit as HTML") |
| mergeTag (inline) | – | – | – | – | – | – | – | – | `namespace`, `key` (no style) |

> `font\*` = `fontFamily`, `fontSize`, `fontWeight`. `border\*` = `borderWidth`,
> `borderStyle`, `borderColor`. For `divider`, border maps to the rule line; for
> `image`/`columns`/`button` it maps to the element box.

### Wire representation (extends 014's structured-document JSON)

```jsonc
// button with explicit style — every style field optional
{
  "type": "button",
  "attrs": {
    "label": "Read more",
    "href": "https://example.com/post/42",
    "style": {
      "backgroundColor": "#1a73e8",
      "color": "#ffffff",
      "borderRadius": 8,
      "paddingTop": 12, "paddingRight": 20, "paddingBottom": 12, "paddingLeft": 20,
      "fontWeight": 700
    }
  }
}
```

```jsonc
// paragraph that only overrides alignment + size; everything else inherits theme
{
  "type": "paragraph",
  "attrs": { "style": { "textAlign": "center", "fontSize": 18 } },
  "content": [ { "type": "text", "text": "Hello" } ]
}
```

A pre-017 block simply has no `style` key (or `attrs` without `style`) — valid,
renders exactly as today (backwards-compat per spec edge case).

### Relationships & lifecycle

- `BlockStyle` belongs to exactly one block; it has no identity of its own and is
  never shared or referenced across blocks.
- It is created/edited only through the right-hand parameters panel, applied to
  the selected block via a `setNodeAttrs(pos, …)` transaction.
- "Reset to default" removes the field (per-field) or the whole `style` object
  (whole-block), restoring inheritance from the document `Theme` (014).
- It is persisted, rendered, sanitized, and round-tripped through the **existing**
  014 save path with no new endpoint.

---

## 2. Block selection (editor session state — not persisted)

| Field | Type | Notes |
|-------|------|-------|
| `selectedPos` | number \| null | ProseMirror doc position of the selected block; `null` = nothing selected |
| `selectedNode` | derived | `{ type, attrs, depth }` snapshot resolved from `selectedPos` |

Rules:

- At most one block selected at a time (single-select; multi-select out of scope).
- `selectedPos` is remapped through every transaction's `mapping`; if it no longer
  resolves to a block (deleted), selection clears and the params panel shows its
  empty state.
- Owned by `useBlockSelection(editor)`; consumed by canvas (decoration), outline
  (highlight), and params panel (which controls to show). This is the single
  source guaranteeing FR-002 / SC-004 "zero desync."
- Pure presentation state — never serialized into `VisualDoc`, never sent to the
  server.

---

## 3. Document structure outline (derived projection — not stored)

A recomputed-on-change tree mirroring `VisualDoc.content`:

| Field | Type | Notes |
|-------|------|-------|
| `pos` | number | block position, used for select/reorder/delete/duplicate |
| `type` | string | block type (drives the icon + which params show) |
| `label` | string | content-derived short label (heading/para excerpt, button label, image alt, "Divider", "N columns") |
| `depth` | number | nesting level (column children indent under their columns) |
| `children` | OutlineEntry[] | for `columns`/`column` containers |
| `collapsed` | boolean | view-only local state (not persisted) |

It is a pure function of the controlled `value` — no parallel mutable copy, so it
cannot desync from the document.

---

## 4. Panel layout preference (client-only — not tenant data)

Persisted in browser `localStorage` via `react-resizable-panels` `autoSaveId`:

| Field | Type | Notes |
|-------|------|-------|
| `leftCollapsed` | boolean | structure panel collapsed? |
| `rightCollapsed` | boolean | parameters panel collapsed? |
| `panelSizes` | number[] | relative widths of the three panels |

- Scope: per browser profile (framed as "per operator" by the spec).
- Never sent to the server; no table, no migration; not part of sent content.
- Keeps all services stateless (Constitution V).

---

## Post-design Constitution re-check

- **I. Tenant Isolation** — PASS. No new tenant-scoped storage; `BlockStyle` lives
  inside already-RLS-bound `body_doc`. Existing visual save/load isolation tests
  extended to carry `style`.
- **II. Test-Backed Delivery** — PASS. `style` adds golden-render variants, TS +
  Go validator bounds tests, sanitizer survival/strip tests, and a lossless
  round-trip assertion (SC-002).
- **III. Incremental** — PASS. US1 ships the style model standalone; US2/US3 layer
  on. No speculative storage (layout stays client-side).
- **IV. Security & Consent** — PASS. Server stays the authoritative
  renderer+sanitizer; `style` is bounded by validators on both tiers before
  persistence.
- **V. Operable & Observable** — PASS. All new state is client-side or pure doc
  data; no new queue/service/migration.
- **VI. Layered Architecture** — PASS. `BlockStyle` is a domain value object with
  a validating path and typed errors mapped once; presentation state stays in
  components/hooks, out of the wire types.

No violations; Complexity Tracking not required.
