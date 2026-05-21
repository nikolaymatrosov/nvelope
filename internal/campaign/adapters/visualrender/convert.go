package visualrender

import (
	"bytes"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// ConversionWarning is a single warning emitted by Convert when a region of
// the input HTML could not be mapped onto the typed block vocabulary and was
// preserved verbatim inside a RawHTML block. The contracts doc surfaces this
// list under "warnings" on the POST /:id/convert-to-visual response so the
// operator can review which regions stayed opaque before saving with the
// visual PUT.
type ConversionWarning struct {
	// Kind is the stable identifier consumed by the SPA.
	Kind string `json:"kind"`
	// Detail is a short human-readable explanation suitable for surfacing
	// inline next to the block in the editor.
	Detail string `json:"detail"`
	// Path points at the resulting block's location in the converted doc
	// using JSONPath-like indices, e.g. "content[3]".
	Path string `json:"path"`
}

// ConvertOptions configures the raw-HTML → VisualDoc conversion.
type ConvertOptions struct {
	// MediaRefs, when non-nil, is consulted before converting an <img> into a
	// typed Image block. An <img> whose src does not validate falls back to a
	// RawHTML block — the verbatim bytes survive even though the typed Image
	// rule (FR-021) would have rejected the src.
	MediaRefs domain.MediaRefValidator
}

// Convert parses raw HTML into a VisualDoc using the deliberately conservative
// heuristics documented in research.md § R6: nodes whose tags map cleanly to
// the block vocabulary become typed blocks; everything else collapses to a
// RawHTML block carrying the source bytes verbatim. The converter never fails
// shape-wise — any input parses into something — but it returns warnings for
// every RawHTML fallback so the operator can review the regions that did not
// round-trip cleanly.
//
// The converter is non-persisting; the HTTP handler returns the candidate
// doc to the operator who reviews the result in the editor and saves via the
// regular visual PUT, which is what triggers placeholder + media-ref
// revalidation and Go-side sanitization.
func Convert(htmlText string, opts ConvertOptions) (*domain.VisualDoc, []ConversionWarning, error) {
	root, err := html.Parse(strings.NewReader(htmlText))
	if err != nil {
		return nil, nil, domain.ErrVisualDocInvalid.WithMessage("the existing HTML cannot be parsed")
	}
	body := findBody(root)
	if body == nil {
		// Document had no <body> element — treat the entire input as one
		// RawHTML block so the bytes survive. html.Parse always synthesizes
		// a body, so this path is mainly defensive.
		raw := strings.TrimSpace(htmlText)
		if raw == "" {
			return &domain.VisualDoc{Version: 1}, nil, nil
		}
		return &domain.VisualDoc{
				Version: 1,
				Nodes:   []domain.Node{domain.RawHTML{HTML: raw}},
			},
			[]ConversionWarning{{
				Kind:   "rawhtml_block",
				Detail: "input had no <body> — preserved verbatim",
				Path:   "nodes[0]",
			}},
			nil
	}

	c := &converter{opts: opts}
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		c.appendBlock(child)
	}
	c.flushPendingInlines()

	doc := &domain.VisualDoc{Version: 1, Nodes: c.nodes}
	return doc, c.warnings, nil
}

// converter accumulates the converted nodes and any RawHTML-fallback warnings
// as it walks the parsed HTML tree.
type converter struct {
	opts            ConvertOptions
	nodes           []domain.Node
	warnings        []ConversionWarning
	pendingInlines  []domain.Inline
	pendingHasMerge bool
}

