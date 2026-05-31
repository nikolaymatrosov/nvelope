// Button block — atom node with `{ label, href }`. The editor renders it as
// a styled chip the operator can click to edit; the BFF renderer emits
// the canonical `<a role="button">` (or VML for Outlook) at save time.

import { Node, mergeAttributes } from "@tiptap/core"
import { blockStyleAttributeSpec } from "./styleAttr"

export const Button = Node.create({
  name: "button",
  group: "block",
  atom: true,
  draggable: true,
  selectable: true,
  addAttributes() {
    return {
      label: { default: "Button" },
      href: { default: "" },
      style: blockStyleAttributeSpec,
    }
  },
  parseHTML() {
    return [{ tag: "a[data-type=\"button\"]" }]
  },
  renderHTML({ HTMLAttributes, node }) {
    return [
      "a",
      mergeAttributes(HTMLAttributes, {
        "data-type": "button",
        class: "ve-button",
        href: node.attrs.href || "#",
      }),
      String(node.attrs.label ?? "Button"),
    ]
  },
})
