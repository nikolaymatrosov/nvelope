// StructureOutline — the left pane of the three-pane editor (feature 017, US2).
// It projects the structured document as an indented, navigable outline derived
// live from the editor's ProseMirror doc (FR-005). Clicking an entry selects
// the block and scrolls it into view (FR-007); the selected entry is
// highlighted (FR-008). Entries can be reordered by drag among same-parent
// siblings (FR-009), deleted/duplicated (FR-010), and container entries
// collapse to hide nested detail (FR-011).

import { useEffect, useReducer, useState } from "react"
import { useTranslation } from "react-i18next"
import type { Editor } from "@tiptap/core"
import type { Node as PMNode } from "@tiptap/pm/model"
import type { BlockSelection } from "../hooks/useBlockSelection"

type Props = {
  editor: Editor
  selection: BlockSelection
}

type OutlineEntry = {
  pos: number
  type: string
  level?: number
  count?: number
  excerpt: string
  children: Array<OutlineEntry>
}

const CONTAINER_TYPES = new Set([
  "columns",
  "column",
  "bulletList",
  "orderedList",
  "listItem",
  "blockquote",
])

// buildChildren walks a parent node's direct block children, returning one
// outline entry per child with its absolute document position.
function buildChildren(parent: PMNode, parentPos: number): Array<OutlineEntry> {
  const entries: Array<OutlineEntry> = []
  // Content of a node starts at parentPos + 1 (after its opening token).
  parent.forEach((child, offset) => {
    const pos = parentPos + 1 + offset
    entries.push(entryFor(child, pos))
  })
  return entries
}

function entryFor(node: PMNode, pos: number): OutlineEntry {
  const type = node.type.name
  return {
    pos,
    type,
    level: type === "heading" ? (node.attrs.level as number) : undefined,
    count: type === "columns" ? (node.attrs.count as number) : undefined,
    excerpt: node.isTextblock ? node.textContent.slice(0, 40) : labelExtra(node),
    children: CONTAINER_TYPES.has(type) ? buildChildren(node, pos) : [],
  }
}

// labelExtra derives a short content hint for non-text blocks.
function labelExtra(node: PMNode): string {
  switch (node.type.name) {
    case "button":
      return String(node.attrs.label ?? "")
    case "image":
      return String(node.attrs.alt ?? "")
    default:
      return ""
  }
}

function buildOutline(editor: Editor): Array<OutlineEntry> {
  // Top-level children live directly in the doc root, whose content starts at
  // position 0 (no opening token to skip, unlike a nested node).
  const entries: Array<OutlineEntry> = []
  editor.state.doc.forEach((node, offset) => {
    entries.push(entryFor(node, offset))
  })
  return entries
}

export function StructureOutline({ editor, selection }: Props) {
  const { t } = useTranslation("visualEditor")
  const [, bump] = useReducer((n: number) => n + 1, 0)
  const [collapsed, setCollapsed] = useState<Set<number>>(new Set())

  // Re-derive the outline whenever the document changes.
  useEffect(() => {
    const onUpdate = () => bump()
    editor.on("update", onUpdate)
    return () => {
      editor.off("update", onUpdate)
    }
  }, [editor])

  const entries = buildOutline(editor)

  return (
    <div className="ve-panel ve-panel--structure" data-testid="ve-structure-panel">
      <div className="ve-panel__title">{t("panel.structureTitle")}</div>
      {entries.length === 0 ? (
        <p className="ve-panel__empty">{t("structure.empty")}</p>
      ) : (
        <ul className="ve-outline" role="tree" data-testid="ve-outline">
          {entries.map((entry) => (
            <OutlineNode
              key={entry.pos}
              entry={entry}
              depth={0}
              editor={editor}
              selection={selection}
              collapsed={collapsed}
              setCollapsed={setCollapsed}
              t={t}
            />
          ))}
        </ul>
      )}
    </div>
  )
}

function labelFor(entry: OutlineEntry, t: (k: string, opts?: Record<string, unknown>) => string): string {
  switch (entry.type) {
    case "heading":
      return t("structure.block.heading", { level: entry.level ?? 1 })
    case "columns":
      return t("structure.block.columns", { count: entry.count ?? 2 })
    case "paragraph":
    case "bulletList":
    case "orderedList":
    case "listItem":
    case "blockquote":
    case "codeBlock":
    case "image":
    case "button":
    case "divider":
    case "column":
    case "rawHtml":
      return t(`structure.block.${entry.type}`)
    default:
      return t("structure.block.unknown")
  }
}

