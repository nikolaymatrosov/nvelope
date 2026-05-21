// Unit tests for the imageUpload plugin (T099).
//
// The drag/paste DOM events themselves are tricky to simulate in jsdom
// because ProseMirror reads from DataTransfer / ClipboardData which jsdom
// implements only partially. We instead exercise the same code path via
// `handler.uploadFile(view, file)` — the entry the drop/paste handlers
// call into after extracting the File object. That keeps the test honest
// (it drives the real upload + placeholder + replace pipeline) without
// fighting jsdom's event surface.

import { afterEach, describe, expect, it, vi } from "vitest"
import { Editor } from "@tiptap/core"
import StarterKit from "@tiptap/starter-kit"
import { ImageBlock } from "../extensions/ImageBlock"
import {
  ImageUpload,
  isPendingMediaRef,
  makeImageDropPasteHandler,
} from "./imageUpload"
import { ALLOWED_MEDIA_CONTENT_TYPES } from "@/lib/api-types"
import { ApiError } from "@/lib/api"

function makeEditor(): Editor {
  const host = document.createElement("div")
  document.body.appendChild(host)
  return new Editor({
    element: host,
    extensions: [
      StarterKit.configure({ hardBreak: false }),
      ImageBlock,
      // The plugin extension itself is exercised via mounting; the
      // upload-handler tests below bypass the suggestion plugin and call
      // makeImageDropPasteHandler directly.
      ImageUpload.configure({ slug: "acme" }),
    ],
    content: { type: "doc", content: [{ type: "paragraph" }] },
  })
}

function findImageNode(editor: Editor): {
  mediaRef: string
  alt: string
} | null {
  let found: { mediaRef: string; alt: string } | null = null
  editor.state.doc.descendants((node) => {
    if (found !== null) return false
    if (node.type.name === "image") {
      found = {
        mediaRef: String(node.attrs.mediaRef ?? ""),
        alt: String(node.attrs.alt ?? ""),
      }
      return false
    }
    return true
  })
  return found
}

afterEach(() => {
  document.body.replaceChildren()
  vi.clearAllMocks()
})

