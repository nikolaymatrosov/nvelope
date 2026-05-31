// Regression test for the drag-to-reorder bug: grabbing a block's drag
// handle and dragging it did nothing because the handle never told
// ProseMirror which node to move. Native `dragstart` only initiates a node
// move when the block is `draggable` in its schema or a NodeSelection
// exists — neither is true for an ordinary paragraph, so the drop produced
// an empty slice and the block stayed put.
//
// jsdom has no layout/drag-image support, so we assert the contract our
// handler owns: a `dragstart` on a block's handle selects that block as a
// NodeSelection. Once the selection is the node, ProseMirror's own (well
// tested) dragstart/drop pipeline performs the actual move.

import { afterEach, describe, expect, it } from "vitest"
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react"
import { EditorContent, useEditor } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"
import { NodeSelection } from "@tiptap/pm/state"
import { DragHandle } from "./DragHandle"
import type { Editor } from "@tiptap/core"

afterEach(() => {
  cleanup()
})

describe("DragHandle reorder", () => {
  it("selects the block as a NodeSelection on dragstart", async () => {
    let editorRef: Editor | null = null
    function Headless() {
      const editor = useEditor({
        extensions: [StarterKit.configure({ hardBreak: false }), DragHandle],
        content: {
          type: "doc",
          content: [
            { type: "paragraph", content: [{ type: "text", text: "First" }] },
            { type: "paragraph", content: [{ type: "text", text: "Second" }] },
          ],
        },
      })
      editorRef = editor
      return <EditorContent editor={editor} />
    }
    render(<Headless />)
    await waitFor(() => expect(editorRef).toBeTruthy())

    const handles = await screen.findAllByTestId("ve-drag-handle")
    expect(handles).toHaveLength(2)

    // Grab the second block's handle. The first paragraph ("First") has
    // nodeSize 1 + 5 + 1 = 7, so the second paragraph begins at pos 7.
    fireEvent.dragStart(handles[1])

    await waitFor(() => {
      const sel = editorRef!.state.selection
      expect(sel instanceof NodeSelection).toBe(true)
      expect(sel.from).toBe(7)
    })
  })
})
