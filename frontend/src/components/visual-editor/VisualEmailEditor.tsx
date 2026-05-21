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
import { useCallback, useEffect, useMemo, useState } from "react"
import { Color } from "@tiptap/extension-color"
import { TextStyle } from "@tiptap/extension-text-style"
import { EditorContent, useEditor } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"
import { Button } from "./extensions/Button"
import { Column, Columns } from "./extensions/Columns"
import { Divider } from "./extensions/Divider"
import { ImageBlock } from "./extensions/ImageBlock"
import { MergeTag } from "./extensions/MergeTag"
import {
  RAWHTML_EDIT_EVENT,
  RawHTML,
  applyRawHTMLEdit,
} from "./extensions/RawHTML"
import { VisualBubbleMenu } from "./ui/BubbleMenu"
import { DragHandle } from "./ui/DragHandle"
import { MergeTagPicker } from "./ui/MergeTagPicker"
import {
  SlashCommandExtension,
  useSlashCommandMenu,
} from "./ui/SlashCommandMenu"
import type { RawHTMLEditRequest } from "./extensions/RawHTML"
import type { Editor } from "@tiptap/core"
import type { VisualDoc } from "@/lib/api-types"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { CodeView } from "@/components/code-editor/CodeView"
import { Button as UIButton } from "@/components/ui/button"

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
  // Optional code-view toggle. When the operator presses the "View HTML"
  // toolbar button, the editor calls this callback with the latest
  // rendered HTML (if any) — the route owns the actual switch to code
  // view because it also owns the save path.
  onSwitchToCodeView?: () => void
  // Optional "Edit as HTML only" opt-out affordance. The toolbar renders
  // the button when supplied; the route owns the confirmation modal and
  // the API call (clears body_doc per T093 / FR-029).
  onOptOutVisual?: () => void
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
  onSwitchToCodeView,
  onOptOutVisual,
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
      RawHTML,
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

  // RawHTML edit-modal state. The RawHTML node view dispatches a
  // CustomEvent on the editor's root DOM when the operator presses
  // "Edit HTML"; we open a CodeMirror-backed modal seeded with the
  // current block's html and write it back via applyRawHTMLEdit on save.
  const [rawHTMLEdit, setRawHTMLEdit] = useState<RawHTMLEditRequest | null>(
    null,
  )
  const [rawHTMLDraft, setRawHTMLDraft] = useState<string>("")

  useEffect(() => {
    const root = editor.view.dom
    const onEditRequest = (event: Event) => {
      const detail = (event as CustomEvent<RawHTMLEditRequest>).detail
      setRawHTMLEdit(detail)
      setRawHTMLDraft(detail.html)
    }
    root.addEventListener(RAWHTML_EDIT_EVENT, onEditRequest)
    return () => {
      root.removeEventListener(RAWHTML_EDIT_EVENT, onEditRequest)
    }
  }, [editor])

  const closeRawHTMLModal = useCallback(() => {
    setRawHTMLEdit(null)
    setRawHTMLDraft("")
  }, [])

  const saveRawHTMLEdit = useCallback(() => {
    if (rawHTMLEdit === null) return
    applyRawHTMLEdit(editor, rawHTMLEdit.pos, rawHTMLDraft)
    closeRawHTMLModal()
  }, [editor, rawHTMLEdit, rawHTMLDraft, closeRawHTMLModal])

  const showToolbar = Boolean(onSwitchToCodeView || onOptOutVisual)

  return (
    <div className="ve-root" data-testid="visual-email-editor">
      {showToolbar && (
        <div className="ve-toolbar" data-testid="ve-toolbar">
          {onSwitchToCodeView && (
            <button
              type="button"
              className="ve-toolbar__btn"
              data-testid="ve-switch-to-code"
              onClick={onSwitchToCodeView}
            >
              View HTML
            </button>
          )}
          {onOptOutVisual && (
            <button
              type="button"
              className="ve-toolbar__btn"
              data-testid="ve-opt-out-visual"
              onClick={onOptOutVisual}
            >
              Edit as HTML only
            </button>
          )}
        </div>
      )}
      <EditorContent editor={editor} />
      <VisualBubbleMenu editor={editor} />
      {slashMenu}
      <MergeTagPicker slug={slug} editor={editor} />
      <Dialog
        open={rawHTMLEdit !== null}
        onOpenChange={(open) => {
          if (!open) closeRawHTMLModal()
        }}
      >
        <DialogContent className="max-w-3xl" data-testid="ve-rawhtml-modal">
          <DialogHeader>
            <DialogTitle>Edit raw HTML block</DialogTitle>
            <DialogDescription>
              These bytes are passed through verbatim. They run through the
              sanitizer at save time — disallowed constructs (script,
              event handlers, dangerous URL schemes) are stripped.
            </DialogDescription>
          </DialogHeader>
          <CodeView
            value={rawHTMLDraft}
            onChange={setRawHTMLDraft}
            ariaLabel="Raw HTML editor"
            testId="ve-rawhtml-codeview"
          />
          <DialogFooter>
            <UIButton
              type="button"
              variant="outline"
              onClick={closeRawHTMLModal}
            >
              Cancel
            </UIButton>
            <UIButton
              type="button"
              onClick={saveRawHTMLEdit}
              data-testid="ve-rawhtml-modal-save"
            >
              Save HTML
            </UIButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>
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
