// Tests for the shared block catalog and insertBlock(). The catalog feeds both
// the block library and the slash menu, so every entry needs labels in both
// locales and must build schema-valid content.

import { afterEach, beforeAll, describe, expect, it } from "vitest"
import i18n from "i18next"
import { Editor } from "@tiptap/core"
import StarterKit from "@tiptap/starter-kit"
import { BLOCK_CATALOG, CATALOG_TYPES, insertBlock } from "./blockCatalog"
import { Column, Columns } from "./extensions/Columns"
import { Divider } from "./extensions/Divider"
import type { JSONContent } from "@tiptap/core"

beforeAll(() => {
  if (typeof Range.prototype.getClientRects !== "function") {
    Range.prototype.getClientRects = () => [] as unknown as DOMRectList
  }
  if (typeof Range.prototype.getBoundingClientRect !== "function") {
    Range.prototype.getBoundingClientRect = () => new DOMRect(0, 0, 0, 0)
  }
})

const editors: Array<Editor> = []
afterEach(() => {
  editors.splice(0).forEach((e) => e.destroy())
})

function makeEditor(content: JSONContent): Editor {
  const editor = new Editor({
    extensions: [StarterKit.configure({ hardBreak: false }), Columns, Column, Divider],
    content,
  })
  editors.push(editor)
  return editor
}

const para = (text: string): JSONContent => ({ type: "paragraph", content: [{ type: "text", text }] })

function childTypes(editor: Editor): Array<string> {
  const out: Array<string> = []
  editor.state.doc.forEach((n) => out.push(n.type.name))
  return out
}

describe("BLOCK_CATALOG", () => {
  it("has unique keys and only declared types", () => {
    const keys = BLOCK_CATALOG.map((i) => i.key)
    expect(new Set(keys).size).toBe(keys.length)
    for (const item of BLOCK_CATALOG) expect(CATALOG_TYPES).toContain(item.type)
  })

  it("has a label for every item and type in en and ru", () => {
    for (const lng of ["en", "ru"]) {
      for (const item of BLOCK_CATALOG) {
        expect(i18n.exists(`visualEditor:catalog.item.${item.key}`, { lng })).toBe(true)
      }
      for (const type of CATALOG_TYPES) {
        expect(i18n.exists(`visualEditor:catalog.type.${type}`, { lng })).toBe(true)
      }
    }
  })

  it("builds columns whose content length matches the count", () => {
    for (const item of BLOCK_CATALOG.filter((i) => i.type === "columns")) {
      const node = item.content()
      expect(node.content).toHaveLength(node.attrs?.count as number)
    }
  })
})

describe("insertBlock", () => {
  it("inserts after the top-level block containing the selection", () => {
    const editor = makeEditor({ type: "doc", content: [para("AAA"), para("BBB")] })
    const at = insertBlock(editor, 0, { type: "divider" })
    expect(childTypes(editor).slice(0, 3)).toEqual(["paragraph", "divider", "paragraph"])
    expect(editor.state.doc.nodeAt(at)?.type.name).toBe("divider")
  })

  it("inserts after the enclosing top-level block when the selection is nested", () => {
    const editor = makeEditor({
      type: "doc",
      content: [
        {
          type: "columns",
          attrs: { count: 2 },
          content: [
            { type: "column", content: [para("left")] },
            { type: "column", content: [para("right")] },
          ],
        },
        para("after"),
      ],
    })
    let nested = -1
    editor.state.doc.descendants((n, p) => {
      if (nested < 0 && n.type.name === "paragraph") nested = p
      return true
    })
    insertBlock(editor, nested, { type: "divider" })
    expect(childTypes(editor).slice(0, 3)).toEqual(["columns", "divider", "paragraph"])
  })

  it("appends at the end when nothing is selected", () => {
    const editor = makeEditor({ type: "doc", content: [para("AAA")] })
    const at = insertBlock(editor, null, { type: "divider" })
    expect(editor.state.doc.nodeAt(at)?.type.name).toBe("divider")
    expect(childTypes(editor)[0]).toBe("paragraph")
  })

  it("replaces an empty document instead of appending", () => {
    const editor = makeEditor({ type: "doc", content: [{ type: "paragraph" }] })
    const at = insertBlock(editor, null, { type: "heading", attrs: { level: 1 } })
    expect(at).toBe(0)
    expect(childTypes(editor)[0]).toBe("heading")
  })
})
