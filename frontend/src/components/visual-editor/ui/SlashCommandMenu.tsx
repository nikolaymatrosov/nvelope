// SlashCommandMenu — type "/" anywhere in the editor and a popover opens
// listing the insertable blocks; filters as the operator types. Built on
// `@tiptap/suggestion` so the matching, range tracking, and ESC dismissal
// come from the same primitive TipTap's mentions use.

import {
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react"
import { Extension } from "@tiptap/core"
import Suggestion from "@tiptap/suggestion"
import { buildColumnsNode } from "../extensions/Columns"
import type { Editor, Range } from "@tiptap/core"

type SlashItem = {
  key: string
  label: string
  description?: string
  // Apply the item: deletes the slash range, then performs the actual
  // editor command. Each item is its own closure so the caller can mix
  // insert / setNode / wrap operations without a switch in the menu.
  apply: (editor: Editor, range: Range) => void
}

const SLASH_ITEMS: Array<SlashItem> = [
  {
    key: "paragraph",
    label: "Paragraph",
    apply: (editor, range) =>
      editor.chain().focus().deleteRange(range).setParagraph().run(),
  },
  {
    key: "heading-1",
    label: "Heading 1",
    apply: (editor, range) =>
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .setNode("heading", { level: 1 })
        .run(),
  },
  {
    key: "heading-2",
    label: "Heading 2",
    apply: (editor, range) =>
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .setNode("heading", { level: 2 })
        .run(),
  },
  {
    key: "heading-3",
    label: "Heading 3",
    apply: (editor, range) =>
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .setNode("heading", { level: 3 })
        .run(),
  },
  {
    key: "bullet-list",
    label: "Bulleted list",
    apply: (editor, range) =>
      editor.chain().focus().deleteRange(range).toggleBulletList().run(),
  },
  {
    key: "ordered-list",
    label: "Numbered list",
    apply: (editor, range) =>
      editor.chain().focus().deleteRange(range).toggleOrderedList().run(),
  },
  {
    key: "quote",
    label: "Quote",
    apply: (editor, range) =>
      editor.chain().focus().deleteRange(range).toggleBlockquote().run(),
  },
  {
    key: "code",
    label: "Code block",
    apply: (editor, range) =>
      editor.chain().focus().deleteRange(range).toggleCodeBlock().run(),
  },
  {
    key: "image",
    label: "Image",
    apply: (editor, range) =>
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent({
          type: "image",
          attrs: { mediaRef: "", alt: "", href: "" },
        })
        .run(),
  },
  {
    key: "button",
    label: "Button",
    apply: (editor, range) =>
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent({
          type: "button",
          attrs: { label: "Button", href: "" },
        })
        .run(),
  },
  {
    key: "divider",
    label: "Divider",
    apply: (editor, range) =>
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent({ type: "divider" })
        .run(),
  },
  {
    key: "columns-2",
    label: "Two columns",
    apply: (editor, range) =>
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent(buildColumnsNode(2))
        .run(),
  },
  {
    key: "columns-3",
    label: "Three columns",
    apply: (editor, range) =>
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent(buildColumnsNode(3))
        .run(),
  },
  {
    key: "columns-4",
    label: "Four columns",
    apply: (editor, range) =>
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent(buildColumnsNode(4))
        .run(),
  },
  {
    key: "merge-tag",
    label: "Merge tag",
    description: "Insert a subscriber or campaign placeholder",
    apply: (editor, range) => {
      editor.chain().focus().deleteRange(range).run()
      // The MergeTagPicker UI listens for this event and opens, then
      // dispatches an insert command into the editor. Decoupling here
      // keeps the slash menu free of merge-tag-specific UI state.
      editor.view.dom.dispatchEvent(
        new CustomEvent("visual-editor:merge-tag-open", { bubbles: true }),
      )
    },
  },
]

// React state container that exposes a ref API the suggestion plugin can
// drive directly (no React-effect re-renders per keystroke — the
// suggestion plugin updates the menu imperatively).
type MenuApi = {
  setItems: (items: Array<SlashItem>) => void
  setOnSelect: (fn: (item: SlashItem) => void) => void
  open: (rect: DOMRect | undefined) => void
  close: () => void
  onKeyDown: (e: { event: KeyboardEvent }) => boolean
}

export const SlashCommandExtension = Extension.create<{ menuApi: MenuApi }>({
  name: "slashCommand",
  addOptions() {
    return {
      menuApi: {
        setItems: () => {},
        setOnSelect: () => {},
        open: () => {},
        close: () => {},
        onKeyDown: () => false,
      },
    }
  },
  addProseMirrorPlugins() {
    const { menuApi } = this.options
    return [
      Suggestion({
        editor: this.editor,
        char: "/",
        startOfLine: false,
        command: ({ editor, range, props }) => {
          const item = props as SlashItem
          item.apply(editor, range)
        },
        items: ({ query }) =>
          SLASH_ITEMS.filter((i) =>
            i.label.toLowerCase().includes(query.toLowerCase()),
          ),
        render: () => ({
          onStart: (props) => {
            menuApi.setItems(props.items)
            menuApi.setOnSelect((item) => props.command(item))
            menuApi.open(props.clientRect?.() ?? undefined)
          },
          onUpdate: (props) => {
            menuApi.setItems(props.items)
            menuApi.setOnSelect((item) => props.command(item))
            menuApi.open(props.clientRect?.() ?? undefined)
          },
          onKeyDown: (props) => menuApi.onKeyDown(props),
          onExit: () => menuApi.close(),
        }),
      }),
    ]
  },
})

export type SlashCommandMenuHandle = {
  api: MenuApi
}

export function useSlashCommandMenu(): {
  ref: React.RefObject<SlashCommandMenuHandle | null>
  menu: React.ReactNode
} {
  const ref = useRef<SlashCommandMenuHandle | null>(null)
  const menu = <SlashCommandMenuRendered ref={ref} />
  return { ref, menu }
}

type RenderedProps = {
  ref: React.RefObject<SlashCommandMenuHandle | null>
}

function SlashCommandMenuRendered({ ref }: RenderedProps) {
  const [open, setOpen] = useState(false)
  const [items, setItems] = useState<Array<SlashItem>>([])
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null)
  const [selected, setSelected] = useState(0)
  const onSelectRef = useRef<(i: SlashItem) => void>(() => {})

  const api = useMemo<MenuApi>(
    () => ({
      setItems: (next) => {
        setItems(next)
        setSelected(0)
      },
      setOnSelect: (fn) => {
        onSelectRef.current = fn
      },
      open: (rect) => {
        if (rect) setPos({ top: rect.bottom, left: rect.left })
        setOpen(true)
      },
      close: () => setOpen(false),
      onKeyDown: ({ event }) => {
        if (event.key === "ArrowDown") {
          event.preventDefault()
          setSelected((s) => Math.min(items.length - 1, s + 1))
          return true
        }
        if (event.key === "ArrowUp") {
          event.preventDefault()
          setSelected((s) => Math.max(0, s - 1))
          return true
        }
        if (event.key === "Enter") {
          const item = items[selected] as SlashItem | undefined
          if (item) onSelectRef.current(item)
          return true
        }
        if (event.key === "Escape") {
          setOpen(false)
          return true
        }
        return false
      },
    }),
    [items, selected],
  )

  useImperativeHandle(ref, () => ({ api }), [api])

  const onClick = useCallback(
    (item: SlashItem) => {
      onSelectRef.current(item)
    },
    [],
  )

  useEffect(() => {
    if (!open) setSelected(0)
  }, [open])

  if (!open || items.length === 0) return null

  return (
    <div
      role="listbox"
      data-testid="ve-slash-menu"
      className="ve-slash-menu"
      style={{
        position: "fixed",
        top: pos?.top ?? 0,
        left: pos?.left ?? 0,
        zIndex: 50,
        background: "white",
        border: "1px solid #e5e7eb",
        borderRadius: 6,
        boxShadow: "0 4px 12px rgba(0,0,0,0.08)",
        maxHeight: 320,
        overflowY: "auto",
        minWidth: 200,
      }}
    >
      {items.map((item, idx) => (
        <button
          key={item.key}
          type="button"
          role="option"
          aria-selected={idx === selected}
          data-testid={`ve-slash-item-${item.key}`}
          onMouseDown={(e) => {
            e.preventDefault()
            onClick(item)
          }}
          style={{
            display: "block",
            width: "100%",
            textAlign: "left",
            padding: "6px 10px",
            background: idx === selected ? "#eff6ff" : "transparent",
            border: 0,
            cursor: "pointer",
          }}
        >
          <div style={{ fontSize: 14 }}>{item.label}</div>
          {item.description ? (
            <div style={{ fontSize: 12, color: "#6b7280" }}>
              {item.description}
            </div>
          ) : null}
        </button>
      ))}
    </div>
  )
}
