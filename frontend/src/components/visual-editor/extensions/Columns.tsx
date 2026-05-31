// Multi-column block — exposed in the JSON as
// `{ type: "columns", attrs: { count }, content: [Column, …] }` plus
// `{ type: "column", content: [...blocks] }`. The editor renders the columns
// in a CSS grid so the operator can edit each column inline; the BFF
// renderer is responsible for emitting the table-based HTML that Outlook
// requires (per FR-015 and research.md § R4).

import { Node, mergeAttributes } from "@tiptap/core"
import { blockStyleAttributeSpec } from "./styleAttr"

export type ColumnsCount = 2 | 3 | 4

export const Column = Node.create({
  name: "column",
  group: "columnContent",
  content: "block+",
  isolating: true,
  addAttributes() {
    return { style: blockStyleAttributeSpec }
  },
  parseHTML() {
    return [{ tag: "div[data-type=\"column\"]" }]
  },
  renderHTML({ HTMLAttributes }) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "column",
        class: "ve-column",
      }),
      0,
    ]
  },
})

export const Columns = Node.create<{ defaultCount: ColumnsCount }>({
  name: "columns",
  group: "block",
  content: "column{2,4}",
  defining: true,
  addOptions() {
    return { defaultCount: 2 }
  },
  addAttributes() {
    return {
      count: {
        default: 2 as ColumnsCount,
        parseHTML: (el) => {
          const n = parseInt(el.getAttribute("data-count") ?? "2", 10)
          if (n === 2 || n === 3 || n === 4) return n
          return 2
        },
        renderHTML: (attrs) => ({ "data-count": String(attrs.count) }),
      },
      style: blockStyleAttributeSpec,
    }
  },
  parseHTML() {
    return [{ tag: "div[data-type=\"columns\"]" }]
  },
  renderHTML({ HTMLAttributes, node }) {
    const count = node.attrs.count as ColumnsCount
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "columns",
        class: "ve-columns",
        style: `display:grid;grid-template-columns:repeat(${count},minmax(0,1fr));gap:1rem`,
      }),
      0,
    ]
  },
})

// Command helper: insert a columns block with `count` empty paragraphs in
// each column. The editor's slash menu uses this so the inserted node's
// content length matches the `count` attribute (validation invariant — see
// `internal/campaign/domain/visualdoc_validate.go`).
export function buildColumnsNode(count: ColumnsCount) {
  return {
    type: "columns",
    attrs: { count },
    content: Array.from({ length: count }, () => ({
      type: "column",
      content: [{ type: "paragraph" }],
    })),
  }
}
