// Divider block — atom node, serializes to `{ type: "divider" }`.

import { Node, mergeAttributes } from "@tiptap/core"
import { blockStyleAttributeSpec } from "./styleAttr"

export const Divider = Node.create({
  name: "divider",
  group: "block",
  atom: true,
  draggable: true,
  selectable: true,
  addAttributes() {
    return { style: blockStyleAttributeSpec }
  },
  parseHTML() {
    return [{ tag: "hr[data-type=\"divider\"]" }]
  },
  renderHTML({ HTMLAttributes }) {
    return [
      "hr",
      mergeAttributes(HTMLAttributes, {
        "data-type": "divider",
        class: "ve-divider",
      }),
    ]
  },
})
