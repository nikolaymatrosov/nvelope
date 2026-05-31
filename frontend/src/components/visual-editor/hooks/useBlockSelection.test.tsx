// Tests for useBlockSelection (T005) — the single selection model shared by
// the three panes. jsdom + ProseMirror don't simulate real pointer hit-testing,
// so we drive selection through the editor command API (the same transactions a
// click produces) and assert the hook tracks it: it derives the block at the
// cursor, selectBlock() selects a node block, and selection clears when the
// selected block is deleted.

import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { cleanup, render, waitFor } from "@testing-library/react"
import { EditorContent, useEditor } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"
import { Divider } from "../extensions/Divider"
import { useBlockSelection } from "./useBlockSelection"
import type { Editor } from "@tiptap/core"

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
  })
  return <EditorContent editor={editor} />
}

function renderHarness() {
  let latest: HarnessHandle = {
    editor: null,
    selectedType: "none",
    selectedPos: null,
    selectBlock: () => {},
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
})
