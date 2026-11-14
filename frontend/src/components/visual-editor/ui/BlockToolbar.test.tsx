import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import { useEffect, useRef } from "react"
import { EditorContent, useEditor } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"
import { Divider } from "../extensions/Divider"
import { useBlockSelection } from "../hooks/useBlockSelection"
import { BlockToolbar } from "./BlockToolbar"
import type { BlockSelection } from "../hooks/useBlockSelection"
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

const content = {
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text: "hi" }] }, { type: "divider" }],
}

function Harness({ onReady }: { onReady: (e: Editor, s: BlockSelection) => void }) {
  const editor = useEditor({ extensions: [StarterKit.configure({ hardBreak: false }), Divider], content })
  const selection = useBlockSelection(editor)
  const pageRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    onReady(editor, selection)
  })
  return (
    <div ref={pageRef} style={{ position: "relative" }}>
      <EditorContent editor={editor} />
      <BlockToolbar editor={editor} selection={selection} pageRef={pageRef} />
    </div>
  )
}

function setup() {
  let editor: Editor | null = null
  let selection: BlockSelection | null = null
  render(<Harness onReady={(e, s) => ((editor = e), (selection = s))} />)
  return { editor: () => editor!, selection: () => selection! }
}

function dividerPos(editor: Editor): number {
  let pos = -1
  editor.state.doc.forEach((n, p) => {
    if (n.type.name === "divider") pos = p
  })
  return pos
}

describe("BlockToolbar", () => {
  it("is hidden until a block is selected", async () => {
    const h = setup()
    await waitFor(() => expect(h.editor()).toBeTruthy())
    expect(screen.queryByTestId("ve-block-toolbar")).toBeNull()
  })

  it("labels the selected block and duplicates / deletes it", async () => {
    const h = setup()
    await waitFor(() => expect(h.editor()).toBeTruthy())
    h.selection().selectBlock(dividerPos(h.editor()))
    expect(await screen.findByText("Divider")).toBeTruthy()
    fireEvent.click(screen.getByTestId("ve-block-toolbar-duplicate"))
    await waitFor(() => {
      let dividers = 0
      h.editor().state.doc.forEach((n) => (dividers += n.type.name === "divider" ? 1 : 0))
      expect(dividers).toBe(2)
    })
    fireEvent.click(screen.getByTestId("ve-block-toolbar-delete"))
    await waitFor(() => {
      let dividers = 0
      h.editor().state.doc.forEach((n) => (dividers += n.type.name === "divider" ? 1 : 0))
      expect(dividers).toBe(1)
    })
  })
})