describe("imageUpload plugin", () => {
  it("isPendingMediaRef recognizes transient placeholder refs", () => {
    expect(isPendingMediaRef("pending:123-1")).toBe(true)
    expect(isPendingMediaRef("https://media.example/x.png")).toBe(false)
    expect(isPendingMediaRef("")).toBe(false)
  })

  it("uploads a file and replaces the placeholder with the returned URL", async () => {
    const editor = makeEditor()
    const upload = vi.fn().mockResolvedValue({
      status: 200,
      ok: true,
      data: {
        id: "asset-1",
        public_url: "https://media.example/tenants/acme/cat.png",
        filename: "cat.png",
      },
    })
    const notify = vi.fn()
    const handler = makeImageDropPasteHandler({
      slug: "acme",
      upload,
      notify,
      allowedTypes: ALLOWED_MEDIA_CONTENT_TYPES,
      maxBytes: 10 * 1024 * 1024,
    })

    const file = new File([new Uint8Array([1, 2, 3])], "cat.png", {
      type: "image/png",
    })
    await handler.uploadFile(editor.view, file)

    expect(upload).toHaveBeenCalledWith("acme", file)
    expect(notify).not.toHaveBeenCalled()
    expect(findImageNode(editor)).toEqual({
      mediaRef: "https://media.example/tenants/acme/cat.png",
      alt: "cat.png",
    })
  })

  it("rejects oversize files up-front without inserting a node", async () => {
    const editor = makeEditor()
    const upload = vi.fn()
    const notify = vi.fn()
    const handler = makeImageDropPasteHandler({
      slug: "acme",
      upload,
      notify,
      allowedTypes: ALLOWED_MEDIA_CONTENT_TYPES,
      maxBytes: 5, // 5 bytes — anything realistic exceeds it
    })

    const file = new File([new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8])], "big.png", {
      type: "image/png",
    })
    await handler.uploadFile(editor.view, file)

    expect(upload).not.toHaveBeenCalled()
    expect(notify).toHaveBeenCalledWith(expect.stringMatching(/exceeds/i))
    expect(findImageNode(editor)).toBeNull()
  })

  it("rejects disallowed content types up-front without inserting a node", async () => {
    const editor = makeEditor()
    const upload = vi.fn()
    const notify = vi.fn()
    const handler = makeImageDropPasteHandler({
      slug: "acme",
      upload,
      notify,
      allowedTypes: ALLOWED_MEDIA_CONTENT_TYPES,
      maxBytes: 10 * 1024 * 1024,
    })

    const file = new File([new Uint8Array([1, 2, 3])], "evil.tiff", {
      type: "image/tiff",
    })
    await handler.uploadFile(editor.view, file)

    expect(upload).not.toHaveBeenCalled()
    expect(notify).toHaveBeenCalledWith(expect.stringMatching(/unsupported/i))
    expect(findImageNode(editor)).toBeNull()
  })

  it("removes the placeholder when the upload is interrupted", async () => {
    const editor = makeEditor()
    const upload = vi
      .fn()
      .mockRejectedValue(
        new ApiError(500, "internal", "upload exploded", "/t/acme/api/media"),
      )
    const notify = vi.fn()
    const handler = makeImageDropPasteHandler({
      slug: "acme",
      upload,
      notify,
      allowedTypes: ALLOWED_MEDIA_CONTENT_TYPES,
      maxBytes: 10 * 1024 * 1024,
    })

    const file = new File([new Uint8Array([1, 2, 3])], "cat.png", {
      type: "image/png",
    })
    await handler.uploadFile(editor.view, file)

    expect(upload).toHaveBeenCalledOnce()
    expect(notify).toHaveBeenCalledWith(expect.stringMatching(/upload failed/i))
    // The placeholder must be removed so the doc doesn't carry an
    // unrenderable `pending:` mediaRef that would fail save-time validation.
    expect(findImageNode(editor)).toBeNull()
  })

  it("keeps placeholder visible while upload is in flight", async () => {
    const editor = makeEditor()
    let resolveUpload: (value: unknown) => void = () => {}
    const upload = vi.fn().mockReturnValue(
      new Promise((resolve) => {
        resolveUpload = resolve
      }),
    )
    const notify = vi.fn()
    const handler = makeImageDropPasteHandler({
      slug: "acme",
      upload,
      notify,
      allowedTypes: ALLOWED_MEDIA_CONTENT_TYPES,
      maxBytes: 10 * 1024 * 1024,
    })

    const file = new File([new Uint8Array([1, 2, 3])], "cat.png", {
      type: "image/png",
    })
    const done = handler.uploadFile(editor.view, file)

    // Before the upload resolves, the placeholder must exist with a
    // `pending:` mediaRef so the editor shows the operator that work is
    // in flight (rendered by the NodeView as a "Uploading..." cell).
    const placeholder = findImageNode(editor)
    expect(placeholder).not.toBeNull()
    expect(isPendingMediaRef(placeholder!.mediaRef)).toBe(true)
    expect(placeholder!.alt).toBe("cat.png")

    resolveUpload({
      status: 200,
      ok: true,
      data: {
        id: "asset-1",
        public_url: "https://media.example/tenants/acme/cat.png",
        filename: "cat.png",
      },
    })
    await done

    expect(findImageNode(editor)).toEqual({
      mediaRef: "https://media.example/tenants/acme/cat.png",
      alt: "cat.png",
    })
  })

  it("inserts the image at the current selection (paste path)", async () => {
    const editor = makeEditor()
    editor.commands.setContent({
      type: "doc",
      content: [
        { type: "paragraph", content: [{ type: "text", text: "before" }] },
      ],
    })
    editor.commands.focus("end")

    const upload = vi.fn().mockResolvedValue({
      status: 200,
      ok: true,
      data: {
        id: "asset-1",
        public_url: "https://media.example/tenants/acme/x.png",
        filename: "x.png",
      },
    })
    const handler = makeImageDropPasteHandler({
      slug: "acme",
      upload,
      notify: vi.fn(),
      allowedTypes: ALLOWED_MEDIA_CONTENT_TYPES,
      maxBytes: 10 * 1024 * 1024,
    })

    const file = new File([new Uint8Array([1, 2, 3])], "x.png", {
      type: "image/png",
    })
    await handler.uploadFile(editor.view, file)

    expect(findImageNode(editor)).toEqual({
      mediaRef: "https://media.example/tenants/acme/x.png",
      alt: "x.png",
    })
  })
})
