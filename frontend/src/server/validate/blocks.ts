// Per-block-type validation rules. Mirrors the Go validator's switch in
// internal/campaign/domain/visualdoc_validate.go::validateNode +
// validateInline + validateMergeTag.

import { AllowedCampaignMergeTags } from "./campaign-keys"
import { ValidatorError } from "./errors"
import { ALLOWED_FONT_FAMILIES } from "./fonts"
import { validateLink } from "./link"
import type {
  BlockStyle,
  ColumnBlock,
  Inline,
  ListItemBlock,
  Mark,
  VisualBlock,
} from "../render/types"

const HEX_COLOR_RE = /^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/

// validateStyle enforces the per-block BlockStyle bounds (feature 017),
// mirroring internal/campaign/domain/visualdoc_validate.go::validateStyle. A
// missing style or an absent field means "inherit" and is skipped; any set
// field outside its bound throws an `invalid_style` error naming the field.
// Per-field applicability per block type is enforced by the editor's controls
// (only the applicable controls render); both validators check value bounds.
export function validateStyle(style: BlockStyle | undefined): void {
  if (!style) return
  for (const [name, val] of [
    ["backgroundColor", style.backgroundColor],
    ["color", style.color],
    ["borderColor", style.borderColor],
  ] as const) {
    if (val !== undefined && !HEX_COLOR_RE.test(val)) {
      throw new ValidatorError("invalid_style", `${name} must be a #RGB or #RRGGBB color`)
    }
  }
  if (style.fontFamily !== undefined && !ALLOWED_FONT_FAMILIES.has(style.fontFamily)) {
    throw new ValidatorError("invalid_style", "fontFamily is not in the allow-list")
  }
  if (style.fontSize !== undefined && (style.fontSize < 8 || style.fontSize > 72)) {
    throw new ValidatorError("invalid_style", "fontSize must be between 8 and 72")
  }
  // Read through `number` so a runtime JSON value the type system never
  // anticipated (e.g. 500) is still rejected.
  const fontWeight = style.fontWeight as number | undefined
  if (fontWeight !== undefined && fontWeight !== 400 && fontWeight !== 700) {
    throw new ValidatorError("invalid_style", "fontWeight must be 400 or 700")
  }
  if (style.lineHeight !== undefined && (style.lineHeight < 1 || style.lineHeight > 3)) {
    throw new ValidatorError("invalid_style", "lineHeight must be between 1.0 and 3.0")
  }
  if (
    style.textAlign !== undefined &&
    !["left", "center", "right"].includes(style.textAlign)
  ) {
    throw new ValidatorError("invalid_style", "textAlign must be left, center, or right")
  }
  for (const [name, val] of [
    ["paddingTop", style.paddingTop],
    ["paddingRight", style.paddingRight],
    ["paddingBottom", style.paddingBottom],
    ["paddingLeft", style.paddingLeft],
  ] as const) {
    if (val !== undefined && (val < 0 || val > 64)) {
      throw new ValidatorError("invalid_style", `${name} must be between 0 and 64`)
    }
  }
  if (style.borderRadius !== undefined && (style.borderRadius < 0 || style.borderRadius > 48)) {
    throw new ValidatorError("invalid_style", "borderRadius must be between 0 and 48")
  }
  if (style.borderWidth !== undefined && (style.borderWidth < 0 || style.borderWidth > 8)) {
    throw new ValidatorError("invalid_style", "borderWidth must be between 0 and 8")
  }
  if (
    style.borderStyle !== undefined &&
    !["solid", "dashed", "dotted"].includes(style.borderStyle)
  ) {
    throw new ValidatorError("invalid_style", "borderStyle must be solid, dashed, or dotted")
  }
}

export type ValidateBlocksContext = {
  // knownSlugs is the union of built-in subscriber pseudo-rows and the
  // tenant's custom field slugs, fetched from Go's GET /subscriber-fields
  // by the Nitro route before validation runs.
  knownSlugs: Set<string>
  // mediaUrlPrefix is the tenant's media-library base URL — every
  // ImageBlock.mediaRef must start with this prefix (FR-021). Resolved
  // from process.env.OBJECT_STORAGE_PUBLIC_BASE_URL at startup.
  mediaUrlPrefix: string
  // unknownPlaceholders accumulates every subscriber-namespace merge tag
  // whose slug is not in knownSlugs, so the route can return them all at
  // once instead of one per request.
  unknownPlaceholders?: Array<string>
}

