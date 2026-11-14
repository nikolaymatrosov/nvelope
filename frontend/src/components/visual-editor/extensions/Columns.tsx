// Multi-column block — exposed in the JSON as
// `{ type: "columns", attrs: { count }, content: [Column, …] }` plus
// `{ type: "column", content: [...blocks] }`. The editor renders the columns
// in a CSS grid so the operator can edit each column inline; the BFF
// renderer is responsible for emitting the table-based HTML that Outlook
// requires (per FR-015 and research.md § R4).

import { Node, mergeAttributes } from "@tiptap/core"
import { NodeSelection } from "@tiptap/pm/state"
import { blockStyleAttributeSpec } from "./styleAttr"
import type { Editor } from "@tiptap/core"
import type { Node as PMNode } from "@tiptap/pm/model"

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

// setColumnsCount changes a columns block to `count` columns in one
// transaction, keeping `attrs.count === content length` (validation
// invariant). Growing appends empty columns. Shrinking folds the removed
// columns' content into the last kept column, so nothing is lost. The columns
// block stays node-selected so the parameters panel keeps showing it.
export function setColumnsCount(editor: Editor, pos: number, count: ColumnsCount): void {
  const node = editor.state.doc.nodeAt(pos)
  if (!node || node.type.name !== "columns" || node.childCount === count) return
  const { schema } = editor
  const columns: Array<PMNode> = []
  node.forEach((child) => columns.push(child))

  let next: Array<PMNode>
  if (count > columns.length) {
    const added = Array.from({ length: count - columns.length }, () =>
      schema.nodes.column.create(null, schema.nodes.paragraph.create()),
    )
    next = [...columns, ...added]
  } else {
    const kept = columns.slice(0, count)
    const last = kept[count - 1]
    const merged = columns
      .slice(count)
      .reduce((acc, col) => acc.append(col.content), last.content)
    next = [...kept.slice(0, count - 1), last.copy(merged)]
  }

  const replacement = node.type.create({ ...node.attrs, count }, next)
  editor
    .chain()
    .focus()
    .command(({ tr }) => {
      tr.replaceWith(pos, pos + node.nodeSize, replacement)
      tr.setSelection(NodeSelection.create(tr.doc, pos))
      return true
    })
    .run()
}
