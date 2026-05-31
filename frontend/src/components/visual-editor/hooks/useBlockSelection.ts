// useBlockSelection — the single block-selection state shared by all three
// panes of the three-pane editor (FR-002). The canvas, the structure outline,
// and the parameters panel all read and write selection through this one hook
// so they can never disagree about "what is selected" (SC-004).
//
// Selection is keyed on the ProseMirror document position of the selected
// block. Because we re-derive that position from the editor's *live* selection
// on every transaction, it is inherently mapping-stable: when content is
// inserted/removed/reordered, ProseMirror remaps the selection and we recompute
// the position from it. When the selected block is deleted and the selection
// lands on no block, the position resolves to null and the params panel shows
// its empty state.

import { useCallback, useEffect, useState } from "react"
import { NodeSelection } from "@tiptap/pm/state"
import type { Editor } from "@tiptap/core"
import type { Node as PMNode } from "@tiptap/pm/model"
import type { EditorState } from "@tiptap/pm/state"

// deriveSelectedPos returns the document position of the block the current
// selection points at, or null when the selection is not inside a block we can
// edit. A NodeSelection (e.g. an image/button/columns selected as a unit)
// resolves to its own position; a text selection resolves to the position of
// the deepest block ancestor containing the cursor.
export function deriveSelectedPos(state: EditorState): number | null {
  const sel = state.selection
  if (sel instanceof NodeSelection) {
    return sel.from
  }
  const $from = sel.$from
  for (let depth = $from.depth; depth >= 1; depth--) {
    const node = $from.node(depth)
    if (node.type.name !== "doc" && node.isBlock) {
      return $from.before(depth)
    }
  }
  return null
}

export type BlockSelection = {
  // Position of the selected block in the document, or null when nothing is
  // selected.
  selectedPos: number | null
  // The selected block node (re-resolved on every transaction so attr changes
  // are reflected), or null.
  selectedNode: PMNode | null
  // Select the block at `pos`: sets the editor selection (NodeSelection for
  // leaf/atom blocks, a caret for text blocks), scrolls it into view, and
  // focuses the canvas.
  selectBlock: (pos: number) => void
  // Merge `attrs` into the selected block's attributes via a single
  // transaction; no-op when nothing is selected.
  updateSelectedAttrs: (attrs: Record<string, unknown>) => void
  // Clear the selection (used to surface the params empty state).
  clear: () => void
}

export function useBlockSelection(editor: Editor | null): BlockSelection {
  const [state, setState] = useState<{ pos: number | null; node: PMNode | null }>(
    { pos: null, node: null },
  )

  useEffect(() => {
    if (!editor) {
      setState({ pos: null, node: null })
      return
    }
    const sync = () => {
      const pos = deriveSelectedPos(editor.state)
      const node = pos == null ? null : editor.state.doc.nodeAt(pos)
      setState({ pos, node })
    }
    sync()
    editor.on("selectionUpdate", sync)
    editor.on("transaction", sync)
    return () => {
      editor.off("selectionUpdate", sync)
      editor.off("transaction", sync)
    }
  }, [editor])

  const selectBlock = useCallback(
    (pos: number) => {
      if (!editor) return
      const node = editor.state.doc.nodeAt(pos)
      if (!node) return
      const chain = editor.chain().focus()
      if (node.isTextblock) {
        // Place the caret just inside the text block.
        chain.setTextSelection(pos + 1)
      } else {
        chain.setNodeSelection(pos)
      }
      chain.scrollIntoView().run()
    },
    [editor],
  )

  const updateSelectedAttrs = useCallback(
    (attrs: Record<string, unknown>) => {
      if (!editor || state.pos == null) return
      const pos = state.pos
      editor
        .chain()
        .focus()
        .command(({ tr }) => {
          const node = tr.doc.nodeAt(pos)
          if (!node) return false
          tr.setNodeMarkup(pos, undefined, { ...node.attrs, ...attrs })
          return true
        })
        .run()
    },
    [editor, state.pos],
  )

  const clear = useCallback(() => {
    setState({ pos: null, node: null })
    editor?.commands.blur()
  }, [editor])

  return {
    selectedPos: state.pos,
    selectedNode: state.node,
    selectBlock,
    updateSelectedAttrs,
    clear,
  }
}