export function validateBlock(block: VisualBlock, ctx: ValidateBlocksContext): void {
  switch (block.type) {
    case "paragraph":
      validateStyle(block.attrs?.style)
      block.content.forEach((i) => validateInline(i, ctx))
      return
    case "heading":
      if (block.attrs.level < 1 || block.attrs.level > 3) {
        throw new ValidatorError("invalid_doc", "heading level must be 1, 2, or 3")
      }
      validateStyle(block.attrs.style)
      block.content.forEach((i) => validateInline(i, ctx))
      return
    case "bulletList":
    case "orderedList":
      validateStyle(block.attrs?.style)
      block.content.forEach((it) => validateListItem(it, ctx))
      return
    case "blockquote":
      validateStyle(block.attrs?.style)
      block.content.forEach((c) => validateBlock(c, ctx))
      return
    case "codeBlock":
      // Verbatim text — no further validation beyond size, which is checked
      // by the Go-side authoritative pass.
      return
    case "image":
      validateStyle(block.attrs.style)
      if (!block.attrs.mediaRef) {
        throw new ValidatorError("invalid_media_ref", "image mediaRef is required")
      }
      if (!isTenantMediaRef(block.attrs.mediaRef, ctx.mediaUrlPrefix)) {
        throw new ValidatorError(
          "invalid_media_ref",
          "image mediaRef must point at the tenant media library",
        )
      }
      if (block.attrs.href) validateLink(block.attrs.href)
      return
    case "button":
      validateStyle(block.attrs.style)
      if (!block.attrs.label.trim()) {
        throw new ValidatorError("invalid_doc", "button label is required")
      }
      validateLink(block.attrs.href)
      return
    case "divider":
      validateStyle(block.attrs?.style)
      return
    case "columns": {
      validateStyle(block.attrs.style)
      const n = block.content.length
      if (n < 2 || n > 4) {
        throw new ValidatorError("invalid_doc", "columns must have 2, 3, or 4 columns")
      }
      if (block.attrs.count !== n) {
        throw new ValidatorError(
          "invalid_doc",
          `columns.attrs.count (${block.attrs.count}) does not match content length (${n})`,
        )
      }
      block.content.forEach((c) => validateColumn(c, ctx))
      return
    }
    case "rawHtml":
      // Sanitization at render time is the authoritative gate; here we
      // only guard against pathological size matching the Go limit
      // (maxRawHTMLBytes in visualdoc_validate.go). 64 KiB is generous
      // for one block.
      if (block.attrs.html.length > 64 * 1024) {
        throw new ValidatorError("invalid_doc", "raw HTML block is too large")
      }
      return
    case "listItem":
    case "column":
      throw new ValidatorError(
        "invalid_doc",
        `${block.type} cannot appear at the document root`,
      )
  }
}

function validateListItem(item: ListItemBlock, ctx: ValidateBlocksContext): void {
  item.content.forEach((c) => validateBlock(c, ctx))
}

function validateColumn(col: ColumnBlock, ctx: ValidateBlocksContext): void {
  validateStyle(col.attrs?.style)
  col.content.forEach((c) => validateBlock(c, ctx))
}

function validateInline(inline: Inline, ctx: ValidateBlocksContext): void {
  if (inline.type === "text") {
    if (inline.marks) inline.marks.forEach(validateMark)
    return
  }
  validateMergeTag(inline, ctx)
}

function validateMark(m: Mark): void {
  if (m.type === "link") {
    validateLink(m.attrs.href)
  }
  // color marks are passed to the renderer as inline style; CSS-level
  // validation happens at render time. Other marks are structural and
  // need no further checks.
}

function validateMergeTag(
  inline: Extract<Inline, { type: "mergeTag" }>,
  ctx: ValidateBlocksContext,
): void {
  // Read through `string` so we still defend against runtime input where
  // namespace is something the type system never anticipated — the
  // typed-but-still-from-JSON nature of the validator boundary.
  const namespace = inline.attrs.namespace as string
  const { key } = inline.attrs
  if (namespace === "subscriber") {
    if (!key) {
      throw new ValidatorError("invalid_doc", "subscriber merge tag is missing a key")
    }
    if (!ctx.knownSlugs.has(key)) {
      if (ctx.unknownPlaceholders) ctx.unknownPlaceholders.push(`subscriber.${key}`)
      else
        throw new ValidatorError(
          "unknown_placeholder",
          `subscriber field not defined: ${key}`,
          [`subscriber.${key}`],
        )
    }
    return
  }
  if (namespace === "campaign") {
    if (!AllowedCampaignMergeTags.has(key)) {
      throw new ValidatorError(
        "invalid_doc",
        `unknown campaign merge tag: ${key}`,
      )
    }
    return
  }
  throw new ValidatorError(
    "invalid_doc",
    "merge tag namespace must be 'subscriber' or 'campaign'",
  )
}

function isTenantMediaRef(ref: string, prefix: string): boolean {
  if (!prefix) {
    // No prefix configured — accept anything; the Go-side revalidation pass
    // is the authoritative gate in this configuration (dev only).
    return true
  }
  return ref.startsWith(prefix)
}
