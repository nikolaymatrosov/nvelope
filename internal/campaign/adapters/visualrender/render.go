// Package visualrender renders a campaign or template's structured visual
// document into the email-ready HTML + plain text the send pipeline consumes.
//
// It is a synchronous, stateless adapter that walks the typed VisualDoc tree
// in one pass and emits inline-styled, table-based HTML following the block
// strategy documented in specs/014-visual-email-editor/research.md § R4.
// RawHTML blocks are sanitized in isolation as they are emitted (per § R5).
//
// The Renderer also implements domain.MediaRefValidator so callers can route
// the same media-URL recognizer through the doc-validation path.
package visualrender

import (
	"fmt"
	"html"
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// Renderer is the in-house VisualDoc → email-HTML + plain-text renderer.
type Renderer struct {
	// mediaURLPrefix is the prefix every tenant media-library URL begins
	// with (e.g. "https://media.nvelope.example/tenants/"). An Image block
	// whose MediaRef does not begin with it is rejected at save time.
	mediaURLPrefix string
}

// NewRenderer builds a Renderer over the supplied media-library URL prefix.
func NewRenderer(mediaURLPrefix string) *Renderer {
	return &Renderer{mediaURLPrefix: strings.TrimSpace(mediaURLPrefix)}
}

var (
	_ domain.Renderer          = (*Renderer)(nil)
	_ domain.MediaRefValidator = (*Renderer)(nil)
)

// IsTenantMediaRef reports whether ref is rooted in the tenant media prefix.
func (r *Renderer) IsTenantMediaRef(ref string) bool {
	if r.mediaURLPrefix == "" {
		return false
	}
	return strings.HasPrefix(ref, r.mediaURLPrefix)
}

// Render walks doc and emits the canonical email-ready HTML + plain text. It
// returns the warnings accumulated during sanitization (e.g. content stripped
// from a RawHTML block).
func (r *Renderer) Render(doc *domain.VisualDoc, theme domain.Theme) (string, string, []domain.RenderWarning, error) {
	if doc == nil {
		return "", "", nil, domain.ErrVisualDocInvalid.WithMessage("nil document")
	}

	var htmlBuf, textBuf strings.Builder
	warnings := []domain.RenderWarning{}

	// Outer container: a single-column wrapper table sized to the theme's
	// container width. Inline-styled so Gmail / Outlook clipping rules
	// don't drop it.
	fmt.Fprintf(&htmlBuf,
		`<table role="presentation" width="%d" cellpadding="0" cellspacing="0" border="0" style="width:%dpx;max-width:100%%;font-family:%s;color:%s;"><tr><td>`,
		theme.ContainerWidth(), theme.ContainerWidth(),
		htmlAttr(theme.FontFamily()), htmlAttr(theme.TextColor()),
	)

	for i, n := range doc.Nodes {
		if i > 0 {
			textBuf.WriteString("\n")
		}
		r.renderBlock(&htmlBuf, &textBuf, n, theme, &warnings)
	}

	htmlBuf.WriteString(`</td></tr></table>`)

	return htmlBuf.String(), strings.TrimRight(textBuf.String(), "\n") + "\n", warnings, nil
}

// renderBlock dispatches one block node into both buffers.
func (r *Renderer) renderBlock(hb, tb *strings.Builder, n domain.Node, theme domain.Theme, warnings *[]domain.RenderWarning) {
	switch v := n.(type) {
	case domain.Paragraph:
		hb.WriteString(`<p style="margin:0 0 16px 0;line-height:1.5;">`)
		r.renderInlines(hb, tb, v.Children, theme)
		hb.WriteString(`</p>`)
		tb.WriteString("\n")
	case domain.Heading:
		level := v.Level
		if level < 1 || level > 3 {
			level = 1
		}
		size := headingSize(level)
		fmt.Fprintf(hb, `<h%d style="margin:0 0 16px 0;font-size:%dpx;line-height:1.25;">`, level, size)
		r.renderInlines(hb, tb, v.Children, theme)
		fmt.Fprintf(hb, `</h%d>`, level)
		tb.WriteString("\n")
	case domain.BulletList:
		hb.WriteString(`<ul style="margin:0 0 16px 24px;padding:0;">`)
		for _, it := range v.Items {
			r.renderListItem(hb, tb, it, theme, warnings, "- ")
		}
		hb.WriteString(`</ul>`)
	case domain.OrderedList:
		hb.WriteString(`<ol style="margin:0 0 16px 24px;padding:0;">`)
		for i, it := range v.Items {
			prefix := fmt.Sprintf("%d. ", i+1)
			r.renderListItem(hb, tb, it, theme, warnings, prefix)
		}
		hb.WriteString(`</ol>`)
	case domain.Quote:
		hb.WriteString(`<blockquote style="margin:0 0 16px 0;padding:0 0 0 12px;border-left:3px solid #cccccc;color:#555555;">`)
		for _, child := range v.Children {
			r.renderBlock(hb, tb, child, theme, warnings)
		}
		hb.WriteString(`</blockquote>`)
	case domain.Code:
		hb.WriteString(`<pre style="margin:0 0 16px 0;padding:12px;background:#f4f4f4;border-radius:4px;font-family:monospace;font-size:13px;line-height:1.4;overflow:auto;"><code>`)
		hb.WriteString(html.EscapeString(v.Text))
		hb.WriteString(`</code></pre>`)
		tb.WriteString(v.Text)
		tb.WriteString("\n")
	case domain.Image:
		alt := htmlAttr(v.Alt)
		src := htmlAttr(v.MediaRef)
		imgTag := fmt.Sprintf(`<img src="%s" alt="%s" style="display:block;max-width:100%%;height:auto;border:0;">`, src, alt)
		if v.Href != "" {
			fmt.Fprintf(hb, `<a href="%s" style="display:inline-block;">%s</a>`, htmlAttr(v.Href), imgTag)
		} else {
			hb.WriteString(imgTag)
		}
		if v.Alt != "" {
			fmt.Fprintf(tb, "[image: %s]\n", v.Alt)
		} else {
			tb.WriteString("[image]\n")
		}
	case domain.Button:
		fmt.Fprintf(hb,
			`<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:0 0 16px 0;"><tr><td style="background:%s;border-radius:4px;"><a href="%s" style="display:inline-block;padding:10px 20px;color:%s;text-decoration:none;font-weight:600;">%s</a></td></tr></table>`,
			htmlAttr(theme.ButtonColor()), htmlAttr(v.Href),
			htmlAttr(theme.ButtonTextColor()), html.EscapeString(v.Label),
		)
		fmt.Fprintf(tb, "[ %s ] (%s)\n", v.Label, v.Href)
	case domain.Divider:
		hb.WriteString(`<hr style="margin:16px 0;border:0;border-top:1px solid #dddddd;">`)
		tb.WriteString("----\n")
	case domain.Columns:
		hb.WriteString(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="width:100%;margin:0 0 16px 0;"><tr>`)
		colCount := len(v.Cols)
		if colCount == 0 {
			colCount = 1
		}
		colPct := 100 / colCount
		for ci, col := range v.Cols {
			fmt.Fprintf(hb, `<td valign="top" style="width:%d%%;padding:0 8px;">`, colPct)
			for _, child := range col {
				r.renderBlock(hb, tb, child, theme, warnings)
			}
			hb.WriteString(`</td>`)
			if ci < len(v.Cols)-1 {
				tb.WriteString("\n")
			}
		}
		hb.WriteString(`</tr></table>`)
	case domain.RawHTML:
		clean, stripped := sanitizeHTML(v.HTML)
		hb.WriteString(clean)
		if stripped {
			*warnings = append(*warnings, domain.RenderWarning{
				Kind:   domain.ErrSanitizationStripped.Slug(),
				Detail: "sanitizer removed disallowed content from a RawHTML block",
			})
		}
		tb.WriteString(rawHTMLToText(clean))
		tb.WriteString("\n")
	}
}

// renderListItem emits one list item into both buffers. The first child's
// inline content (if it's a paragraph) is rendered as the item's main text;
// further children render as nested blocks inside the <li>.
func (r *Renderer) renderListItem(hb, tb *strings.Builder, it domain.ListItem, theme domain.Theme, warnings *[]domain.RenderWarning, textPrefix string) {
	hb.WriteString(`<li style="margin:0 0 4px 0;">`)
	tb.WriteString(textPrefix)
	for i, child := range it.Children {
		switch c := child.(type) {
		case domain.Paragraph:
			// Inline children rendered directly inside the <li> for the first
			// paragraph so we don't emit a nested <p> with margins.
			if i == 0 {
				r.renderInlines(hb, tb, c.Children, theme)
			} else {
				r.renderBlock(hb, tb, child, theme, warnings)
			}
		default:
			r.renderBlock(hb, tb, child, theme, warnings)
		}
	}
	hb.WriteString(`</li>`)
	tb.WriteString("\n")
}

// renderInlines emits a stream of inline nodes (text runs and merge tags).
func (r *Renderer) renderInlines(hb, tb *strings.Builder, items []domain.Inline, theme domain.Theme) {
	for _, in := range items {
		switch v := in.(type) {
		case domain.Text:
			r.renderTextRun(hb, tb, v, theme)
		case domain.MergeTag:
			lit := fmt.Sprintf("{{ %s.%s }}", string(v.Namespace), v.Key)
			hb.WriteString(lit)
			tb.WriteString(lit)
		}
	}
}

// renderTextRun emits a Text run honoring its Marks. Marks nest in a fixed
// outer→inner order so the closing tags stack symmetrically: link → color →
// bold → italic → underline → strike → text.
func (r *Renderer) renderTextRun(hb, tb *strings.Builder, t domain.Text, theme domain.Theme) {
	escaped := html.EscapeString(t.Text)
	var openTags, closeTags strings.Builder
	push := func(o, c string) {
		openTags.WriteString(o)
		// closeTags accumulates in reverse so we can write it as-is at the end.
		closeTags.WriteString(c)
	}
	closes := []string{}
	pushOrdered := func(o, c string) {
		openTags.WriteString(o)
		closes = append([]string{c}, closes...)
	}
	_ = push // retained for symmetry; not used in the ordered path below
	if t.Marks.Link != "" {
		pushOrdered(
			fmt.Sprintf(`<a href="%s" style="color:%s;text-decoration:underline;">`,
				htmlAttr(t.Marks.Link), htmlAttr(theme.LinkColor())),
			`</a>`,
		)
	}
	if c := strings.TrimSpace(t.Marks.Color); c != "" {
		pushOrdered(fmt.Sprintf(`<span style="color:%s;">`, htmlAttr(c)), `</span>`)
	}
	if t.Marks.Bold {
		pushOrdered(`<strong>`, `</strong>`)
	}
	if t.Marks.Italic {
		pushOrdered(`<em>`, `</em>`)
	}
	if t.Marks.Underline {
		pushOrdered(`<u>`, `</u>`)
	}
	if t.Marks.Strike {
		pushOrdered(`<s>`, `</s>`)
	}
	for _, c := range closes {
		closeTags.WriteString(c)
	}
	hb.WriteString(openTags.String())
	hb.WriteString(escaped)
	hb.WriteString(closeTags.String())
	tb.WriteString(t.Text)
	if t.Marks.Link != "" {
		fmt.Fprintf(tb, " (%s)", t.Marks.Link)
	}
}

// headingSize returns the px font-size for a heading level.
func headingSize(level int) int {
	switch level {
	case 1:
		return 28
	case 2:
		return 22
	case 3:
		return 18
	}
	return 16
}

// htmlAttr escapes a string for use as an HTML attribute value. Empty input
// returns empty output.
func htmlAttr(s string) string {
	return html.EscapeString(s)
}