// appendBlock converts a single child of <body> into one or more top-level
// blocks. Whitespace-only text nodes between blocks are dropped; non-empty
// inline content between blocks is accumulated into a synthesized Paragraph
// (flushed lazily when the next block-level node arrives, or at the end).
func (c *converter) appendBlock(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		if strings.TrimSpace(n.Data) == "" {
			return
		}
		c.pendingInlines = append(c.pendingInlines, domain.Text{Text: n.Data})
		return

	case html.CommentNode:
		// Drop HTML comments — they are not visible content. MSO conditional
		// comments inside RawHTML survive because they're inside a verbatim
		// raw block, not at the top level.
		return

	case html.ElementNode:
		// Element node — fall through to the per-tag switch below.

	default:
		return
	}

	// At this point n.Type == html.ElementNode. If an inline run was building
	// up, finalize it as a Paragraph before introducing a block-level break.
	if isInlineElement(n) {
		c.collectInlinesInto(n, &c.pendingInlines)
		return
	}
	c.flushPendingInlines()

	switch n.DataAtom {
	case atom.P:
		inlines := c.gatherInlines(n)
		if len(inlines) > 0 {
			c.nodes = append(c.nodes, domain.Paragraph{Children: inlines})
		}

	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		level := int(n.Data[1] - '0')
		if level > 3 {
			level = 3
		}
		inlines := c.gatherInlines(n)
		if len(inlines) == 0 {
			return
		}
		c.nodes = append(c.nodes, domain.Heading{Level: level, Children: inlines})

	case atom.Ul:
		items := c.gatherListItems(n)
		if len(items) == 0 {
			return
		}
		c.nodes = append(c.nodes, domain.BulletList{Items: items})

	case atom.Ol:
		items := c.gatherListItems(n)
		if len(items) == 0 {
			return
		}
		c.nodes = append(c.nodes, domain.OrderedList{Items: items})

	case atom.Blockquote:
		children := c.gatherBlocks(n)
		if len(children) == 0 {
			return
		}
		c.nodes = append(c.nodes, domain.Quote{Children: children})

	case atom.Hr:
		c.nodes = append(c.nodes, domain.Divider{})

	case atom.Img:
		if img, ok := c.imageFromElement(n, ""); ok {
			c.nodes = append(c.nodes, img)
			return
		}
		c.appendRawFallback(n, "image src is not a tenant media reference")

	case atom.A:
		// A bare <a> at block level is most commonly a button-shaped link or
		// an image link. If it wraps exactly one <img>, convert to an Image
		// block with Href; otherwise treat the link as inline text.
		if img, href, ok := c.imageLink(n); ok {
			img.Href = href
			c.nodes = append(c.nodes, img)
			return
		}
		c.collectInlinesInto(n, &c.pendingInlines)
		c.flushPendingInlines()

	case atom.Table:
		if cols, ok := c.tryColumns(n); ok {
			c.nodes = append(c.nodes, cols)
			return
		}
		c.appendRawFallback(n, "table does not match a 2/3/4-column layout")

	case atom.Pre, atom.Code:
		text := textContent(n)
		if text == "" {
			return
		}
		c.nodes = append(c.nodes, domain.Code{Text: text})

	case atom.Br:
		// A stray <br> at block level becomes an empty paragraph for spacing.
		c.nodes = append(c.nodes, domain.Paragraph{Children: []domain.Inline{domain.Text{Text: ""}}})

	case atom.Div, atom.Section, atom.Article, atom.Header, atom.Footer,
		atom.Main, atom.Nav, atom.Aside:
		// Container elements: recurse into their children as block-level
		// content. This is how typical Gmail / newsletter HTML — which wraps
		// real content in nested <div> shells — converts cleanly.
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			c.appendBlock(child)
		}
		c.flushPendingInlines()

	default:
		c.appendRawFallback(n, "unsupported element <"+n.Data+">")
	}
}

// flushPendingInlines wraps any accumulated inlines into a Paragraph and
// clears the buffer. Called between block-level nodes and at the end of the
// document.
func (c *converter) flushPendingInlines() {
	if len(c.pendingInlines) == 0 {
		return
	}
	if !inlinesHaveVisibleContent(c.pendingInlines) {
		c.pendingInlines = nil
		c.pendingHasMerge = false
		return
	}
	c.nodes = append(c.nodes, domain.Paragraph{Children: c.pendingInlines})
	c.pendingInlines = nil
	c.pendingHasMerge = false
}

