// Tests for useBlockSelection (T005) — the single selection model shared by
// the three panes. jsdom + ProseMirror don't simulate real pointer hit-testing,
// so we drive selection through the editor command API (the same transactions a
// click produces) and assert the hook tracks it: it derives the block at the
// cursor, selectBlock() selects a node block, and selection clears when the
// selected block is deleted.

import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { act, cleanup, fireEvent, render, renderHook, waitFor } from "@testing-library/react"
import { EditorContent, useEditor } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"
import { Editor } from "@tiptap/core"
import { Divider } from "../extensions/Divider"
import { useBlockSelection } from "./useBlockSelection"

beforeAll(() => {
  // jsdom doesn't implement Range rect APIs that ProseMirror's coordsAtPos /
  // scrollToSelection rely on when a transaction scrolls the selection into
  // view. Stub them so dispatches don't throw (same stub the editor test uses).
  if (typeof Range.prototype.getClientRects !== "function") {
    Range.prototype.getClientRects = () => [] as unknown as DOMRectList
  }
  if (typeof Range.prototype.getBoundingClientRect !== "function") {
    Range.prototype.getBoundingClientRect = () => new DOMRect(0, 0, 0, 0)
  }
  // ProseMirror's mousedown handler calls posAtCoords -> elementFromPoint,
  // which jsdom lacks; an unhandled throw fails the whole run.
  if (typeof document.elementFromPoint !== "function") {
    document.elementFromPoint = () => null
  }
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

// content: paragraph("hello"), divider, paragraph("world")
const content = {
  type: "doc",
  content: [
    { type: "paragraph", content: [{ type: "text", text: "hello" }] },
    { type: "divider" },
    { type: "paragraph", content: [{ type: "text", text: "world" }] },
  ],
}

type HarnessHandle = {
  editor: Editor | null
  selectedType: string
  selectedPos: number | null
  selectBlock: (pos: number) => void
  clear: () => void
}

function Harness({ onState }: { onState: (h: HarnessHandle) => void }) {
  const editor = useEditor({
    extensions: [StarterKit.configure({ hardBreak: false }), Divider],
    content,
  })
  const selection = useBlockSelection(editor)
  onState({
    editor,
    selectedType: selection.selectedNode?.type.name ?? "none",
    selectedPos: selection.selectedPos,
    selectBlock: selection.selectBlock,
    clear: selection.clear,
  })
  return <EditorContent editor={editor} />
}

function renderHarness() {
  let latest: HarnessHandle = {
    editor: null,
    selectedType: "none",
    selectedPos: null,
    selectBlock: () => {},
    clear: () => {},
  }
  render(<Harness onState={(h) => (latest = h)} />)
  return () => latest
}

// dividerPos walks the top-level nodes to find the divider's position.
function dividerPos(editor: Editor): number {
  let pos = -1
  editor.state.doc.descendants((node, p) => {
    if (node.type.name === "divider") pos = p
    return true
  })
  return pos
}

describe("useBlockSelection", () => {
  it("derives the block containing the cursor", async () => {
    const get = renderHarness()
    await waitFor(() => expect(get().editor).toBeTruthy())
    const editor = get().editor!
    // Place the caret inside the first paragraph.
    fireEvent.mouseDown(editor.view.dom)
    editor.commands.setTextSelection(2)
    await waitFor(() => expect(get().selectedType).toBe("paragraph"))
  })

  it("selectBlock selects a leaf (node) block", async () => {
    const get = renderHarness()
    await waitFor(() => expect(get().editor).toBeTruthy())
    const editor = get().editor!
    const pos = dividerPos(editor)
    expect(pos).toBeGreaterThanOrEqual(0)
    get().selectBlock(pos)
    await waitFor(() => {
      expect(get().selectedType).toBe("divider")
      expect(get().selectedPos).toBe(pos)
    })
  })

  it("stops reporting a deleted block as selected", async () => {
    const get = renderHarness()
    await waitFor(() => expect(get().editor).toBeTruthy())
    const editor = get().editor!
    const pos = dividerPos(editor)
    get().selectBlock(pos)
    await waitFor(() => expect(get().selectedType).toBe("divider"))
    // Delete the selected divider node.
    editor.chain().setNodeSelection(pos).deleteSelection().run()
    await waitFor(() => expect(get().selectedType).not.toBe("divider"))
  })

  it("reports nothing selected until the operator selects", async () => {
    const get = renderHarness()
    await waitFor(() => expect(get().editor).toBeTruthy())
    expect(get().selectedPos).toBeNull()
    expect(get().selectedType).toBe("none")
  })

  it("activates on a mousedown in the canvas", async () => {
    const get = renderHarness()
    await waitFor(() => expect(get().editor).toBeTruthy())
    fireEvent.mouseDown(get().editor!.view.dom)
    await waitFor(() => expect(get().selectedType).toBe("paragraph"))
  })

  it("clear() keeps the selection empty across later transactions", async () => {
    const get = renderHarness()
    await waitFor(() => expect(get().editor).toBeTruthy())
    const editor = get().editor!
    get().selectBlock(dividerPos(editor))
    await waitFor(() => expect(get().selectedType).toBe("divider"))
    act(() => get().clear())
    await waitFor(() => expect(get().selectedPos).toBeNull())
    // A programmatic edit (it moves the internal selection) must not
    // resurrect a block selection.
    act(() => {
      editor.commands.insertContentAt(editor.state.doc.content.size, { type: "paragraph" })
    })
    expect(get().selectedPos).toBeNull()
  })
})

describe("useBlockSelection editor swap", () => {
  it("resets the active flag when the editor instance changes", async () => {
    const mk = () =>
      new Editor({ extensions: [StarterKit.configure({ hardBreak: false }), Divider], content })
    const a = mk()
    const b = mk()
    const { result, rerender } = renderHook(({ ed }: { ed: Editor }) => useBlockSelection(ed), {
      initialProps: { ed: a },
    })
    fireEvent.mouseDown(a.view.dom)
    a.commands.setTextSelection(2)
    await waitFor(() => expect(result.current.selectedPos).not.toBeNull())
    rerender({ ed: b })
    await waitFor(() => expect(result.current.selectedPos).toBeNull())
    fireEvent.mouseDown(b.view.dom)
    b.commands.setTextSelection(2)
    await waitFor(() => expect(result.current.selectedPos).not.toBeNull())
    a.destroy()
    b.destroy()
  })
})
