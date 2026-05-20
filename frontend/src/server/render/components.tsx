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
  return <REText style={{ color: theme.textColor }}>{renderInlines(block.content)}</REText>
}

function HeadingView({ block, theme }: { block: HeadingBlock; theme: Theme }) {
  // Index by level rather than constructing the tag via template literal —
  // sidesteps a TS / eslint disagreement about narrowing template-literal
  // types onto react-email's strict `as` union ("h1" | "h2" | … | "h6").
  const tag = (["h1", "h2", "h3"] as const)[block.attrs.level - 1] ?? "h1"
  return (
    <REHeading as={tag} style={{ color: theme.textColor }}>
      {renderInlines(block.content)}
    </REHeading>
  )
}

function BulletListView({
  block,
  theme,
}: {
  block: { type: "bulletList"; content: Array<{ type: "listItem"; content: Array<VisualBlock> }> }
  theme: Theme
}) {
  return (
    <ul style={{ color: theme.textColor, paddingLeft: "20px", margin: "8px 0" }}>
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
  block: { type: "orderedList"; content: Array<{ type: "listItem"; content: Array<VisualBlock> }> }
  theme: Theme
}) {
  return (
    <ol style={{ color: theme.textColor, paddingLeft: "20px", margin: "8px 0" }}>
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
  block: { type: "blockquote"; content: Array<VisualBlock> }
  theme: Theme
}) {
  return (
    <blockquote
      style={{
        margin: "8px 0",
        paddingLeft: "12px",
        borderLeft: "4px solid #cccccc",
        color: theme.textColor,
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
  const img = <REImg src={block.attrs.mediaRef} alt={block.attrs.alt} />
  if (block.attrs.href) {
    return <RELink href={block.attrs.href}>{img}</RELink>
  }
  return img
}

function ButtonView({ block, theme }: { block: ButtonBlock; theme: Theme }) {
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
      }}
    >
      {block.attrs.label}
    </REButton>
  )
}

function DividerView({ block: _block }: { block: DividerBlock; theme: Theme }) {
  return <REHr />
}

function ColumnsView({ block, theme }: { block: ColumnsBlock; theme: Theme }) {
  // Equal-width columns. react-email's <Row>/<Column> already emits MSO
  // conditional comments so Outlook desktop renders the table correctly.
  return (
    <RERow>
      {block.content.map((col, i) => (
        <REColumn key={i}>
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
