// SelectionDecoration — a ProseMirror plugin (wrapped as a TipTap extension)
// that draws a `ve-selected` outline around the block the current selection
// points at, so the canvas visibly agrees with the structure outline and the
// parameters panel about which block is selected (FR-002 / FR-008).
//
// The decoration is a pure function of editor state — it reuses the same
// `deriveSelectedPos` logic the selection hook uses — so it needs no external
// React state and stays in sync automatically as the selection moves.

import { Extension } from "@tiptap/core"
import { Plugin, PluginKey } from "@tiptap/pm/state"
import { Decoration, DecorationSet } from "@tiptap/pm/view"
import { deriveSelectedPos } from "../hooks/useBlockSelection"

export const SelectionDecoration = Extension.create({
  name: "selectionDecoration",

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: new PluginKey("selectionDecoration"),
        props: {
          decorations(state) {
            const pos = deriveSelectedPos(state)
            if (pos == null) return null
            const node = state.doc.nodeAt(pos)
            if (!node) return null
            return DecorationSet.create(state.doc, [
              Decoration.node(pos, pos + node.nodeSize, {
                class: "ve-selected",
              }),
            ])
          },
        },
      }),
    ]
  },
})
