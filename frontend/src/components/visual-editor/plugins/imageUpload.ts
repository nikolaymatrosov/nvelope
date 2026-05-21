// imageUpload — ProseMirror plugin (wrapped as a TipTap Extension) that
// handles drag-and-drop + paste of image files into the visual editor. Each
// accepted file is uploaded through the tenant media library (Phase 6
// `api.media.upload`); on success the resulting tenant media URL is inserted
// as an ImageBlock node. Files that violate the content-type allowlist or
// the size cap are rejected inline via a sonner toast, before any node is
// created. An interrupted upload removes the placeholder so the doc stays
// valid (no orphan `pending:` mediaRef would pass save-time validation).
//
// Why a plugin instead of a slash-command: drop and paste must override the
// default ProseMirror behaviour (which would otherwise insert raw bytes /
// inline data URLs that violate FR-021's tenant-media-only constraint).

import { Extension } from "@tiptap/core"
import { Plugin, PluginKey } from "@tiptap/pm/state"
import { toast } from "sonner"
import type { ApiResult } from "@/lib/api"
import type { EditorView } from "@tiptap/pm/view"
import type { MediaUploadResult } from "@/lib/api-types"
import { ApiError, api } from "@/lib/api"
import {
  ALLOWED_MEDIA_CONTENT_TYPES,
  DEFAULT_MEDIA_MAX_BYTES,
} from "@/lib/api-types"

// Marker prefix used on a placeholder ImageBlock's mediaRef while the
// upload is in flight. The placeholder is replaced by setNodeMarkup once
// the upload resolves; on failure the node is removed. The save-time
// validator rejects this string explicitly so a partial upload can't be
// persisted.
const PENDING_PREFIX = "pending:"

export type ImageUploadOptions = {
  // Tenant slug — required to address api.media.upload.
  slug: string
  // Override hook for tests. The real plugin uses `api.media.upload`.
  upload?: (slug: string, file: File) => Promise<ApiResult<MediaUploadResult>>
  // Override hook for tests. The real plugin uses sonner's `toast.error`.
  notify?: (message: string) => void
  // Override the content-type allowlist. Defaults to the media-library
  // allowlist mirrored from the backend.
  allowedTypes?: ReadonlyArray<string>
  // Override the per-file size cap. Defaults to the media-library default.
  maxBytes?: number
}

export const imageUploadPluginKey = new PluginKey("visualEditorImageUpload")

export const ImageUpload = Extension.create<ImageUploadOptions>({
  name: "visualEditorImageUpload",
  addOptions() {
    return {
      slug: "",
      upload: undefined,
      notify: undefined,
      allowedTypes: undefined,
      maxBytes: undefined,
    }
  },
  addProseMirrorPlugins() {
    const { slug, upload, notify, allowedTypes, maxBytes } = this.options
    const handler = makeImageDropPasteHandler({
      slug,
      upload: upload ?? api.media.upload,
      notify: notify ?? ((m: string) => toast.error(m)),
      allowedTypes: allowedTypes ?? ALLOWED_MEDIA_CONTENT_TYPES,
      maxBytes: maxBytes ?? DEFAULT_MEDIA_MAX_BYTES,
    })
    return [
      new Plugin({
        key: imageUploadPluginKey,
        props: {
          handleDOMEvents: {
            drop: (view, event) => handler.handleDrop(view, event),
            paste: (view, event) => handler.handlePaste(view, event),
          },
        },
      }),
    ]
  },
})

// Internal: filters and dispatches an upload, returning true when the
// plugin has consumed the event (and ProseMirror should skip its default
// drop/paste handling). Exported for direct unit-testing without
// simulating real DOM events on an EditorView.
export type ImageDropPasteHandler = {
  handleDrop: (view: EditorView, event: DragEvent) => boolean
  handlePaste: (view: EditorView, event: ClipboardEvent) => boolean
  // Test-only — start an upload for `file` at the cursor (no event).
  uploadFile: (view: EditorView, file: File) => Promise<void>
}

