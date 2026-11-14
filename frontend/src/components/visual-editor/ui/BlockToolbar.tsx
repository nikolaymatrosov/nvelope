// BlockToolbar — the floating label centered under the selected block: the
// block's name (same label as in Layers) plus Duplicate / Delete. It is
// absolutely positioned inside the white page from the block's DOM rect, so it
// scrolls with the canvas and never competes with the text bubble menu.

import { useLayoutEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { Copy, Trash2 } from "lucide-react"
import { deleteBlock, duplicateBlock, labelFor } from "../panels/StructureOutline"
import type { BlockSelection } from "../hooks/useBlockSelection"
import type { Editor } from "@tiptap/core"
import type { RefObject } from "react"

type Props = {
  editor: Editor
  selection: BlockSelection
  pageRef: RefObject<HTMLDivElement | null>
}

export function BlockToolbar({ editor, selection, pageRef }: Props) {
  const { t } = useTranslation("visualEditor")
  const { selectedPos, selectedNode } = selection
  const [coords, setCoords] = useState<{ top: number; left: number } | null>(null)

  useLayoutEffect(() => {
    const page = pageRef.current
    const dom = selectedPos == null ? null : editor.view.nodeDOM(selectedPos)
    if (!page || !(dom instanceof HTMLElement)) {
      setCoords(null)
      return
    }
    const measure = () => {
      const block = dom.getBoundingClientRect()
      const box = page.getBoundingClientRect()
      setCoords({ top: block.bottom - box.top + 8, left: block.left - box.left + block.width / 2 })
    }
    measure()
    // Re-measure when the page width changes (the device toggle animates it).
    if (typeof ResizeObserver === "undefined") return
    const ro = new ResizeObserver(measure)
    ro.observe(page)
    return () => ro.disconnect()
  }, [editor, selectedPos, selectedNode, pageRef])

  if (selectedPos == null || selectedNode == null || coords == null) return null

  const label = labelFor(
    {
      type: selectedNode.type.name,
      level: selectedNode.attrs.level as number | undefined,
      count: selectedNode.attrs.count as number | undefined,
    },
    t,
  )

  return (
    <div className="ve-block-toolbar" data-testid="ve-block-toolbar" style={{ top: coords.top, left: coords.left }}>
      <span className="ve-block-toolbar__label">{label}</span>
      <button
        type="button"
        aria-label={t("structure.actions.duplicate")}
        title={t("structure.actions.duplicate")}
        data-testid="ve-block-toolbar-duplicate"
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => duplicateBlock(editor, selectedPos)}
      >
        <Copy className="size-4" aria-hidden />
      </button>
      <button
        type="button"
        aria-label={t("structure.actions.delete")}
        title={t("structure.actions.delete")}
        data-testid="ve-block-toolbar-delete"
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => deleteBlock(editor, selectedPos)}
      >
        <Trash2 className="size-4" aria-hidden />
      </button>
    </div>
  )
}
