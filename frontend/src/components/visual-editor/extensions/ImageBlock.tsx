// Image block — atom node, serializes to
// `{ type: "image", attrs: { mediaRef, alt, href } }`. mediaRef is the
// canonical tenant media URL (per FR-021); the BFF renderer emits the
// final `<img>` against the same URL.
//
// In-editor presentation: a NodeView that exposes two affordances —
//   1. A "Pick from media library" button when `mediaRef` is empty (the
//      slash-command inserts an image with `mediaRef: ""`, and the
//      MediaPicker resolves it).
//   2. A "Replace from media library" button on a filled image so the
//      operator can swap the asset.
// Both dispatch a `IMAGEBLOCK_PICK_EVENT` CustomEvent on the editor's root
// DOM, mirroring the RawHTML pattern — the parent VisualEmailEditor hosts
// the React MediaPicker so the extension stays React-free.
//
// Missing-asset placeholder (T098 / FR-021): if the loaded `<img>` fires an
// `error` event — typically because the referenced media asset was deleted
// from the library — the NodeView swaps the broken-image DOM for a labelled
// placeholder so the operator immediately knows the asset is gone.

import { Node, mergeAttributes } from "@tiptap/core"
import { isPendingMediaRef } from "../plugins/imageUpload"
import { blockStyleAttributeSpec } from "./styleAttr"

export const IMAGEBLOCK_PICK_EVENT = "ve-imageblock-pick-request"

// Event payload — the parent listens on the editor's root DOM and opens
// the MediaPicker modal. On pick it calls `applyImageBlockPick(editor, pos,
// mediaRef, alt)` to update the node attrs without reparsing the doc.
export type ImageBlockPickRequest = {
  pos: number
  mediaRef: string
  alt: string
}

export const ImageBlock = Node.create({
  name: "image",
  group: "block",
  atom: true,
  draggable: true,
  selectable: true,
  addAttributes() {
    return {
      mediaRef: { default: "" },
      alt: { default: "" },
      href: { default: "" },
      style: blockStyleAttributeSpec,
    }
  },
  parseHTML() {
    return [{ tag: "img[data-type=\"image\"]" }]
  },
  renderHTML({ HTMLAttributes, node }) {
    return [
      "img",
      mergeAttributes(HTMLAttributes, {
        "data-type": "image",
        class: "ve-image",
        src: node.attrs.mediaRef || "",
        alt: node.attrs.alt || "",
      }),
    ]
  },
  addNodeView() {
    return ({ node, getPos, editor }) => {
      const wrapper = document.createElement("div")
      wrapper.className = "ve-image-block"
      wrapper.setAttribute("data-type", "image")
      wrapper.setAttribute("contenteditable", "false")

      const dispatchPick = () => {
        const pos = typeof getPos === "function" ? getPos() : undefined
        if (pos === undefined || pos < 0) return
        const detail: ImageBlockPickRequest = {
          pos,
          mediaRef: String(node.attrs.mediaRef ?? ""),
          alt: String(node.attrs.alt ?? ""),
        }
        editor.view.dom.dispatchEvent(
          new CustomEvent<ImageBlockPickRequest>(IMAGEBLOCK_PICK_EVENT, {
            detail,
            bubbles: true,
          }),
        )
      }

      const render = (mediaRef: string, alt: string) => {
        wrapper.replaceChildren()
        if (mediaRef === "") {
          renderEmptyPicker(wrapper, dispatchPick)
          return
        }
        if (isPendingMediaRef(mediaRef)) {
          renderPending(wrapper, alt)
          return
        }
        renderImage(wrapper, mediaRef, alt, dispatchPick)
      }

      render(String(node.attrs.mediaRef ?? ""), String(node.attrs.alt ?? ""))

      return {
        dom: wrapper,
        update: (updated) => {
          if (updated.type.name !== "image") return false
          render(
            String(updated.attrs.mediaRef ?? ""),
            String(updated.attrs.alt ?? ""),
          )
          return true
        },
      }
    }
  },
})

