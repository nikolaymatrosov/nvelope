// In-house drag-handle ProseMirror plugin. Attaches a hover-revealed handle
// to the left of every top-level block. The handle supports two affordances:
//
//   1. Drag — uses ProseMirror's native drop-cursor behavior. The
//      `draggable=true` attribute on the handle wires the browser drag
//      events; ProseMirror's editor view translates the drop into a node
//      move.
//   2. Quick-add — clicking the handle emits a `visual-editor:slash-open`
//      DOM event carrying the anchor position. The slash-command menu UI
//      listens for it and opens anchored at the block.
//
// This is intentionally a thin in-house implementation — see plan.md
// "Constraints" (no TipTap Pro / no `@tiptap/extension-drag-handle-pro`).
// We rely only on ProseMirror primitives shipped with TipTap MIT core.

import { Extension } from "@tiptap/core"
import { Plugin, PluginKey } from "@tiptap/pm/state"
import { Decoration, DecorationSet } from "@tiptap/pm/view"

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
                  () => buildHandle(),
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

function buildHandle(): HTMLElement {
  const el = document.createElement("button")
  el.type = "button"
  el.setAttribute("data-testid", "ve-drag-handle")
  el.setAttribute("aria-label", "Block actions")
  el.className = "ve-drag-handle"
  el.draggable = true
  el.textContent = "⋮⋮"
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
