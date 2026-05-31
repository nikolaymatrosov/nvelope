// TypeScript mirror of the Go `internal/campaign/domain` VisualDoc / Theme
// shapes. The BFF renderer walks this exact tree; the SPA editor produces it
// over the wire. Field names and casing match the wire format (see
// specs/014-visual-email-editor/contracts/tenant-api.md § Structured-document
// JSON schema).

export type MergeTagNamespace = "subscriber" | "campaign"

// BlockStyle is the optional, email-safe per-block style the three-pane editor's
// parameters panel produces (feature 017). Every field is optional; an absent
// field means "inherit the document theme / default". The renderer layers any
// set field over the theme defaults as inline CSS. Bounds are enforced by the
// validator (validate/blocks.ts) and mirrored in the Go domain (BlockStyle in
// internal/campaign/domain/visualdoc.go); see
// specs/017-three-pane-visual-editor/data-model.md for the canonical matrix.
export type BlockStyle = {
  backgroundColor?: string // #RGB / #RRGGBB
  color?: string // #RGB / #RRGGBB
  fontFamily?: string // member of the font allow-list (validate/fonts.ts)
  fontSize?: number // px, 8–72
  fontWeight?: 400 | 700
  lineHeight?: number // unitless, 1.0–3.0
  textAlign?: "left" | "center" | "right"
  paddingTop?: number // px, 0–64
  paddingRight?: number
  paddingBottom?: number
  paddingLeft?: number
  borderRadius?: number // px, 0–48
  borderWidth?: number // px, 0–8
  borderStyle?: "solid" | "dashed" | "dotted"
  borderColor?: string // #RGB / #RRGGBB
}

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

export type ParagraphBlock = {
  type: "paragraph"
  attrs?: { style?: BlockStyle }
  content: Array<Inline>
}
export type HeadingBlock = {
  type: "heading"
  attrs: { level: 1 | 2 | 3; style?: BlockStyle }
  content: Array<Inline>
}
export type ListItemBlock = { type: "listItem"; content: Array<VisualBlock> }
export type BulletListBlock = {
  type: "bulletList"
  attrs?: { style?: BlockStyle }
  content: Array<ListItemBlock>
}
export type OrderedListBlock = {
  type: "orderedList"
  attrs?: { style?: BlockStyle }
  content: Array<ListItemBlock>
}
export type BlockquoteBlock = {
  type: "blockquote"
  attrs?: { style?: BlockStyle }
  content: Array<VisualBlock>
}
export type CodeBlock = {
  type: "codeBlock"
  content: Array<{ type: "text"; text: string }>
}
export type ImageBlock = {
  type: "image"
  attrs: { mediaRef: string; alt: string; href: string; style?: BlockStyle }
}
export type ButtonBlock = {
  type: "button"
  attrs: { label: string; href: string; style?: BlockStyle }
}
export type DividerBlock = { type: "divider"; attrs?: { style?: BlockStyle } }
export type ColumnBlock = {
  type: "column"
  attrs?: { style?: BlockStyle }
  content: Array<VisualBlock>
}
export type ColumnsBlock = {
  type: "columns"
  attrs: { count: 2 | 3 | 4; style?: BlockStyle }
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