export function makeImageDropPasteHandler(opts: {
  slug: string
  upload: (slug: string, file: File) => Promise<ApiResult<MediaUploadResult>>
  notify: (message: string) => void
  allowedTypes: ReadonlyArray<string>
  maxBytes: number
}): ImageDropPasteHandler {
  let pendingCounter = 0

  function nextPendingRef(): string {
    pendingCounter += 1
    return `${PENDING_PREFIX}${Date.now()}-${pendingCounter}`
  }

  function imageFilesFrom(list: FileList | null | undefined): Array<File> {
    if (!list || list.length === 0) return []
    const out: Array<File> = []
    for (let i = 0; i < list.length; i++) {
      const file = list.item(i)
      if (file && file.type.startsWith("image/")) out.push(file)
    }
    return out
  }

  function validate(file: File): string | null {
    if (!opts.allowedTypes.includes(file.type)) {
      return `Unsupported image type ${file.type}`
    }
    if (file.size > opts.maxBytes) {
      const mb = (opts.maxBytes / (1024 * 1024)).toFixed(0)
      return `Image exceeds the ${mb} MB limit`
    }
    return null
  }

  function insertPlaceholder(
    view: EditorView,
    file: File,
    pos: number,
  ): string {
    const pendingRef = nextPendingRef()
    const tr = view.state.tr.insert(
      pos,
      view.state.schema.nodes.image.create({
        mediaRef: pendingRef,
        alt: file.name,
        href: "",
      }),
    )
    view.dispatch(tr)
    return pendingRef
  }

  function findPlaceholderPos(
    view: EditorView,
    pendingRef: string,
  ): number | null {
    let found: number | null = null
    view.state.doc.descendants((node, pos) => {
      if (found !== null) return false
      if (node.type.name === "image" && node.attrs.mediaRef === pendingRef) {
        found = pos
        return false
      }
      return true
    })
    return found
  }

  function replacePlaceholder(
    view: EditorView,
    pendingRef: string,
    mediaRef: string,
  ): boolean {
    const pos = findPlaceholderPos(view, pendingRef)
    if (pos === null) return false
    const node = view.state.doc.nodeAt(pos)
    if (!node) return false
    view.dispatch(
      view.state.tr.setNodeMarkup(pos, null, {
        ...node.attrs,
        mediaRef,
      }),
    )
    return true
  }

  function deletePlaceholder(view: EditorView, pendingRef: string): boolean {
    const pos = findPlaceholderPos(view, pendingRef)
    if (pos === null) return false
    const node = view.state.doc.nodeAt(pos)
    if (!node) return false
    view.dispatch(view.state.tr.delete(pos, pos + node.nodeSize))
    return true
  }

  async function startUpload(
    view: EditorView,
    file: File,
    pos: number,
  ): Promise<void> {
    const err = validate(file)
    if (err !== null) {
      opts.notify(err)
      return
    }
    const pendingRef = insertPlaceholder(view, file, pos)
    try {
      const res = await opts.upload(opts.slug, file)
      if (!replacePlaceholder(view, pendingRef, res.data.public_url)) {
        // The editor was unmounted or the node deleted before the upload
        // resolved — nothing to do.
        return
      }
    } catch (cause) {
      deletePlaceholder(view, pendingRef)
      const message =
        cause instanceof ApiError
          ? `Upload failed: ${cause.message}`
          : "Upload failed"
      opts.notify(message)
    }
  }

  function handleDrop(view: EditorView, event: DragEvent): boolean {
    const files = imageFilesFrom(event.dataTransfer?.files)
    if (files.length === 0) return false
    event.preventDefault()
    const coords = { left: event.clientX, top: event.clientY }
    const dropPos =
      view.posAtCoords(coords)?.pos ?? view.state.doc.content.size
    let cursor = dropPos
    for (const file of files) {
      // Fire-and-forget — each upload manages its own placeholder.
      void startUpload(view, file, cursor)
      cursor += 1
    }
    return true
  }

  function handlePaste(view: EditorView, event: ClipboardEvent): boolean {
    const files = imageFilesFrom(event.clipboardData?.files)
    if (files.length === 0) return false
    event.preventDefault()
    const at = view.state.selection.from
    let cursor = at
    for (const file of files) {
      void startUpload(view, file, cursor)
      cursor += 1
    }
    return true
  }

  return {
    handleDrop,
    handlePaste,
    uploadFile: (view, file) =>
      startUpload(view, file, view.state.selection.from),
  }
}

// isPendingMediaRef reports whether a mediaRef value is the transient
// placeholder used while an upload is in flight. The save-time validator
// uses this to refuse saving a doc that still carries pending uploads.
export function isPendingMediaRef(mediaRef: string): boolean {
  return mediaRef.startsWith(PENDING_PREFIX)
}
