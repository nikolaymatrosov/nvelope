// React-email components that render one VisualBlock or Inline at a time.
// The renderer (index.ts) walks the typed VisualDoc tree and emits this
// component tree to @react-email/render. The mapping table is fixed by
// specs/014-visual-email-editor/research.md § R4.
//
// Marks (bold/italic/underline/strike/color/link) become nested inline
// tags; links use react-email's <Link>. MergeTag inlines emit their literal
// `{{ namespace.key }}` text so substitution at send time stays a pure
// string replacement on the persisted HTML.

import * as React from "react"
import {
  Button as REButton,
  Column as REColumn,
  Container as REContainer,
  Heading as REHeading,
  Hr as REHr,
  Img as REImg,
  Link as RELink,
  Row as RERow,
  Text as REText,
} from "react-email"

import type {
  BlockStyle,
  ButtonBlock,
  CodeBlock as CodeBlockNode,
  ColumnsBlock,
  DividerBlock,
  HeadingBlock,
  ImageBlock,
  Inline,
  Mark,
  ParagraphBlock,
  RawHtmlBlock,
  Theme,
  VisualBlock,
} from "./types"

// mapBlockStyle translates the email-safe BlockStyle value object (feature 017)
// into a React inline-style object. Only set fields are emitted, so callers can
// spread the result *after* the theme defaults to get
// "theme default → per-block override" semantics (FR-014/FR-019). Pixel fields
// gain a "px" unit; line-height stays unitless.
export function mapBlockStyle(style?: BlockStyle): React.CSSProperties {
  const s: React.CSSProperties = {}
  if (!style) return s
  if (style.backgroundColor) s.backgroundColor = style.backgroundColor
  if (style.color) s.color = style.color
  if (style.fontFamily) s.fontFamily = style.fontFamily
  if (style.fontSize != null) s.fontSize = `${style.fontSize}px`
  if (style.fontWeight != null) s.fontWeight = style.fontWeight
  if (style.lineHeight != null) s.lineHeight = style.lineHeight
  if (style.textAlign) s.textAlign = style.textAlign
  if (style.paddingTop != null) s.paddingTop = `${style.paddingTop}px`
  if (style.paddingRight != null) s.paddingRight = `${style.paddingRight}px`
  if (style.paddingBottom != null) s.paddingBottom = `${style.paddingBottom}px`
  if (style.paddingLeft != null) s.paddingLeft = `${style.paddingLeft}px`
  if (style.borderRadius != null) s.borderRadius = `${style.borderRadius}px`
  if (style.borderWidth != null) s.borderWidth = `${style.borderWidth}px`
  if (style.borderStyle) s.borderStyle = style.borderStyle
  if (style.borderColor) s.borderColor = style.borderColor
  return s
}

// inlineStyleForMarks composes the React inline-style object that
// non-structural marks (color) need; structural marks (bold/italic/etc.) use
// nested tags.
function inlineStyleForMarks(marks?: Array<Mark>): React.CSSProperties {
  const style: React.CSSProperties = {}
  if (!marks) return style
  for (const m of marks) {
    if (m.type === "color") {
      style.color = m.attrs.color
    }
  }
  return style
}

// renderInline wraps a Text or MergeTag node in the appropriate mark tags.
// The output is a span (or a Link if a link mark is present); callers may
// place it inside a paragraph, heading, list item, or column.
export function renderInline(inline: Inline, index: number): React.ReactElement {
  if (inline.type === "mergeTag") {
    // The literal placeholder text is what the send-pipeline substituter
    // operates on. Whitespace inside the braces is allowed by the parser
    // regex so the canonical emission keeps single spaces for readability.
    const literal = `{{ ${inline.attrs.namespace}.${inline.attrs.key} }}`
    return <React.Fragment key={index}>{literal}</React.Fragment>
  }
  const marks = inline.marks ?? []
  // Apply structural marks innermost-first so the order of nesting matches
  // browser/email-client rendering: <a><strong><em>text</em></strong></a>.
  let node: React.ReactNode = inline.text
  for (const m of marks) {
    switch (m.type) {
      case "bold":
        node = <strong>{node}</strong>
        break
      case "italic":
        node = <em>{node}</em>
        break
      case "underline":
        node = <u>{node}</u>
        break
      case "strike":
        node = <s>{node}</s>
        break
      // color is handled via inline style on the wrapping span (below)
      case "color":
      case "link":
        break
    }
  }
  const linkMark = marks.find((m): m is Extract<Mark, { type: "link" }> => m.type === "link")
  const colorStyle = inlineStyleForMarks(marks)
  const hasColor = Object.keys(colorStyle).length > 0
  const wrappedColor = hasColor ? <span style={colorStyle}>{node}</span> : node
  if (linkMark) {
    return (
      <RELink key={index} href={linkMark.attrs.href}>
        {wrappedColor}
      </RELink>
    )
  }
  return <React.Fragment key={index}>{wrappedColor}</React.Fragment>
}

