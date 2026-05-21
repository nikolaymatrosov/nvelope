// ImageBlock NodeView tests (T100 / FR-021).
//
// Exercises the editor-side affordances on the image block:
//   - An empty mediaRef renders the "Pick from media library" button.
//   - A filled mediaRef renders an <img> alongside a "Replace" button.
//   - When the <img> fires an `error` event (the referenced asset was
//     deleted from the library), the NodeView swaps the broken image for
//     a labelled placeholder with a clear message and a "Pick" button so
//     the operator can resolve the broken reference.

import { afterEach, beforeEach, describe, expect, it } from "vitest"
import { Editor } from "@tiptap/core"
import StarterKit from "@tiptap/starter-kit"
import { ImageBlock } from "./ImageBlock"

function makeEditor(initialContent: object): Editor {
  const host = document.createElement("div")
  document.body.appendChild(host)
  return new Editor({
    element: host,
    extensions: [StarterKit.configure({ hardBreak: false }), ImageBlock],
    content: initialContent,
  })
}

let editor: Editor | null = null

beforeEach(() => {
  document.body.replaceChildren()
})

afterEach(() => {
  editor?.destroy()
  editor = null
})

describe("ImageBlock NodeView", () => {
  it("renders the picker affordance when mediaRef is empty", () => {
    editor = makeEditor({
      type: "doc",
      content: [
        { type: "image", attrs: { mediaRef: "", alt: "", href: "" } },
      ],
    })
    const pick = document.querySelector("[data-testid=\"ve-image-pick\"]")
    expect(pick).not.toBeNull()
    expect(pick?.textContent).toMatch(/pick from media library/i)
  })

  it("renders the image and a replace button when mediaRef is filled", () => {
    editor = makeEditor({
      type: "doc",
      content: [
        {
          type: "image",
          attrs: {
            mediaRef: "https://media.example/tenants/acme/cat.png",
            alt: "Cat",
            href: "",
          },
        },
      ],
    })
    const img = document.querySelector(
      "[data-testid=\"ve-image-img\"]",
    )
    expect(img).not.toBeNull()
    expect(img?.getAttribute("src")).toBe(
      "https://media.example/tenants/acme/cat.png",
    )
    expect(img?.getAttribute("alt")).toBe("Cat")
    expect(
      document.querySelector("[data-testid=\"ve-image-replace\"]"),
    ).not.toBeNull()
  })

  it("swaps the broken image for the missing-asset placeholder on load error", () => {
    editor = makeEditor({
      type: "doc",
      content: [
        {
          type: "image",
          attrs: {
            // A media URL whose backing asset was deleted from the library —
            // the browser's image-load triggers an `error` event which the
            // NodeView observes.
            mediaRef: "https://media.example/tenants/acme/deleted.png",
            alt: "Deleted asset",
            href: "",
          },
        },
      ],
    })

    const img = document.querySelector(
      "[data-testid=\"ve-image-img\"]",
    )
    expect(img).not.toBeNull()

    // Synthesize the `error` event the browser would dispatch for a 404
    // image. jsdom doesn't actually fetch the URL but the `error` event
    // path on HTMLImageElement is standard DOM.
    img!.dispatchEvent(new Event("error"))

    const placeholder = document.querySelector(
      "[data-testid=\"ve-image-missing\"]",
    )
    expect(placeholder).not.toBeNull()
    expect(placeholder?.textContent).toMatch(/no longer available/i)
    // The placeholder retains a path to recovery — operator can pick a
    // replacement asset right from the broken state.
    expect(
      placeholder?.querySelector("[data-testid=\"ve-image-pick\"]"),
    ).not.toBeNull()
  })
})