function OutlineNode({
  entry,
  depth,
  editor,
  selection,
  collapsed,
  setCollapsed,
  t,
}: {
  entry: OutlineEntry
  depth: number
  editor: Editor
  selection: BlockSelection
  collapsed: Set<number>
  setCollapsed: React.Dispatch<React.SetStateAction<Set<number>>>
  t: (k: string, opts?: Record<string, unknown>) => string
}) {
  const isContainer = entry.children.length > 0
  const isCollapsed = collapsed.has(entry.pos)
  const isSelected = selection.selectedPos === entry.pos

  const toggle = () =>
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(entry.pos)) next.delete(entry.pos)
      else next.add(entry.pos)
      return next
    })

  return (
    <li className="ve-outline__item" role="treeitem" aria-selected={isSelected}>
      <div
        className={`ve-outline__row${isSelected ? " is-selected" : ""}`}
        style={{ paddingLeft: `${depth * 12 + 4}px` }}
        data-testid={`ve-outline-row-${entry.type}`}
        data-pos={entry.pos}
        draggable
        onDragStart={(e) => {
          e.dataTransfer.setData("text/ve-pos", String(entry.pos))
          e.dataTransfer.effectAllowed = "move"
        }}
        onDragOver={(e) => e.preventDefault()}
        onDrop={(e) => {
          e.preventDefault()
          const raw = e.dataTransfer.getData("text/ve-pos")
          if (!raw) return
          moveBlock(editor, Number(raw), entry.pos)
        }}
      >
        {isContainer && (
          <button
            type="button"
            className="ve-outline__toggle"
            aria-label={isCollapsed ? t("structure.actions.expand") : t("structure.actions.collapse")}
            aria-expanded={!isCollapsed}
            data-testid={`ve-outline-toggle-${entry.pos}`}
            onClick={toggle}
          >
            {isCollapsed ? "▸" : "▾"}
          </button>
        )}
        <button
          type="button"
          className="ve-outline__label"
          data-testid={`ve-outline-select-${entry.pos}`}
          onClick={() => selection.selectBlock(entry.pos)}
        >
          <span className="ve-outline__type">{labelFor(entry, t)}</span>
          {entry.excerpt && <span className="ve-outline__excerpt">{entry.excerpt}</span>}
        </button>
        <span className="ve-outline__actions">
          <button
            type="button"
            aria-label={t("structure.actions.duplicate")}
            title={t("structure.actions.duplicate")}
            data-testid={`ve-outline-duplicate-${entry.pos}`}
            onClick={() => duplicateBlock(editor, entry.pos)}
          >
            ⧉
          </button>
          <button
            type="button"
            aria-label={t("structure.actions.delete")}
            title={t("structure.actions.delete")}
            data-testid={`ve-outline-delete-${entry.pos}`}
            onClick={() => deleteBlock(editor, entry.pos)}
          >
            ✕
          </button>
        </span>
      </div>
      {isContainer && !isCollapsed && (
        <ul className="ve-outline" role="group">
          {entry.children.map((child) => (
            <OutlineNode
              key={child.pos}
              entry={child}
              depth={depth + 1}
              editor={editor}
              selection={selection}
              collapsed={collapsed}
              setCollapsed={setCollapsed}
              t={t}
            />
          ))}
        </ul>
      )}
    </li>
  )
}

// deleteBlock removes the block at pos.
function deleteBlock(editor: Editor, pos: number): void {
  const node = editor.state.doc.nodeAt(pos)
  if (!node) return
  editor
    .chain()
    .focus()
    .command(({ tr }) => {
      tr.delete(pos, pos + node.nodeSize)
      return true
    })
    .run()
}

// duplicateBlock inserts a copy of the block immediately after it.
function duplicateBlock(editor: Editor, pos: number): void {
  const node = editor.state.doc.nodeAt(pos)
  if (!node) return
  editor
    .chain()
    .focus()
    .command(({ tr }) => {
      tr.insert(pos + node.nodeSize, node.copy(node.content))
      return true
    })
    .run()
}

// moveBlock reorders srcPos relative to targetPos. To keep the move
// schema-valid it only reorders siblings that share the same parent; a drop
// onto a node in a different parent (or into the dragged node's own subtree) is
// rejected, leaving the document unchanged (FR-009 / edge case O5).
export function moveBlock(editor: Editor, srcPos: number, targetPos: number): boolean {
  if (srcPos === targetPos) return false
  const doc = editor.state.doc
  const node = doc.nodeAt(srcPos)
  const target = doc.nodeAt(targetPos)
  if (!node || !target) return false
  // Reject dropping a node into its own subtree.
  if (targetPos > srcPos && targetPos < srcPos + node.nodeSize) return false
  // Only reorder within the same parent (schema-safe).
  if (doc.resolve(srcPos).start() !== doc.resolve(targetPos).start()) return false

  const insertBefore = srcPos > targetPos
  const insertPos = insertBefore ? targetPos : targetPos + target.nodeSize
  editor
    .chain()
    .focus()
    .command(({ tr }) => {
      tr.delete(srcPos, srcPos + node.nodeSize)
      const mapped = tr.mapping.map(insertPos)
      tr.insert(mapped, node)
      return true
    })
    .run()
  return true
}