function renderInlines(inlines: Array<Inline>): Array<React.ReactElement> {
  return inlines.map((it, i) => renderInline(it, i))
}

// ── Block renderers ────────────────────────────────────────────────────────

function ParagraphView({ block, theme }: { block: ParagraphBlock; theme: Theme }) {
  return (
    <REText style={{ color: theme.textColor, ...mapBlockStyle(block.attrs?.style) }}>
      {renderInlines(block.content)}
    </REText>
  )
}

function HeadingView({ block, theme }: { block: HeadingBlock; theme: Theme }) {
  // Index by level rather than constructing the tag via template literal —
  // sidesteps a TS / eslint disagreement about narrowing template-literal
  // types onto react-email's strict `as` union ("h1" | "h2" | … | "h6").
  const tag = (["h1", "h2", "h3"] as const)[block.attrs.level - 1] ?? "h1"
  return (
    <REHeading as={tag} style={{ color: theme.textColor, ...mapBlockStyle(block.attrs.style) }}>
      {renderInlines(block.content)}
    </REHeading>
  )
}

function BulletListView({
  block,
  theme,
}: {
  block: {
    type: "bulletList"
    attrs?: { style?: BlockStyle }
    content: Array<{ type: "listItem"; content: Array<VisualBlock> }>
  }
  theme: Theme
}) {
  return (
    <ul
      style={{
        color: theme.textColor,
        paddingLeft: "20px",
        margin: "8px 0",
        ...mapBlockStyle(block.attrs?.style),
      }}
    >
      {block.content.map((item, i) => (
        <li key={i}>
          {item.content.map((child, j) => (
            <BlockView key={j} block={child} theme={theme} />
          ))}
        </li>
      ))}
    </ul>
  )
}

function OrderedListView({
  block,
  theme,
}: {
  block: {
    type: "orderedList"
    attrs?: { style?: BlockStyle }
    content: Array<{ type: "listItem"; content: Array<VisualBlock> }>
  }
  theme: Theme
}) {
  return (
    <ol
      style={{
        color: theme.textColor,
        paddingLeft: "20px",
        margin: "8px 0",
        ...mapBlockStyle(block.attrs?.style),
      }}
    >
      {block.content.map((item, i) => (
        <li key={i}>
          {item.content.map((child, j) => (
            <BlockView key={j} block={child} theme={theme} />
          ))}
        </li>
      ))}
    </ol>
  )
}

function BlockquoteView({
  block,
  theme,
}: {
  block: { type: "blockquote"; attrs?: { style?: BlockStyle }; content: Array<VisualBlock> }
  theme: Theme
}) {
  return (
    <blockquote
      style={{
        margin: "8px 0",
        paddingLeft: "12px",
        borderLeft: "4px solid #cccccc",
        color: theme.textColor,
        ...mapBlockStyle(block.attrs?.style),
      }}
    >
      {block.content.map((child, i) => (
        <BlockView key={i} block={child} theme={theme} />
      ))}
    </blockquote>
  )
}

function CodeBlockView({ block }: { block: CodeBlockNode; theme: Theme }) {
  // CodeBlock node carries one text child whose `text` is the verbatim code.
  // We deliberately do not use react-email's <CodeBlock> — that primitive
  // applies Prism syntax highlighting which is overkill for transactional
  // email and forces a `language` enum onto operators who just want a
  // monospace block. A plain inline-styled <pre> renders consistently
  // across clients.
  const code = block.content.map((t) => t.text).join("")
  return (
    <pre
      style={{
        background: "#f5f5f5",
        padding: "12px",
        borderRadius: "4px",
        fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
        fontSize: "13px",
        whiteSpace: "pre-wrap",
      }}
    >
      {code}
    </pre>
  )
}