function renderEmptyPicker(
  wrapper: HTMLDivElement,
  onClick: () => void,
): void {
  const btn = document.createElement("button")
  btn.type = "button"
  btn.className = "ve-image-block__pick"
  btn.setAttribute("data-testid", "ve-image-pick")
  btn.textContent = "Pick from media library"
  btn.addEventListener("click", (e) => {
    e.preventDefault()
    e.stopPropagation()
    onClick()
  })
  wrapper.appendChild(btn)
}

function renderPending(wrapper: HTMLDivElement, alt: string): void {
  const note = document.createElement("div")
  note.className = "ve-image-block__pending"
  note.setAttribute("data-testid", "ve-image-pending")
  note.textContent = alt ? `Uploading ${alt}…` : "Uploading…"
  wrapper.appendChild(note)
}

function renderImage(
  wrapper: HTMLDivElement,
  mediaRef: string,
  alt: string,
  onReplaceClick: () => void,
): void {
  const img = document.createElement("img")
  img.className = "ve-image"
  img.setAttribute("data-type", "image")
  img.setAttribute("data-testid", "ve-image-img")
  img.src = mediaRef
  img.alt = alt
  img.addEventListener("error", () => {
    renderMissing(wrapper, alt, onReplaceClick)
  })
  wrapper.appendChild(img)

  const replace = document.createElement("button")
  replace.type = "button"
  replace.className = "ve-image-block__replace"
  replace.setAttribute("data-testid", "ve-image-replace")
  replace.textContent = "Replace"
  replace.addEventListener("click", (e) => {
    e.preventDefault()
    e.stopPropagation()
    onReplaceClick()
  })
  wrapper.appendChild(replace)
}

function renderMissing(
  wrapper: HTMLDivElement,
  alt: string,
  onPickClick: () => void,
): void {
  wrapper.replaceChildren()
  const placeholder = document.createElement("div")
  placeholder.className = "ve-image-block__missing"
  placeholder.setAttribute("data-testid", "ve-image-missing")

  const label = document.createElement("div")
  label.className = "ve-image-block__missing-label"
  label.textContent = alt
    ? `Image "${alt}" is no longer available`
    : "Image is no longer available"

  const hint = document.createElement("div")
  hint.className = "ve-image-block__missing-hint"
  hint.textContent = "Pick a replacement from the media library."

  const btn = document.createElement("button")
  btn.type = "button"
  btn.className = "ve-image-block__pick"
  btn.setAttribute("data-testid", "ve-image-pick")
  btn.textContent = "Pick from media library"
  btn.addEventListener("click", (e) => {
    e.preventDefault()
    e.stopPropagation()
    onPickClick()
  })

  placeholder.appendChild(label)
  placeholder.appendChild(hint)
  placeholder.appendChild(btn)
  wrapper.appendChild(placeholder)
}

// applyImageBlockPick replaces the mediaRef + alt on the image node at the
// supplied position. Mirrors `applyRawHTMLEdit` so the parent component can
// update attrs in-place after the MediaPicker resolves a chosen asset
// without re-emitting the entire doc shape.
export function applyImageBlockPick(
  editor: { commands: { command: (fn: (props: { tr: unknown; dispatch?: (tr: unknown) => void }) => boolean) => boolean } },
  pos: number,
  mediaRef: string,
  alt: string,
): boolean {
  return editor.commands.command(({ tr, dispatch }) => {
    if (!dispatch) return true
    const transaction = tr as {
      doc: { nodeAt: (pos: number) => { type: { name: string }; attrs: Record<string, unknown> } | null }
      setNodeMarkup: (pos: number, type: null, attrs: Record<string, unknown>) => unknown
    }
    const node = transaction.doc.nodeAt(pos)
    if (!node || node.type.name !== "image") return false
    transaction.setNodeMarkup(pos, null, { ...node.attrs, mediaRef, alt })
    return true
  })
}
