// Tests for StructureOutline (T032) — projection fidelity, click-to-select
// sync, reorder among siblings, invalid-target rejection, delete/duplicate, and
// container collapse. The outline derives from a live ProseMirror doc, so we
// mount a headless editor + the shared selection hook and drive behavior
// through the rendered controls / the exported moveBlock helper.

import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import { useEffect } from "react"
import { EditorContent, useEditor } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"
import { Column, Columns } from "../extensions/Columns"
import { Divider } from "../extensions/Divider"
import { useBlockSelection } from "../hooks/useBlockSelection"
import { StructureOutline, moveBlock } from "./StructureOutline"
import type { Editor } from "@tiptap/core"

beforeAll(() => {
  if (typeof Range.prototype.getClientRects !== "function") {
    Range.prototype.getClientRects = () => [] as unknown as DOMRectList
  }
  if (typeof Range.prototype.getBoundingClientRect !== "function") {
    Range.prototype.getBoundingClientRect = () => new DOMRect(0, 0, 0, 0)
  }
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

const nestedDoc = {
  type: "doc",
  content: [
    { type: "heading", attrs: { level: 1 }, content: [{ type: "text", text: "Title" }] },
    { type: "paragraph", content: [{ type: "text", text: "intro" }] },
    {
      type: "columns",
      attrs: { count: 2 },
      content: [
        { type: "column", content: [{ type: "paragraph", content: [{ type: "text", text: "left" }] }] },
        { type: "column", content: [{ type: "paragraph", content: [{ type: "text", text: "right" }] }] },
      ],
    },
  ],
}

const threeParas = {
  type: "doc",
  content: [
    { type: "paragraph", content: [{ type: "text", text: "AAA" }] },
    { type: "paragraph", content: [{ type: "text", text: "BBB" }] },
    { type: "paragraph", content: [{ type: "text", text: "CCC" }] },
  ],
}

function Harness({ content, onReady }: { content: object; onReady: (e: Editor) => void }) {
  const editor = useEditor({
    extensions: [StarterKit.configure({ hardBreak: false }), Columns, Column, Divider],
    content,
  })
  const selection = useBlockSelection(editor)
  useEffect(() => {
    onReady(editor)
  }, [editor, onReady])
  return (
    <div>
      <StructureOutline editor={editor} selection={selection} />
      <EditorContent editor={editor} />
    </div>
  )
}

function renderHarness(content: object) {
  let editor: Editor | null = null
  render(<Harness content={content} onReady={(e) => (editor = e)} />)
  return () => editor
}

function topLevelPositions(editor: Editor): Array<number> {
  const positions: Array<number> = []
  editor.state.doc.forEach((_node, offset) => positions.push(offset))
  return positions
}

describe("StructureOutline", () => {
  it("projects the document hierarchy including nested columns", async () => {
    renderHarness(nestedDoc)
    await waitFor(() => expect(screen.getByTestId("ve-outline")).toBeTruthy())
    expect(screen.getByTestId("ve-outline-row-heading")).toBeTruthy()
    expect(screen.getByTestId("ve-outline-row-columns")).toBeTruthy()
    // One top-level intro paragraph + one paragraph nested in each column.
    expect(screen.getAllByTestId("ve-outline-row-paragraph")).toHaveLength(3)
    // Two column entries nested under the columns container.
    expect(screen.getAllByTestId("ve-outline-row-column")).toHaveLength(2)
  })

  it("selects the block when its outline entry is clicked", async () => {
    const get = renderHarness(nestedDoc)
    await waitFor(() => expect(get()).toBeTruthy())
    const editor = get()!
    const headingPos = topLevelPositions(editor)[0]
    fireEvent.click(screen.getByTestId(`ve-outline-select-${headingPos}`))
    await waitFor(() => {
      expect(editor.state.doc.nodeAt(headingPos)?.type.name).toBe("heading")
      // The row reflects the selection.
      const row = screen.getByTestId("ve-outline-row-heading")
      expect(row.className).toContain("is-selected")
    })
  })

  it("reorders sibling blocks via moveBlock", async () => {
    const get = renderHarness(threeParas)
    await waitFor(() => expect(get()).toBeTruthy())
    const editor = get()!
    const [p0, , p2] = topLevelPositions(editor)
    // Move the third paragraph (CCC) before the first (AAA).
    expect(moveBlock(editor, p2, p0)).toBe(true)
    const first = editor.state.doc.child(0)
    expect(first.textContent).toBe("CCC")
  })

  it("rejects a reorder across different parents", async () => {
    const get = renderHarness(nestedDoc)
    await waitFor(() => expect(get()).toBeTruthy())
    const editor = get()!
    const before = editor.getJSON()
    const [, paraPos] = topLevelPositions(editor)
    // Find a nested paragraph position (inside the first column).
    let nestedParaPos = -1
    editor.state.doc.descendants((node, pos) => {
      if (nestedParaPos < 0 && node.type.name === "paragraph" && node.textContent === "left") {
        nestedParaPos = pos
      }
      return true
    })
    // Top-level paragraph and a column-nested paragraph have different parents.
    expect(moveBlock(editor, paraPos, nestedParaPos)).toBe(false)
    expect(editor.getJSON()).toEqual(before)
  })

  it("deletes and duplicates a block from the outline", async () => {
    const get = renderHarness(threeParas)
    await waitFor(() => expect(get()).toBeTruthy())
    const editor = get()!
    expect(editor.state.doc.childCount).toBe(3)
    const firstPos = topLevelPositions(editor)[0]
    fireEvent.click(screen.getByTestId(`ve-outline-duplicate-${firstPos}`))
    await waitFor(() => expect(editor.state.doc.childCount).toBe(4))
    const delPos = topLevelPositions(editor)[0]
    fireEvent.click(screen.getByTestId(`ve-outline-delete-${delPos}`))
    await waitFor(() => expect(editor.state.doc.childCount).toBe(3))
  })

  it("collapses a container entry to hide its children", async () => {
    const get = renderHarness(nestedDoc)
    await waitFor(() => expect(get()).toBeTruthy())
    const editor = get()!
    const columnsPos = topLevelPositions(editor)[2]
    expect(screen.getAllByTestId("ve-outline-row-column")).toHaveLength(2)
    fireEvent.click(screen.getByTestId(`ve-outline-toggle-${columnsPos}`))
    await waitFor(() => expect(screen.queryByTestId("ve-outline-row-column")).toBeNull())
  })
})
