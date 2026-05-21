// Image block — atom node, serializes to
// `{ type: "image", attrs: { mediaRef, alt, href } }`. mediaRef is the
// canonical tenant media URL (per FR-021); the BFF renderer emits the
// final `<img>` against the same URL. The MediaPicker affordance lives in
// the slash command menu / bubble menu; this extension only defines the
// node shape and the editor rendering.

import { Node, mergeAttributes } from "@tiptap/core"

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
})
