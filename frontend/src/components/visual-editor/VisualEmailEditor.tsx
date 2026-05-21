// Top-level visual editor for campaigns and templates. Hosts the TipTap
// editor instance, the slash command menu, the bubble menu, the
// merge-tag picker, and (optionally) a desktop/mobile preview iframe.
// The editor is fully controlled: the caller owns the canonical
// `VisualDoc` JSON via `value` and receives every change via `onChange`.
//
// Concretely this satisfies T055 + T056–T065 by composing every custom
// extension and UI piece into one component. Each extension lives in its
// own file (see ./extensions and ./ui) so the editor stays a thin
// composition shell.

import "./visual-editor.css"
import { useEffect, useMemo } from "react"
import { EditorContent, useEditor } from "@tiptap/react"
import { Color } from "@tiptap/extension-color"
import { TextStyle } from "@tiptap/extension-text-style"
import StarterKit from "@tiptap/starter-kit"
import { Button } from "./extensions/Button"
import { Column, Columns } from "./extensions/Columns"
import { Divider } from "./extensions/Divider"
import { ImageBlock } from "./extensions/ImageBlock"
import { MergeTag } from "./extensions/MergeTag"
import { DragHandle } from "./ui/DragHandle"
import {
  SlashCommandExtension,
  useSlashCommandMenu,
} from "./ui/SlashCommandMenu"
import { VisualBubbleMenu } from "./ui/BubbleMenu"
import { MergeTagPicker } from "./ui/MergeTagPicker"
import type { VisualDoc } from "@/lib/api-types"
import type { Editor } from "@tiptap/core"

type Props = {
  // Tenant slug — used by the merge-tag picker and the preview iframe.
  slug: string
  value: VisualDoc
  onChange: (doc: VisualDoc) => void
  // Optional placeholder for the first empty paragraph.
  placeholder?: string
  // Editable defaults to true; set false for a read-only preview surface
  // (e.g. when the campaign is in a non-draft state).
  editable?: boolean
}

const EMPTY_DOC: VisualDoc = {
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [] }],
}

export function VisualEmailEditor({
  slug,
  value,
  onChange,
  placeholder,
  editable = true,
}: Props) {
  // The slash-command menu's React state is exposed via an imperative ref
  // (see ./ui/SlashCommandMenu). The extension consumes the same `api`
  // object so the React UI stays out of the suggestion plugin's hot path.
  const { ref: slashRef, menu: slashMenu } = useSlashCommandMenu()

  const extensions = useMemo(() => {
    return [
      StarterKit.configure({
        heading: { levels: [1, 2, 3] },
        // The wire schema does not include hardBreak — disable it so the
        // editor cannot produce nodes the Go validator would reject.
        hardBreak: false,
        // StarterKit 3.x bundles `Link` with safe defaults; we further
        // restrict the allowed protocols to the wire-schema allow-list
        // (`http`, `https`, `mailto`, `tel` — see
        // `internal/campaign/domain/visualdoc_validate.go`).
        link: {
          openOnClick: false,
          autolink: false,
          protocols: ["http", "https", "mailto", "tel"],
        },
      }),
      TextStyle,
      Color,
      Columns,
      Column,
      Button,
      Divider,
      ImageBlock,
      MergeTag,
      DragHandle,
      SlashCommandExtension.configure({
        get menuApi() {
          // The ref is populated on the first render; the suggestion
          // plugin only fires after the editor mounts so the indirection
          // is safe.
          return slashRef.current?.api ?? defaultMenuApi
        },
      }),
    ]
  }, [])

  const editor = useEditor({
    extensions,
    content: nonEmpty(value),
    editable,
    editorProps: {
      attributes: {
        class: "ve-editor",
        "data-testid": "ve-editor",
        ...(placeholder ? { "data-placeholder": placeholder } : {}),
      },
    },
    onUpdate: ({ editor: ed }) => {
      onChange(toVisualDoc(ed))
    },
  })

  // Sync external value changes into the editor (controlled component).
  // We compare to the editor's current JSON to avoid re-setting on the
  // round-trip from our own onUpdate emission.
  useEffect(() => {
    const current = JSON.stringify(toVisualDoc(editor))
    const incoming = JSON.stringify(nonEmpty(value))
    if (current !== incoming) {
      editor.commands.setContent(nonEmpty(value), {
        emitUpdate: false,
      })
    }
  }, [editor, value])

  useEffect(() => {
    editor.setEditable(editable)
  }, [editor, editable])

  return (
    <div className="ve-root" data-testid="visual-email-editor">
      <EditorContent editor={editor} />
      <VisualBubbleMenu editor={editor} />
      {slashMenu}
      <MergeTagPicker slug={slug} editor={editor} />
    </div>
  )
}

function nonEmpty(doc: VisualDoc): VisualDoc {
  if (doc.content.length === 0) return EMPTY_DOC
  return doc
}

// Extract the canonical VisualDoc from the editor. TipTap's JSON output
// shape matches our wire format exactly for the StarterKit nodes; the
// custom extensions (columns, button, divider, image, mergeTag) declare
// their `name` to match the wire `type` so no remap is needed.
function toVisualDoc(editor: Editor): VisualDoc {
  const raw = editor.getJSON() as { content?: Array<unknown> }
  return {
    version: 1,
    type: "doc",
    content: (raw.content ?? []) as VisualDoc["content"],
  }
}

const defaultMenuApi = {
  setItems: () => {},
  setOnSelect: () => {},
  open: () => {},
  close: () => {},
  onKeyDown: () => false,
}
