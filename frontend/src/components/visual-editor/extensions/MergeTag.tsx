// MergeTag — inline atom node, serializes to
// `{ type: "mergeTag", attrs: { namespace, key } }`. The editor renders it
// as a styled pill carrying the display label as visible text and the
// raw `{{ … }}` placeholder string in `title=`. The BFF renderer emits
// the literal placeholder string into the HTML output so the send-time
// substituter (internal/sending/domain/substitution.go) can resolve it
// per recipient.

import { Node, mergeAttributes } from "@tiptap/core"
import type { MergeTagNamespace } from "@/lib/api-types"

export type MergeTagAttrs = {
  namespace: MergeTagNamespace
  key: string
  // Optional display name carried for the in-editor pill — never persisted
  // to the wire. The renderer ignores it.
  label?: string
}

export const MergeTag = Node.create({
  name: "mergeTag",
  group: "inline",
  inline: true,
  atom: true,
  selectable: true,
  addAttributes() {
    return {
      namespace: {
        default: "subscriber" as MergeTagNamespace,
        rendered: true,
      },
      key: { default: "" },
      // Display label is editor-only — strip it from JSON via
      // `toJSON` semantics handled by `addStorage` below.
      label: {
        default: "",
        rendered: false,
        parseHTML: (el) => el.getAttribute("data-label") ?? "",
        renderHTML: (attrs) => {
          if (!attrs.label) return {}
          return { "data-label": String(attrs.label), title: placeholderOf(attrs as MergeTagAttrs) }
        },
      },
    }
  },
  parseHTML() {
    return [{ tag: "span[data-type=\"merge-tag\"]" }]
  },
  renderHTML({ HTMLAttributes, node }) {
    const attrs = node.attrs as MergeTagAttrs
    return [
      "span",
      mergeAttributes(HTMLAttributes, {
        "data-type": "merge-tag",
        class: "ve-merge-tag",
        title: placeholderOf(attrs),
      }),
      attrs.label || placeholderOf(attrs),
    ]
  },
})

export function placeholderOf({ namespace, key }: MergeTagAttrs): string {
  return `{{ ${namespace}.${key} }}`
}
