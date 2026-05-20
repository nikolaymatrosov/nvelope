// TypeScript mirror of the Go `internal/campaign/domain` VisualDoc / Theme
// shapes. The BFF renderer walks this exact tree; the SPA editor produces it
// over the wire. Field names and casing match the wire format (see
// specs/014-visual-email-editor/contracts/tenant-api.md § Structured-document
// JSON schema).

export type MergeTagNamespace = "subscriber" | "campaign"

export type Mark =
  | { type: "bold" }
  | { type: "italic" }
  | { type: "underline" }
  | { type: "strike" }
  | { type: "color"; attrs: { color: string } }
  | { type: "link"; attrs: { href: string } }

export type TextInline = {
  type: "text"
  text: string
  marks?: Array<Mark>
}

export type MergeTagInline = {
  type: "mergeTag"
  attrs: { namespace: MergeTagNamespace; key: string }
}

export type Inline = TextInline | MergeTagInline

export type ParagraphBlock = { type: "paragraph"; content: Array<Inline> }
export type HeadingBlock = {
  type: "heading"
  attrs: { level: 1 | 2 | 3 }
  content: Array<Inline>
}
export type ListItemBlock = { type: "listItem"; content: Array<VisualBlock> }
export type BulletListBlock = { type: "bulletList"; content: Array<ListItemBlock> }
export type OrderedListBlock = { type: "orderedList"; content: Array<ListItemBlock> }
export type BlockquoteBlock = { type: "blockquote"; content: Array<VisualBlock> }
export type CodeBlock = {
  type: "codeBlock"
  content: Array<{ type: "text"; text: string }>
}
export type ImageBlock = {
  type: "image"
  attrs: { mediaRef: string; alt: string; href: string }
}
export type ButtonBlock = {
  type: "button"
  attrs: { label: string; href: string }
}
export type DividerBlock = { type: "divider" }
export type ColumnBlock = { type: "column"; content: Array<VisualBlock> }
export type ColumnsBlock = {
  type: "columns"
  attrs: { count: 2 | 3 | 4 }
  content: Array<ColumnBlock>
}
export type RawHtmlBlock = { type: "rawHtml"; attrs: { html: string } }

export type VisualBlock =
  | ParagraphBlock
  | HeadingBlock
  | BulletListBlock
  | OrderedListBlock
  | ListItemBlock
  | BlockquoteBlock
  | CodeBlock
  | ImageBlock
  | ButtonBlock
  | DividerBlock
  | ColumnsBlock
  | ColumnBlock
  | RawHtmlBlock

export type VisualDoc = {
  version: 1
  type: "doc"
  content: Array<VisualBlock>
}

// Theme value object. Mirrors `internal/campaign/domain.Theme`. NULL on the
// row means "inherit tenant branding"; at render time the BFF resolves the
// effective theme by fetching `GET /branding` from Go and applying defaults.
export type Theme = {
  textColor: string
  linkColor: string
  buttonColor: string
  buttonTextColor: string
  fontFamily: string
  containerWidth: number
}

export type RenderWarning = {
  kind: string
  detail: string
}