// gatherInlines collects the inline children of n into a slice. The result
// preserves text content and mark formatting from supported inline tags;
// unrecognized inlines fall through as plain text via textContent.
func (c *converter) gatherInlines(n *html.Node) []domain.Inline {
	var out []domain.Inline
	c.collectInlinesInto(n, &out)
	return out
}

// collectInlinesInto walks the children of n and appends Text / MergeTag
// inlines to out, applying any marks that the current inline ancestor chain
// carries. It is the recursive workhorse behind gatherInlines and behind the
// pending-inline accumulator.
func (c *converter) collectInlinesInto(n *html.Node, out *[]domain.Inline) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		c.collectInline(child, domain.Marks{}, out)
	}
}

func (c *converter) collectInline(n *html.Node, marks domain.Marks, out *[]domain.Inline) {
	switch n.Type {
	case html.TextNode:
		c.appendTextSegments(n.Data, marks, out)
		return
	case html.ElementNode:
		// fall through to the per-tag switch.
	default:
		return
	}

	switch n.DataAtom {
	case atom.B, atom.Strong:
		next := marks
		next.Bold = true
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			c.collectInline(child, next, out)
		}
	case atom.I, atom.Em:
		next := marks
		next.Italic = true
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			c.collectInline(child, next, out)
		}
	case atom.U:
		next := marks
		next.Underline = true
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			c.collectInline(child, next, out)
		}
	case atom.S, atom.Strike, atom.Del:
		next := marks
		next.Strike = true
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			c.collectInline(child, next, out)
		}
	case atom.A:
		next := marks
		if href := attrValue(n, "href"); href != "" {
			next.Link = href
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			c.collectInline(child, next, out)
		}
	case atom.Span, atom.Font:
		next := marks
		if color := cssColorFromStyle(attrValue(n, "style")); color != "" {
			next.Color = color
		} else if color := attrValue(n, "color"); color != "" {
			next.Color = color
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			c.collectInline(child, next, out)
		}
	case atom.Br:
		*out = append(*out, domain.Text{Text: "\n", Marks: marks})
	default:
		// Unknown inline element — drop its tags but preserve its text.
		// Marks accumulated up to this point still apply.
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			c.collectInline(child, marks, out)
		}
	}
}

// appendTextSegments tokenizes a text run into Text and MergeTag inlines.
// MergeTag literals — `{{ subscriber.<slug> }}` / `{{ campaign.<name> }}` —
// surface as typed MergeTag inlines so the editor renders them as chips on
// reload rather than as literal curly-brace text. Unrecognized `{{ … }}`
// literals stay as plain text and the validator will catch them on save.
func (c *converter) appendTextSegments(text string, marks domain.Marks, out *[]domain.Inline) {
	for {
		open := strings.Index(text, "{{")
		if open < 0 {
			if text != "" {
				*out = append(*out, domain.Text{Text: text, Marks: marks})
			}
			return
		}
		close := strings.Index(text[open:], "}}")
		if close < 0 {
			*out = append(*out, domain.Text{Text: text, Marks: marks})
			return
		}
		close += open
		if open > 0 {
			*out = append(*out, domain.Text{Text: text[:open], Marks: marks})
		}
		inner := strings.TrimSpace(text[open+2 : close])
		if ns, key, ok := parseMergeTag(inner); ok {
			*out = append(*out, domain.MergeTag{Namespace: ns, Key: key})
			c.pendingHasMerge = true
		} else {
			*out = append(*out, domain.Text{Text: text[open : close+2], Marks: marks})
		}
		text = text[close+2:]
	}
}

// parseMergeTag recognizes the namespace.key form, accepting only the two
// platform namespaces; anything else stays as literal text so the save
// handler's validation surface can report it.
func parseMergeTag(s string) (domain.MergeTagNamespace, string, bool) {
	dot := strings.IndexByte(s, '.')
	if dot < 1 || dot == len(s)-1 {
		return "", "", false
	}
	ns := strings.TrimSpace(s[:dot])
	key := strings.TrimSpace(s[dot+1:])
	if key == "" {
		return "", "", false
	}
	switch domain.MergeTagNamespace(ns) {
	case domain.MergeTagSubscriber, domain.MergeTagCampaign:
		return domain.MergeTagNamespace(ns), key, true
	}
	return "", "", false
}

