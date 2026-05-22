// BubbleMenu — appears above the current text selection with formatting
// controls (bold, italic, link, color), heading-level controls when the
// selection is inside a heading, and an "insert merge tag" button that
// triggers the MergeTagPicker via the same DOM event the slash menu uses.
//
// Implemented on top of `@tiptap/extension-bubble-menu`'s React component;
// the menu DOM is rendered inside the editor host container.

import { useState } from "react"
import { useTranslation } from "react-i18next"
import { BubbleMenu as TipTapBubbleMenu } from "@tiptap/react/menus"
import type { Editor } from "@tiptap/core"

type Props = { editor: Editor | null }

export function VisualBubbleMenu({ editor }: Props) {
  const { t } = useTranslation("visualEditor")
  const [linkOpen, setLinkOpen] = useState(false)
  const [linkHref, setLinkHref] = useState("")

  if (!editor) return null

  const isHeading = editor.isActive("heading")

  function openMergeTagPicker() {
    if (!editor) return
    editor.view.dom.dispatchEvent(
      new CustomEvent("visual-editor:merge-tag-open", { bubbles: true }),
    )
  }

  function applyLink() {
    if (!editor) return
    const href = linkHref.trim()
    if (!href) {
      editor.chain().focus().unsetLink().run()
    } else {
      editor.chain().focus().extendMarkRange("link").setLink({ href }).run()
    }
    setLinkOpen(false)
    setLinkHref("")
  }

  return (
    <TipTapBubbleMenu
      editor={editor}
      pluginKey="visual-editor-bubble-menu"
    >
      <div
        data-testid="ve-bubble-menu"
        className="ve-bubble-menu"
        style={{
          display: "inline-flex",
          gap: 4,
          background: "white",
          border: "1px solid #e5e7eb",
          borderRadius: 6,
          padding: 4,
          boxShadow: "0 2px 8px rgba(0,0,0,0.08)",
        }}
      >
        <BMButton
          testId="ve-bm-bold"
          active={editor.isActive("bold")}
          onClick={() => editor.chain().focus().toggleBold().run()}
        >
          B
        </BMButton>
        <BMButton
          testId="ve-bm-italic"
          active={editor.isActive("italic")}
          onClick={() => editor.chain().focus().toggleItalic().run()}
        >
          I
        </BMButton>
        <BMButton
          testId="ve-bm-strike"
          active={editor.isActive("strike")}
          onClick={() => editor.chain().focus().toggleStrike().run()}
        >
          S
        </BMButton>
        <BMButton
          testId="ve-bm-link"
          active={editor.isActive("link")}
          onClick={() => {
            const current = editor.getAttributes("link").href ?? ""
            setLinkHref(current)
            setLinkOpen((v) => !v)
          }}
        >
          🔗
        </BMButton>
        <BMButton
          testId="ve-bm-color"
          onClick={() => {
            const next = window.prompt(t("bubbleMenu.colorPrompt"))
            if (next == null) return
            const trimmed = next.trim()
            if (!trimmed) {
              editor.chain().focus().unsetColor().run()
            } else {
              editor.chain().focus().setColor(trimmed).run()
            }
          }}
        >
          A
        </BMButton>
        {isHeading ? (
          <>
            <BMButton
              testId="ve-bm-h1"
              active={editor.isActive("heading", { level: 1 })}
              onClick={() =>
                editor.chain().focus().setHeading({ level: 1 }).run()
              }
            >
              H1
            </BMButton>
            <BMButton
              testId="ve-bm-h2"
              active={editor.isActive("heading", { level: 2 })}
              onClick={() =>
                editor.chain().focus().setHeading({ level: 2 }).run()
              }
            >
              H2
            </BMButton>
            <BMButton
              testId="ve-bm-h3"
              active={editor.isActive("heading", { level: 3 })}
              onClick={() =>
                editor.chain().focus().setHeading({ level: 3 }).run()
              }
            >
              H3
            </BMButton>
          </>
        ) : null}
        <BMButton
          testId="ve-bm-merge-tag"
          onClick={openMergeTagPicker}
        >
          {"{ }"}
        </BMButton>
      </div>
      {linkOpen ? (
        <div
          data-testid="ve-bm-link-form"
          style={{
            marginTop: 4,
            display: "flex",
            gap: 4,
            background: "white",
            border: "1px solid #e5e7eb",
            borderRadius: 6,
            padding: 4,
          }}
        >
          <input
            autoFocus
            type="url"
            value={linkHref}
            placeholder={t("bubbleMenu.linkPlaceholder")}
            onChange={(e) => setLinkHref(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault()
                applyLink()
              }
            }}
            style={{
              padding: "4px 6px",
              border: "1px solid #e5e7eb",
              borderRadius: 4,
              minWidth: 180,
            }}
          />
          <button type="button" onClick={applyLink}>
            {t("bubbleMenu.apply")}
          </button>
        </div>
      ) : null}
    </TipTapBubbleMenu>
  )
}

function BMButton({
  children,
  active,
  onClick,
  testId,
}: {
  children: React.ReactNode
  active?: boolean
  onClick: () => void
  testId?: string
}) {
  return (
    <button
      type="button"
      data-testid={testId}
      onMouseDown={(e) => {
        e.preventDefault()
        onClick()
      }}
      style={{
        padding: "2px 6px",
        background: active ? "#dbeafe" : "transparent",
        border: 0,
        borderRadius: 4,
        cursor: "pointer",
      }}
    >
      {children}
    </button>
  )
}