function ImageView({ block }: { block: ImageBlock; theme: Theme }) {
  // Image style applies to the <img> box itself (border radius / border). The
  // textAlign field, when set, positions the image within a wrapping block.
  const style = block.attrs.style
  const { textAlign, ...boxStyle } = mapBlockStyle(style)
  const img = <REImg src={block.attrs.mediaRef} alt={block.attrs.alt} style={boxStyle} />
  const linked = block.attrs.href ? <RELink href={block.attrs.href}>{img}</RELink> : img
  if (textAlign) {
    return <div style={{ textAlign }}>{linked}</div>
  }
  return linked
}

function ButtonView({ block, theme }: { block: ButtonBlock; theme: Theme }) {
  // Theme button colors are the base; per-block style overrides them.
  return (
    <REButton
      href={block.attrs.href}
      style={{
        backgroundColor: theme.buttonColor,
        color: theme.buttonTextColor,
        padding: "12px 20px",
        borderRadius: "4px",
        textDecoration: "none",
        display: "inline-block",
        ...mapBlockStyle(block.attrs.style),
      }}
    >
      {block.attrs.label}
    </REButton>
  )
}

function DividerView({ block }: { block: DividerBlock; theme: Theme }) {
  const style = block.attrs?.style
  if (!style) return <REHr />
  // The divider's "line" is the rule's top border; borderColor/Width/Style map
  // onto it, and padding becomes spacing above/below.
  const s = mapBlockStyle(style)
  const hr: React.CSSProperties = {
    borderTopWidth: s.borderWidth,
    borderTopStyle: s.borderStyle,
    borderTopColor: s.borderColor,
    paddingTop: s.paddingTop,
    paddingBottom: s.paddingBottom,
  }
  return <REHr style={hr} />
}

function ColumnsView({ block, theme }: { block: ColumnsBlock; theme: Theme }) {
  // Equal-width columns. react-email's <Row>/<Column> already emits MSO
  // conditional comments so Outlook desktop renders the table correctly. The
  // container style applies to the row; each column carries its own style on
  // the cell so backgrounds/padding survive in Outlook (FR-015).
  return (
    <RERow style={mapBlockStyle(block.attrs.style)}>
      {block.content.map((col, i) => (
        <REColumn key={i} style={mapBlockStyle(col.attrs?.style)}>
          {col.content.map((child, j) => (
            <BlockView key={j} block={child} theme={theme} />
          ))}
        </REColumn>
      ))}
    </RERow>
  )
}

function RawHtmlView({ block }: { block: RawHtmlBlock; theme: Theme }) {
  // The Go-side bluemonday pass is the authoritative gate before
  // persistence (FR-014); this passthrough is sanitized after render.
  return <div dangerouslySetInnerHTML={{ __html: block.attrs.html }} />
}

// BlockView dispatches on the block's `type` discriminant.
export function BlockView({ block, theme }: { block: VisualBlock; theme: Theme }) {
  switch (block.type) {
    case "paragraph":
      return <ParagraphView block={block} theme={theme} />
    case "heading":
      return <HeadingView block={block} theme={theme} />
    case "bulletList":
      return <BulletListView block={block} theme={theme} />
    case "orderedList":
      return <OrderedListView block={block} theme={theme} />
    case "blockquote":
      return <BlockquoteView block={block} theme={theme} />
    case "codeBlock":
      return <CodeBlockView block={block} theme={theme} />
    case "image":
      return <ImageView block={block} theme={theme} />
    case "button":
      return <ButtonView block={block} theme={theme} />
    case "divider":
      return <DividerView block={block} theme={theme} />
    case "columns":
      return <ColumnsView block={block} theme={theme} />
    case "rawHtml":
      return <RawHtmlView block={block} theme={theme} />
    // listItem and column never appear at the top level — they are reached
    // only through their parent list / columns block above. The validator
    // rejects an envelope that places them at the document root.
    case "listItem":
    case "column":
      return null
  }
}

// DocumentView wraps the full block list inside a fixed-width <Container>
// for the row's theme. Email clients clip wider than ~640 px on mobile;
// 320–800 is the validator-enforced range.
export function DocumentView({
  blocks,
  theme,
}: {
  blocks: Array<VisualBlock>
  theme: Theme
}) {
  return (
    <REContainer
      style={{
        maxWidth: `${theme.containerWidth}px`,
        fontFamily: theme.fontFamily,
        color: theme.textColor,
      }}
    >
      {blocks.map((block, i) => (
        <BlockView key={i} block={block} theme={theme} />
      ))}
    </REContainer>
  )
}
