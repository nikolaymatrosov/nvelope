// In-house drag-handle ProseMirror plugin. Attaches a hover-revealed handle
// to the left of every top-level block. The handle supports two affordances:
//
//   1. Drag — on `dragstart` we select the handle's block as a
//      `NodeSelection`, then let ProseMirror's native dragstart/drop
//      pipeline carry the move. This step is essential: native dragstart
//      only initiates a *node move* when the dragged element is part of a
//      `draggable` schema node or a NodeSelection already exists. A bare
//      `draggable=true` widget on an ordinary paragraph yields an empty
//      drag slice, so the drop is a no-op and the block stays put.
//   2. Quick-add — clicking the handle emits a `visual-editor:slash-open`
//      DOM event carrying the anchor position. The slash-command menu UI
//      listens for it and opens anchored at the block.
//
// This is intentionally a thin in-house implementation — see plan.md
// "Constraints" (no TipTap Pro / no `@tiptap/extension-drag-handle-pro`).
// We rely only on ProseMirror primitives shipped with TipTap MIT core.

import { Extension } from "@tiptap/core"
import { NodeSelection, Plugin, PluginKey } from "@tiptap/pm/state"
import { Decoration, DecorationSet } from "@tiptap/pm/view"
import type { EditorView } from "@tiptap/pm/view"

export const dragHandlePluginKey = new PluginKey("visualEditorDragHandle")

export const DragHandle = Extension.create({
  name: "visualEditorDragHandle",
  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: dragHandlePluginKey,
        props: {
          decorations(state) {
            const decorations: Array<Decoration> = []
            state.doc.forEach((_node, offset) => {
              decorations.push(
                Decoration.widget(
                  offset + 1,
                  (view, getPos) => buildHandle(view, getPos),
                  { side: -1, key: `drag-${offset}` },
                ),
              )
            })
            return DecorationSet.create(state.doc, decorations)
          },
        },
      }),
    ]
  },
})

function buildHandle(
  view: EditorView,
  getPos: () => number | undefined,
): HTMLElement {
  const el = document.createElement("button")
  el.type = "button"
  el.setAttribute("data-testid", "ve-drag-handle")
  el.setAttribute("aria-label", "Block actions")
  el.className = "ve-drag-handle"
  el.draggable = true
  el.textContent = "⋮⋮"
  el.addEventListener("dragstart", () => {
    // The widget anchors at `offset + 1` (just inside the block), so the
    // top-level block begins one position earlier. Selecting it as a
    // NodeSelection is what makes ProseMirror's native dragstart pick up
    // the whole block as the drag slice and move it on drop.
    const widgetPos = getPos()
    if (widgetPos == null) return
    const blockPos = widgetPos - 1
    if (blockPos < 0) return
    try {
      const selection = NodeSelection.create(view.state.doc, blockPos)
      view.dispatch(view.state.tr.setSelection(selection))
    } catch {
      // blockPos was not a valid node boundary — leave the selection be.
    }
  })
  el.addEventListener("click", (e) => {
    e.preventDefault()
    el.dispatchEvent(
      new CustomEvent("visual-editor:slash-open", {
        bubbles: true,
        detail: {
          // The slash-command menu UI reads `clientX`/`clientY` from the
          // anchor to position itself.
          rect: el.getBoundingClientRect(),
        },
      }),
    )
  })
  return el
}
