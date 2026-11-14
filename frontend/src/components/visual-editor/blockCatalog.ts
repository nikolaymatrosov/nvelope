// blockCatalog — the single list of blocks an operator can insert. The block
// library (panels/BlockLibrary) and the slash menu (ui/SlashCommandMenu) both
// read it, so a block added here shows up in both places with the same i18n
// label (`visualEditor:catalog.item.<key>`). insertBlock() is the library's
// insertion rule: always a new top-level block.

import { buildColumnsNode } from "./extensions/Columns"
import type { Editor, JSONContent } from "@tiptap/core"

export type CatalogType =
  | "text"
  | "heading"
  | "image"
  | "button"
  | "columns"
  | "divider"
  | "html"

export type CatalogKey =
  | "paragraph"
  | "bullet-list"
  | "ordered-list"
  | "quote"
  | "code"
  | "heading-1"
  | "heading-2"
  | "heading-3"
  | "image"
  | "button"
  | "columns-2"
  | "columns-3"
  | "columns-4"
  | "divider"
  | "raw-html"

export type CatalogItem = {
  key: CatalogKey
  type: CatalogType
  content: () => JSONContent
}

export const CATALOG_TYPES: ReadonlyArray<CatalogType> = [
  "text",
  "heading",
  "image",
  "button",
  "columns",
  "divider",
  "html",
]

const emptyList = (type: "bulletList" | "orderedList"): JSONContent => ({
  type,
  content: [{ type: "listItem", content: [{ type: "paragraph" }] }],
})

export const BLOCK_CATALOG: ReadonlyArray<CatalogItem> = [
  { key: "paragraph", type: "text", content: () => ({ type: "paragraph" }) },
  { key: "bullet-list", type: "text", content: () => emptyList("bulletList") },
  { key: "ordered-list", type: "text", content: () => emptyList("orderedList") },
  {
    key: "quote",
    type: "text",
    content: () => ({ type: "blockquote", content: [{ type: "paragraph" }] }),
  },
  { key: "code", type: "text", content: () => ({ type: "codeBlock" }) },
  { key: "heading-1", type: "heading", content: () => ({ type: "heading", attrs: { level: 1 } }) },
  { key: "heading-2", type: "heading", content: () => ({ type: "heading", attrs: { level: 2 } }) },
  { key: "heading-3", type: "heading", content: () => ({ type: "heading", attrs: { level: 3 } }) },
  {
    key: "image",
    type: "image",
    content: () => ({ type: "image", attrs: { mediaRef: "", alt: "", href: "" } }),
  },
  {
    key: "button",
    type: "button",
    content: () => ({ type: "button", attrs: { label: "Button", href: "" } }),
  },
  { key: "columns-2", type: "columns", content: () => buildColumnsNode(2) },
  { key: "columns-3", type: "columns", content: () => buildColumnsNode(3) },
  { key: "columns-4", type: "columns", content: () => buildColumnsNode(4) },
  { key: "divider", type: "divider", content: () => ({ type: "divider" }) },
  { key: "raw-html", type: "html", content: () => ({ type: "rawHtml", attrs: { html: "" } }) },
]

// insertBlock inserts `content` as a new top-level block right after the
// top-level block that contains `selectedPos`, or at the end of the document
// when nothing is selected. An empty document (a single empty paragraph) is
// replaced rather than appended to. Returns the new block's position.
export function insertBlock(
  editor: Editor,
  selectedPos: number | null,
  content: JSONContent,
): number {
  const { doc } = editor.state
  if (editor.isEmpty) {
    editor.chain().insertContentAt({ from: 0, to: doc.content.size }, content).run()
    return 0
  }
  let at = doc.content.size
  if (selectedPos != null) {
    const $pos = doc.resolve(selectedPos)
    const top = $pos.depth === 0 ? selectedPos : $pos.before(1)
    const topNode = doc.nodeAt(top)
    if (topNode) at = top + topNode.nodeSize
  }
  editor.chain().insertContentAt(at, content).run()
  return at
}
