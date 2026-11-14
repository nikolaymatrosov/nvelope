import { afterEach, beforeAll, describe, expect, it } from "vitest"
import { Editor } from "@tiptap/core"
import StarterKit from "@tiptap/starter-kit"
import { Column, Columns, buildColumnsNode, setColumnsCount } from "./Columns"
import type { JSONContent } from "@tiptap/core"

beforeAll(() => {
  if (typeof Range.prototype.getClientRects !== "function") {
    Range.prototype.getClientRects = () => [] as unknown as DOMRectList
  }
  if (typeof Range.prototype.getBoundingClientRect !== "function") {
    Range.prototype.getBoundingClientRect = () => new DOMRect(0, 0, 0, 0)
  }
})

let editor: Editor | null = null
afterEach(() => editor?.destroy())

const para = (text: string): JSONContent => ({ type: "paragraph", content: [{ type: "text", text }] })

function columnsDoc(texts: Array<string>): JSONContent {
  return {
    type: "doc",
    content: [
      {
        type: "columns",
        attrs: { count: texts.length },
        content: texts.map((t) => ({ type: "column", content: [para(t)] })),
      },
    ],
  }
}

function make(content: JSONContent): Editor {
  editor = new Editor({ extensions: [StarterKit.configure({ hardBreak: false }), Columns, Column], content })
  return editor
}

describe("setColumnsCount", () => {
  it("adds empty columns when growing", () => {
    const ed = make(columnsDoc(["a", "b"]))
    setColumnsCount(ed, 0, 3)
    const node = ed.state.doc.child(0)
    expect(node.attrs.count).toBe(3)
    expect(node.childCount).toBe(3)
    expect(node.child(2).textContent).toBe("")
  })

  it("folds removed columns into the last kept column when shrinking", () => {
    const ed = make(columnsDoc(["a", "b", "c", "d"]))
    setColumnsCount(ed, 0, 2)
    const node = ed.state.doc.child(0)
    expect(node.attrs.count).toBe(2)
    expect(node.childCount).toBe(2)
    expect(node.child(0).textContent).toBe("a")
    expect(node.child(1).textContent).toBe("bcd")
  })

  it("is a no-op for the same count or a non-columns node", () => {
    const ed = make(columnsDoc(["a", "b"]))
    const before = ed.getJSON()
    setColumnsCount(ed, 0, 2)
    setColumnsCount(ed, 1, 3)
    expect(ed.getJSON()).toEqual(before)
  })

  it("keeps buildColumnsNode's count invariant", () => {
    expect(buildColumnsNode(3).content).toHaveLength(3)
  })
})
