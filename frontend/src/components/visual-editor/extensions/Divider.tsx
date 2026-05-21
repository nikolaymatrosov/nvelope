// Divider block — atom node, serializes to `{ type: "divider" }`.

import { Node, mergeAttributes } from "@tiptap/core"

export const Divider = Node.create({
  name: "divider",
  group: "block",
  atom: true,
  draggable: true,
  selectable: true,
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
