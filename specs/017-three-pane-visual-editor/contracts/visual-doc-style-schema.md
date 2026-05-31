# Contract: Extended Structured-Document JSON Schema (per-block `style`)

**Feature**: 017-three-pane-visual-editor | **Date**: 2026-05-31

This feature adds **no new HTTP endpoints**. The only contract change is an
additive extension to the structured-document (`VisualDoc`) JSON that already
flows over the 014 endpoints:

- `PUT /t/{slug}/api/campaigns/{id}/visual` (BFF-hosted)
- `PUT /t/{slug}/api/templates/{id}/visual` (BFF-hosted)
- `POST /t/{slug}/api/render-preview` (BFF-only)

All three already accept a `bodyDoc` (the `VisualDoc`). This contract specifies
the new optional `style` attribute that styleable blocks may now carry. The
endpoints' request/response envelopes, status codes, concurrency
(`ifUnmodifiedSince` / `409 stale_row`), and warning semantics are **unchanged**
from 014.

## `BlockStyle` schema

```jsonc
// Optional. Attached as attrs.style on styleable blocks. Every field optional.
// Absent field ⇒ inherit document Theme / default.
BlockStyle = {
  "backgroundColor": "#RRGGBB",   // or #RGB
  "color":           "#RRGGBB",   // text / foreground
  "fontFamily":      "<allow-listed family>",
  "fontSize":        <int 8..72>,         // px
  "fontWeight":      400 | 700,
  "lineHeight":      <number 1.0..3.0>,   // unitless
  "textAlign":       "left" | "center" | "right",
  "paddingTop":      <int 0..64>,         // px
  "paddingRight":    <int 0..64>,
  "paddingBottom":   <int 0..64>,
  "paddingLeft":     <int 0..64>,
  "borderRadius":    <int 0..48>,         // px
  "borderWidth":     <int 0..8>,          // px
  "borderStyle":     "solid" | "dashed" | "dotted",
  "borderColor":     "#RRGGBB"
}
```

## Per-block placement

`style` is accepted on these block `attrs` (others reject it): `paragraph`,
`heading`, `blockquote`, `bulletList`, `orderedList`, `button`, `image`,
`divider`, `columns`, `column`. Each type honors only its applicable subset (see
[data-model.md](../data-model.md) matrix); a field outside a type's subset is
ignored by the renderer and SHOULD be omitted by the editor.

Not accepted on: `codeBlock`, `rawHtml`, `mergeTag`, `listItem`, text/`text`
inlines.

## Validation contract (both server tiers)

On save, the BFF validator and then the authoritative Go validator MUST reject a
`bodyDoc` whose any block `style`:

- has a color field not matching `^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`,
- has a numeric field out of its bound or non-integral (except `lineHeight`),
- has an enum field outside its literals,
- has a `fontFamily` not in the shared allow-list,
- carries `style` on a non-styleable block type, or a property outside that
  block's applicable subset.

Rejection returns the existing error envelope with a typed kind
`invalid_style` (HTTP 422), naming the offending block/field (FR-024). No partial
save (atomic, as in 014).

## Render contract

The BFF renderer MUST emit each honored `style` field as inline CSS on the block's
email-ready element, layered as `theme defaults → block style` (block style wins).
Container styles (`backgroundColor`, `padding*`, `border*`, `borderRadius` on
`columns`/`column`/`blockquote`/`button`) MUST be applied to the table-cell /
wrapper element so they render in Outlook (table discipline, 014 FR-015). The Go
sanitizer MUST allow exactly the CSS properties this schema can produce and strip
any other inline-style property.

## Backwards compatibility

A block without a `style` key (every pre-017 block, and any block the operator
never restyled) is valid and renders identically to 014 output. No data migration
backfills `style`.

## Golden-fixture impact

`frontend/src/server/render/__fixtures__/` gains a `*-styled.html` / `*-styled.txt`
pair per styleable block type, asserted byte-for-byte, pinned to the exact
react-email version (consistent with 014's fixture-stability approach).