// gatherListItems walks the <li> children of a <ul>/<ol> and converts each
// into a ListItem carrying its block-level children.
func (c *converter) gatherListItems(n *html.Node) []domain.ListItem {
	var items []domain.ListItem
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || child.DataAtom != atom.Li {
			continue
		}
		inner := c.subConvert(child)
		if len(inner) == 0 {
			continue
		}
		items = append(items, domain.ListItem{Children: inner})
	}
	return items
}

// gatherBlocks walks the children of n and returns them as block-level nodes
// — used for <blockquote> bodies, which may carry nested blocks.
func (c *converter) gatherBlocks(n *html.Node) []domain.Node {
	return c.subConvert(n)
}

// subConvert runs a nested conversion over the children of n with a fresh
// inline buffer; warnings from the nested pass merge back into the parent.
// Used for list-item and quote bodies.
func (c *converter) subConvert(n *html.Node) []domain.Node {
	nested := &converter{opts: c.opts}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		nested.appendBlock(child)
	}
	nested.flushPendingInlines()
	c.warnings = append(c.warnings, nested.warnings...)
	return nested.nodes
}

// imageFromElement converts an <img> into an Image block. When the optional
// MediaRefs validator rejects the src, the conversion is refused so the
// caller can fall back to a RawHTML block.
func (c *converter) imageFromElement(n *html.Node, href string) (domain.Image, bool) {
	src := attrValue(n, "src")
	if src == "" {
		return domain.Image{}, false
	}
	if c.opts.MediaRefs != nil && !c.opts.MediaRefs.IsTenantMediaRef(src) {
		return domain.Image{}, false
	}
	return domain.Image{
		MediaRef: src,
		Alt:      attrValue(n, "alt"),
		Href:     href,
	}, true
}

// imageLink recognizes the common <a href="…"><img …></a> shape (a clickable
// image) and returns the wrapped image plus its href. The conversion is
// refused when the <a> wraps anything besides a single <img>.
func (c *converter) imageLink(a *html.Node) (domain.Image, string, bool) {
	var only *html.Node
	for child := a.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode && strings.TrimSpace(child.Data) == "" {
			continue
		}
		if only != nil {
			return domain.Image{}, "", false
		}
		only = child
	}
	if only == nil || only.Type != html.ElementNode || only.DataAtom != atom.Img {
		return domain.Image{}, "", false
	}
	img, ok := c.imageFromElement(only, attrValue(a, "href"))
	if !ok {
		return domain.Image{}, "", false
	}
	return img, attrValue(a, "href"), true
}

// tryColumns recognizes a <table> shape whose immediate <tr> has 2/3/4 <td>
// children, none of which use colspan or rowspan. Each cell's content is
// recursively converted into block-level children of one Column.
func (c *converter) tryColumns(table *html.Node) (domain.Columns, bool) {
	tr := firstRowOf(table)
	if tr == nil {
		return domain.Columns{}, false
	}
	var cells []*html.Node
	for child := tr.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		if child.DataAtom != atom.Td && child.DataAtom != atom.Th {
			continue
		}
		if attrValue(child, "colspan") != "" || attrValue(child, "rowspan") != "" {
			return domain.Columns{}, false
		}
		cells = append(cells, child)
	}
	switch len(cells) {
	case 2, 3, 4:
		// proceed
	default:
		return domain.Columns{}, false
	}
	// A <table> with more than one immediate <tr> in <tbody>/<thead> is not
	// a column layout — bail out so the table preserves verbatim.
	if extraRowsBeyond(table, tr) {
		return domain.Columns{}, false
	}
	cols := make([][]domain.Node, len(cells))
	for i, cell := range cells {
		inner := c.subConvert(cell)
		if len(inner) == 0 {
			inner = []domain.Node{domain.Paragraph{Children: []domain.Inline{domain.Text{Text: ""}}}}
		}
		cols[i] = inner
	}
	return domain.Columns{Cols: cols}, true
}

