// RawHTML — atom block node carrying a verbatim HTML region. Used to host
// (a) pre-existing raw-HTML content during opt-in migration to the visual
// editor (FR-031), and (b) code-view edits the editor cannot round-trip
// into structured blocks (FR-027). The BFF renderer pipes the bytes
// through and Go's bluemonday sanitizer enforces the email-safe policy at
// save time.
//
// Wire shape: `{ type: "rawHtml", attrs: { html: string } }` — exactly what
// internal/campaign/domain/visualdoc.go's `RawHTML` block emits.
//
// In-editor presentation: a labeled container that renders the sanitized
// HTML preview alongside an "Edit HTML" button. Clicking the button
// dispatches a `ve-rawhtml-edit-request` CustomEvent on the editor's root
// DOM so the parent component (VisualEmailEditor) can host the modal +
// CodeMirror editor without the extension owning a React tree.

import DOMPurify from "isomorphic-dompurify"
import { Node, mergeAttributes } from "@tiptap/core"

export type RawHTMLAttrs = {
  html: string
}

// Event payload dispatched when the operator presses the "Edit HTML"
// button. The editor parent listens on the editor's root DOM and opens
// a modal seeded with `html`; on save it replaces the node's attrs via
// the supplied `replace` callback.
export type RawHTMLEditRequest = {
  html: string
  // ProseMirror absolute position of the RawHTML node — used by the
  // parent to update the node's attrs without re-parsing the doc.
  pos: number
}

export const RAWHTML_EDIT_EVENT = "ve-rawhtml-edit-request"

export const RawHTML = Node.create({
  name: "rawHtml",
  group: "block",
  atom: true,
  draggable: true,
  selectable: true,
  addAttributes() {
    return {
      html: { default: "" },
    }
  },
  parseHTML() {
    // The serializer below uses a wrapping <div data-type="raw-html">; if
    // a raw-HTML region is pasted into the editor it remains opaque
    // because non-matching markup falls through to other handlers.
    return [{ tag: "div[data-type=\"raw-html\"]" }]
  },
  renderHTML({ HTMLAttributes }) {
    // Outside the editor (e.g. in a clipboard copy), serialize the node
    // as a marker div carrying the html attribute. The BFF renderer does
    // not consume this output — it reads the JSON `attrs.html` directly.
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "raw-html",
        class: "ve-rawhtml",
      }),
      0,
    ]
  },
  addNodeView() {
    return ({ node, getPos, editor }) => {
      const wrapper = document.createElement("div")
      wrapper.className = "ve-rawhtml"
      wrapper.setAttribute("data-type", "raw-html")
      wrapper.setAttribute("contenteditable", "false")

      const label = document.createElement("div")
      label.className = "ve-rawhtml__label"
      label.textContent = "Raw HTML"

      const editBtn = document.createElement("button")
      editBtn.type = "button"
      editBtn.className = "ve-rawhtml__edit"
      editBtn.setAttribute("data-testid", "ve-rawhtml-edit")
      editBtn.textContent = "Edit HTML"

      const preview = document.createElement("div")
      preview.className = "ve-rawhtml__preview"
      preview.setAttribute("data-testid", "ve-rawhtml-preview")

      // Sanitize the preview so the editor cannot transiently render
      // disallowed markup. The Go-side bluemonday pass is the
      // authoritative sanitizer at save time — this is just the editor's
      // local guard against rendering `<script>` even momentarily.
      preview.innerHTML = sanitizeForPreview(node.attrs.html ?? "")

      const header = document.createElement("div")
      header.className = "ve-rawhtml__header"
      header.appendChild(label)
      header.appendChild(editBtn)
      wrapper.appendChild(header)
      wrapper.appendChild(preview)

      editBtn.addEventListener("click", (e) => {
        e.preventDefault()
        e.stopPropagation()
        const pos = typeof getPos === "function" ? getPos() : undefined
        if (pos === undefined || pos < 0) return
        const detail: RawHTMLEditRequest = {
          html: node.attrs.html ?? "",
          pos,
        }
        editor.view.dom.dispatchEvent(
          new CustomEvent<RawHTMLEditRequest>(RAWHTML_EDIT_EVENT, {
            detail,
            bubbles: true,
          })
        )
      })

      return {
        dom: wrapper,
        update: (updated) => {
          if (updated.type.name !== "rawHtml") return false
          preview.innerHTML = sanitizeForPreview(updated.attrs.html ?? "")
          return true
        },
      }
    }
  },
})

// sanitizeForPreview runs DOMPurify with a strict allow-list — strips
// every element and attribute the email policy bans regardless of where
// they appear. The save-time Go-side sanitizer is the authoritative gate;
// this is the editor's local preview-only guard.
function sanitizeForPreview(html: string): string {
  return DOMPurify.sanitize(html, {
    FORBID_TAGS: [
      "script",
      "style",
      "iframe",
      "object",
      "embed",
      "form",
      "input",
      "link",
    ],
    FORBID_ATTR: ["onerror", "onload", "onclick"],
    USE_PROFILES: { html: true },
  })
}

// applyRawHTMLEdit replaces the html on the RawHTML node at the supplied
// position. Exposed as a helper for the editor parent that owns the
// modal; the edit flow doesn't go through TipTap commands because we
// want to update attrs in-place without re-emitting the entire doc shape.
export function applyRawHTMLEdit(
  editor: { commands: { command: (fn: (props: { tr: unknown; dispatch?: (tr: unknown) => void }) => boolean) => boolean } },
  pos: number,
  html: string
): boolean {
  return editor.commands.command(({ tr, dispatch }) => {
    if (!dispatch) return true
    // dispatch is wired by TipTap; tr is a Transaction.
    const transaction = tr as {
      doc: { nodeAt: (pos: number) => { type: { name: string }; attrs: Record<string, unknown> } | null }
      setNodeMarkup: (pos: number, type: null, attrs: Record<string, unknown>) => unknown
    }
    const node = transaction.doc.nodeAt(pos)
    if (!node || node.type.name !== "rawHtml") return false
    transaction.setNodeMarkup(pos, null, { ...node.attrs, html })
    return true
  })
}
