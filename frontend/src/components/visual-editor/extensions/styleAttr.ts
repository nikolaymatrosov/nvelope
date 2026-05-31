// Shared per-block `style` attribute for the three-pane editor (feature 017).
//
// The parameters panel writes a BlockStyle object onto the selected block's
// `style` attribute; this module is how that attribute lives on the TipTap
// nodes:
//   - `BlockStyleAttribute` adds `style` to the StarterKit nodes (paragraph,
//     heading, lists, blockquote) via addGlobalAttributes.
//   - `blockStyleAttributeSpec` is reused by the custom nodes (Button, Columns,
//     Column, Divider, ImageBlock) in their own addAttributes.
//   - `blockStyleToCss` renders the object to an inline CSS string so the
//     canvas reflects the style live (FR-014); the email-ready CSS is produced
//     server-side by the BFF renderer (mapBlockStyle).
//   - `pruneBlockStyles` strips null/empty style from the emitted VisualDoc so
//     an unstyled block stays `{ type: "paragraph", content: [...] }` with no
//     noisy `attrs: { style: null }` (absent ⇒ inherit).

import { Extension } from "@tiptap/core"
import type { BlockStyle } from "@/lib/api-types"

const PX_FIELDS: Array<[keyof BlockStyle, string]> = [
  ["fontSize", "font-size"],
  ["paddingTop", "padding-top"],
  ["paddingRight", "padding-right"],
  ["paddingBottom", "padding-bottom"],
  ["paddingLeft", "padding-left"],
  ["borderRadius", "border-radius"],
  ["borderWidth", "border-width"],
]

// blockStyleToCss renders a BlockStyle to an inline CSS declaration string for
// the canvas DOM. Mirrors the BFF renderer's mapBlockStyle property mapping.
export function blockStyleToCss(style?: BlockStyle | null): string {
  if (!style) return ""
  const parts: Array<string> = []
  if (style.backgroundColor) parts.push(`background-color:${style.backgroundColor}`)
  if (style.color) parts.push(`color:${style.color}`)
  if (style.fontFamily) parts.push(`font-family:${style.fontFamily}`)
  if (style.fontWeight != null) parts.push(`font-weight:${style.fontWeight}`)
  if (style.lineHeight != null) parts.push(`line-height:${style.lineHeight}`)
  if (style.textAlign) parts.push(`text-align:${style.textAlign}`)
  for (const [key, css] of PX_FIELDS) {
    const v = style[key]
    if (typeof v === "number") parts.push(`${css}:${v}px`)
  }
  if (style.borderStyle) parts.push(`border-style:${style.borderStyle}`)
  if (style.borderColor) parts.push(`border-color:${style.borderColor}`)
  return parts.join(";")
}

// isEmptyBlockStyle reports whether a style carries no set field (so it is
// equivalent to "inherit" and should not be serialized).
export function isEmptyBlockStyle(style?: BlockStyle | null): boolean {
  if (!style) return true
  // Values come from JSON at runtime, so guard against null too (== null).
  return Object.values(style as Record<string, unknown>).every(
    (v) => v == null || v === "",
  )
}

// blockStyleAttributeSpec is the TipTap attribute definition for `style`,
// shared by every styleable node. The object is stored verbatim in the node's
// attrs (so getJSON yields the BlockStyle); on the DOM it is mirrored as inline
// CSS plus a data-ve-style round-trip hook for HTML paste.
export const blockStyleAttributeSpec = {
  default: null as BlockStyle | null,
  parseHTML: (element: HTMLElement): BlockStyle | null => {
    const raw = element.getAttribute("data-ve-style")
    if (!raw) return null
    try {
      return JSON.parse(raw) as BlockStyle
    } catch {
      return null
    }
  },
  renderHTML: (attributes: Record<string, unknown>): Record<string, string> => {
    const style = attributes.style as BlockStyle | null
    if (isEmptyBlockStyle(style)) return {}
    return {
      "data-ve-style": JSON.stringify(style),
      style: blockStyleToCss(style),
    }
  },
  keepOnSplit: false,
}

// BlockStyleAttribute adds the `style` attribute to the StarterKit block nodes
// that the parameters panel can target.
export const BlockStyleAttribute = Extension.create({
  name: "blockStyleAttribute",
  addGlobalAttributes() {
    return [
      {
        types: ["paragraph", "heading", "bulletList", "orderedList", "blockquote"],
        attributes: { style: blockStyleAttributeSpec },
      },
    ]
  },
})

// pruneBlockStyles walks a VisualDoc content tree and removes `style` attrs that
// are null/empty, and removes an `attrs` object that becomes empty as a result.
// Mutates in place (the caller passes a fresh getJSON() result) and returns it.
export function pruneBlockStyles<T>(content: T): T {
  const walk = (node: unknown): void => {
    if (!node || typeof node !== "object") return
    const n = node as { attrs?: Record<string, unknown>; content?: Array<unknown> }
    if (n.attrs && "style" in n.attrs && isEmptyBlockStyle(n.attrs.style as BlockStyle | null)) {
      delete n.attrs.style
      if (Object.keys(n.attrs).length === 0) delete n.attrs
    }
    if (Array.isArray(n.content)) n.content.forEach(walk)
  }
  if (Array.isArray(content)) content.forEach(walk)
  return content
}