// appendRawFallback serializes n verbatim and records a warning pointing at
// the resulting RawHTML block in the converted doc. Used whenever a node
// cannot be safely mapped to a typed block.
func (c *converter) appendRawFallback(n *html.Node, reason string) {
	var buf bytes.Buffer
	if err := html.Render(&buf, n); err != nil {
		// Render only fails on a writer error; bytes.Buffer never errors.
		return
	}
	raw := strings.TrimSpace(buf.String())
	if raw == "" {
		return
	}
	c.nodes = append(c.nodes, domain.RawHTML{HTML: raw})
	c.warnings = append(c.warnings, ConversionWarning{
		Kind:   "rawhtml_block",
		Detail: reason,
		Path:   "nodes[" + strconv.Itoa(len(c.nodes)-1) + "]",
	})
}

// findBody returns the first <body> descendant of n, or nil if none exists.
func findBody(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	if n.Type == html.ElementNode && n.DataAtom == atom.Body {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if b := findBody(child); b != nil {
			return b
		}
	}
	return nil
}

// firstRowOf returns the first <tr> descendant of a <table>, looking inside
// the optional <thead> / <tbody> wrappers the parser inserts.
func firstRowOf(table *html.Node) *html.Node {
	for child := table.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		switch child.DataAtom {
		case atom.Tr:
			return child
		case atom.Thead, atom.Tbody, atom.Tfoot:
			if row := firstRowOf(child); row != nil {
				return row
			}
		}
	}
	return nil
}

// extraRowsBeyond reports whether the table has any <tr> besides the supplied
// first row. Used to bail out of the Columns heuristic for multi-row tables.
func extraRowsBeyond(table, firstRow *html.Node) bool {
	count := 0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if child.Type != html.ElementNode {
				continue
			}
			switch child.DataAtom {
			case atom.Tr:
				count++
				if count > 1 {
					return
				}
			case atom.Thead, atom.Tbody, atom.Tfoot:
				walk(child)
			}
		}
	}
	walk(table)
	_ = firstRow
	return count > 1
}

// isInlineElement reports whether the element is treated as inline content
// for block-construction purposes. Anything else is a block boundary.
func isInlineElement(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch n.DataAtom {
	case atom.B, atom.Strong, atom.I, atom.Em, atom.U, atom.S, atom.Strike,
		atom.Del, atom.Span, atom.Font, atom.Mark, atom.Small,
		atom.Sub, atom.Sup, atom.Br:
		return true
	}
	return false
}

// attrValue returns the value of the named attribute on n, or "" when absent.
func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// textContent returns the concatenated text descendants of n, with no
// formatting preserved.
func textContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return sb.String()
}

// inlinesHaveVisibleContent reports whether at least one inline in the slice
// is a MergeTag or a non-empty Text run. Used by flushPendingInlines so a
// stretch of whitespace-only text nodes between two block elements does not
// produce a stray empty Paragraph.
func inlinesHaveVisibleContent(items []domain.Inline) bool {
	for _, it := range items {
		switch v := it.(type) {
		case domain.Text:
			if strings.TrimSpace(v.Text) != "" {
				return true
			}
		case domain.MergeTag:
			return true
		}
	}
	return false
}

// cssColorFromStyle extracts the `color: …;` value from a CSS style attribute,
// returning "" when no color rule is present. The lookup is conservative:
// only the literal `color` property is recognized, not `background-color`.
func cssColorFromStyle(style string) string {
	for _, decl := range strings.Split(style, ";") {
		colon := strings.IndexByte(decl, ':')
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(decl[:colon])
		if !strings.EqualFold(name, "color") {
			continue
		}
		return strings.TrimSpace(decl[colon+1:])
	}
	return ""
}
