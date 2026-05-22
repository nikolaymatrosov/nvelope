package visualrender_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/visualrender"
)

// mediaPrefix is a tenant media-base URL stand-in used by the tests that
// exercise the MediaRefValidator branch in Convert. It mirrors the shape the
// real adapter validates against (an absolute https URL under a per-tenant
// prefix) without coupling these tests to the production config.
const mediaPrefix = "https://media.test/t/acme/"

type mediaPrefixValidator string

func (p mediaPrefixValidator) IsTenantMediaRef(ref string) bool {
	return strings.HasPrefix(ref, string(p))
}

func TestConvert_Paragraph(t *testing.T) {
	t.Parallel()
	doc, warnings, err := visualrender.Convert(`<p>hello world</p>`, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Empty(t, warnings)
	require.Len(t, doc.Nodes, 1)
	p, ok := doc.Nodes[0].(domain.Paragraph)
	require.True(t, ok)
	require.Len(t, p.Children, 1)
	assert.Equal(t, domain.Text{Text: "hello world"}, p.Children[0])
}

func TestConvert_HeadingsClampLevel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		html  string
		level int
	}{
		{"h1", "<h1>a</h1>", 1},
		{"h2", "<h2>a</h2>", 2},
		{"h3", "<h3>a</h3>", 3},
		{"h4 clamps to 3", "<h4>a</h4>", 3},
		{"h6 clamps to 3", "<h6>a</h6>", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doc, _, err := visualrender.Convert(tc.html, visualrender.ConvertOptions{})
			require.NoError(t, err)
			require.Len(t, doc.Nodes, 1)
			h, ok := doc.Nodes[0].(domain.Heading)
			require.True(t, ok)
			assert.Equal(t, tc.level, h.Level)
		})
	}
}

func TestConvert_BulletList(t *testing.T) {
	t.Parallel()
	doc, _, err := visualrender.Convert(`<ul><li>one</li><li>two</li></ul>`, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	list, ok := doc.Nodes[0].(domain.BulletList)
	require.True(t, ok)
	require.Len(t, list.Items, 2)
	// Each <li> wraps its text in a synthesized paragraph.
	first := list.Items[0].Children
	require.Len(t, first, 1)
	p, ok := first[0].(domain.Paragraph)
	require.True(t, ok)
	require.Len(t, p.Children, 1)
	assert.Equal(t, domain.Text{Text: "one"}, p.Children[0])
}

func TestConvert_OrderedList(t *testing.T) {
	t.Parallel()
	doc, _, err := visualrender.Convert(`<ol><li>one</li><li>two</li></ol>`, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	_, ok := doc.Nodes[0].(domain.OrderedList)
	require.True(t, ok)
}

func TestConvert_InlineLink(t *testing.T) {
	t.Parallel()
	doc, _, err := visualrender.Convert(
		`<p>visit <a href="https://example.test/x">here</a></p>`,
		visualrender.ConvertOptions{},
	)
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	p, ok := doc.Nodes[0].(domain.Paragraph)
	require.True(t, ok)
	require.Len(t, p.Children, 2)

	first, ok := p.Children[0].(domain.Text)
	require.True(t, ok)
	assert.Equal(t, "visit ", first.Text)
	assert.Equal(t, "", first.Marks.Link)

	link, ok := p.Children[1].(domain.Text)
	require.True(t, ok)
	assert.Equal(t, "here", link.Text)
	assert.Equal(t, "https://example.test/x", link.Marks.Link)
}

func TestConvert_TenantImage(t *testing.T) {
	t.Parallel()
	html := `<img src="` + mediaPrefix + `cat.png" alt="cat">`
	doc, warnings, err := visualrender.Convert(html, visualrender.ConvertOptions{
		MediaRefs: mediaPrefixValidator(mediaPrefix),
	})
	require.NoError(t, err)
	require.Empty(t, warnings)
	require.Len(t, doc.Nodes, 1)
	img, ok := doc.Nodes[0].(domain.Image)
	require.True(t, ok)
	assert.Equal(t, mediaPrefix+"cat.png", img.MediaRef)
	assert.Equal(t, "cat", img.Alt)
}

func TestConvert_NonTenantImageFallsBackToRawHTML(t *testing.T) {
	t.Parallel()
	html := `<img src="https://elsewhere.test/cat.png" alt="cat">`
	doc, warnings, err := visualrender.Convert(html, visualrender.ConvertOptions{
		MediaRefs: mediaPrefixValidator(mediaPrefix),
	})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	_, ok := doc.Nodes[0].(domain.RawHTML)
	require.True(t, ok)
	require.Len(t, warnings, 1)
	assert.Equal(t, "rawhtml_block", warnings[0].Kind)
}

func TestConvert_LinkedImage(t *testing.T) {
	t.Parallel()
	html := `<a href="https://example.test/cta"><img src="` + mediaPrefix + `cta.png" alt="cta"></a>`
	doc, _, err := visualrender.Convert(html, visualrender.ConvertOptions{
		MediaRefs: mediaPrefixValidator(mediaPrefix),
	})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	img, ok := doc.Nodes[0].(domain.Image)
	require.True(t, ok)
	assert.Equal(t, mediaPrefix+"cta.png", img.MediaRef)
	assert.Equal(t, "https://example.test/cta", img.Href)
}

func TestConvert_Hr(t *testing.T) {
	t.Parallel()
	doc, _, err := visualrender.Convert(`<hr>`, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	_, ok := doc.Nodes[0].(domain.Divider)
	require.True(t, ok)
}

func TestConvert_Blockquote(t *testing.T) {
	t.Parallel()
	doc, _, err := visualrender.Convert(`<blockquote><p>quoted</p></blockquote>`, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	q, ok := doc.Nodes[0].(domain.Quote)
	require.True(t, ok)
	require.Len(t, q.Children, 1)
	_, ok = q.Children[0].(domain.Paragraph)
	require.True(t, ok)
}

func TestConvert_CodeBlock(t *testing.T) {
	t.Parallel()
	doc, _, err := visualrender.Convert(`<pre>line one
line two</pre>`, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	code, ok := doc.Nodes[0].(domain.Code)
	require.True(t, ok)
	assert.Equal(t, "line one\nline two", code.Text)
}

func TestConvert_TwoColumnTable(t *testing.T) {
	t.Parallel()
	html := `<table><tr><td><p>left</p></td><td><p>right</p></td></tr></table>`
	doc, _, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	cols, ok := doc.Nodes[0].(domain.Columns)
	require.True(t, ok)
	require.Len(t, cols.Cols, 2)
	require.Len(t, cols.Cols[0], 1)
	require.Len(t, cols.Cols[1], 1)
}

func TestConvert_ThreeColumnTable(t *testing.T) {
	t.Parallel()
	html := `<table><tr><td>a</td><td>b</td><td>c</td></tr></table>`
	doc, _, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	cols, ok := doc.Nodes[0].(domain.Columns)
	require.True(t, ok)
	require.Len(t, cols.Cols, 3)
}

func TestConvert_FourColumnTable(t *testing.T) {
	t.Parallel()
	html := `<table><tr><td>a</td><td>b</td><td>c</td><td>d</td></tr></table>`
	doc, _, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	cols, ok := doc.Nodes[0].(domain.Columns)
	require.True(t, ok)
	require.Len(t, cols.Cols, 4)
}

func TestConvert_FiveColumnTableFallsBack(t *testing.T) {
	t.Parallel()
	html := `<table><tr><td>a</td><td>b</td><td>c</td><td>d</td><td>e</td></tr></table>`
	doc, warnings, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	_, ok := doc.Nodes[0].(domain.RawHTML)
	require.True(t, ok)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Detail, "column")
}

func TestConvert_ColspanFallsBack(t *testing.T) {
	t.Parallel()
	html := `<table><tr><td colspan="2">a</td><td>b</td></tr></table>`
	doc, warnings, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	_, ok := doc.Nodes[0].(domain.RawHTML)
	require.True(t, ok)
	assert.NotEmpty(t, warnings)
}

func TestConvert_RowspanFallsBack(t *testing.T) {
	t.Parallel()
	html := `<table><tr><td rowspan="2">a</td><td>b</td></tr><tr><td>c</td></tr></table>`
	doc, warnings, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	_, ok := doc.Nodes[0].(domain.RawHTML)
	require.True(t, ok)
	assert.NotEmpty(t, warnings)
}

func TestConvert_MultiRowTableFallsBack(t *testing.T) {
	t.Parallel()
	html := `<table><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></table>`
	doc, warnings, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	_, ok := doc.Nodes[0].(domain.RawHTML)
	require.True(t, ok)
	assert.NotEmpty(t, warnings)
}

func TestConvert_NestedTableFallsBack(t *testing.T) {
	t.Parallel()
	// Outer 2-col table where one cell hosts another <table>. The inner
	// table is recursively converted; the outer still resolves to a 2-col
	// Columns block. Each cell renders as one column whose content includes
	// either a paragraph or the nested table's converted result.
	html := `<table><tr><td><table><tr><td>inner-a</td><td>inner-b</td></tr></table></td><td><p>outer</p></td></tr></table>`
	doc, _, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	cols, ok := doc.Nodes[0].(domain.Columns)
	require.True(t, ok)
	require.Len(t, cols.Cols, 2)
}

func TestConvert_UnknownTagFallsBack(t *testing.T) {
	t.Parallel()
	html := `<custom-block>opaque</custom-block>`
	doc, warnings, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	raw, ok := doc.Nodes[0].(domain.RawHTML)
	require.True(t, ok)
	assert.Contains(t, raw.HTML, "opaque")
	require.Len(t, warnings, 1)
	assert.Equal(t, "rawhtml_block", warnings[0].Kind)
	assert.Equal(t, "nodes[0]", warnings[0].Path)
}

func TestConvert_DivContainerRecurses(t *testing.T) {
	t.Parallel()
	doc, _, err := visualrender.Convert(`<div><p>hello</p><p>world</p></div>`, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 2)
	_, ok := doc.Nodes[0].(domain.Paragraph)
	require.True(t, ok)
	_, ok = doc.Nodes[1].(domain.Paragraph)
	require.True(t, ok)
}

func TestConvert_MarksOnText(t *testing.T) {
	t.Parallel()
	html := `<p>hello <strong>bold</strong> and <em>italic</em> and <u>underline</u> and <s>strike</s></p>`
	doc, _, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	p, ok := doc.Nodes[0].(domain.Paragraph)
	require.True(t, ok)

	marks := map[string]domain.Marks{}
	for _, in := range p.Children {
		txt, ok := in.(domain.Text)
		if !ok {
			continue
		}
		marks[strings.TrimSpace(txt.Text)] = txt.Marks
	}
	assert.True(t, marks["bold"].Bold)
	assert.True(t, marks["italic"].Italic)
	assert.True(t, marks["underline"].Underline)
	assert.True(t, marks["strike"].Strike)
}

func TestConvert_ColorMarkFromStyle(t *testing.T) {
	t.Parallel()
	html := `<p>hi <span style="color: #cc0000; font-weight: 700">red</span></p>`
	doc, _, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	p, ok := doc.Nodes[0].(domain.Paragraph)
	require.True(t, ok)
	var found bool
	for _, in := range p.Children {
		if txt, ok := in.(domain.Text); ok && txt.Text == "red" {
			assert.Equal(t, "#cc0000", txt.Marks.Color)
			found = true
		}
	}
	assert.True(t, found, "expected to find the colored Text inline")
}

func TestConvert_MergeTagLiterals(t *testing.T) {
	t.Parallel()
	html := `<p>hello {{ subscriber.first_name }}, your unsubscribe link is {{ campaign.unsubscribe_url }}</p>`
	doc, _, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	p, ok := doc.Nodes[0].(domain.Paragraph)
	require.True(t, ok)

	var (
		subscriberMerge bool
		campaignMerge   bool
	)
	for _, in := range p.Children {
		if m, ok := in.(domain.MergeTag); ok {
			switch m.Namespace {
			case domain.MergeTagSubscriber:
				if m.Key == "first_name" {
					subscriberMerge = true
				}
			case domain.MergeTagCampaign:
				if m.Key == "unsubscribe_url" {
					campaignMerge = true
				}
			}
		}
	}
	assert.True(t, subscriberMerge, "expected MergeTag(subscriber.first_name)")
	assert.True(t, campaignMerge, "expected MergeTag(campaign.unsubscribe_url)")
}

func TestConvert_BareTextWrapsInParagraph(t *testing.T) {
	t.Parallel()
	doc, _, err := visualrender.Convert(`<body>just text</body>`, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Len(t, doc.Nodes, 1)
	p, ok := doc.Nodes[0].(domain.Paragraph)
	require.True(t, ok)
	require.Len(t, p.Children, 1)
	txt, ok := p.Children[0].(domain.Text)
	require.True(t, ok)
	assert.Equal(t, "just text", txt.Text)
}

func TestConvert_RoundTripStable(t *testing.T) {
	t.Parallel()
	// Convert returns the same shape on a second pass over its own rendered
	// markdown-flavored input. Because Convert is conservative — typed
	// blocks for the recognized vocabulary and RawHTML for everything else —
	// the second pass over an input made up only of recognized blocks
	// produces an identical document.
	html := `<h1>Newsletter</h1><p>Hello <strong>world</strong>.</p><ul><li>one</li><li>two</li></ul><hr><blockquote><p>a quote</p></blockquote>`
	first, w1, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Empty(t, w1)

	// Render `first` back to a serialized HTML approximation good enough
	// for a round-trip check. The test fakes the renderer by hand-writing
	// the equivalent HTML for each typed block — the goal is not to test
	// the BFF renderer (that lives in the frontend tests) but to confirm
	// Convert is idempotent on its own output shape.
	second, w2, err := visualrender.Convert(serializeForRoundTrip(first), visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Empty(t, w2)
	assert.Equal(t, first, second, "round-trip should produce the same VisualDoc")
}

// serializeForRoundTrip turns a VisualDoc back into HTML using a minimal,
// conservative emitter — enough to let TestConvert_RoundTripStable confirm
// Convert is stable on its own output without coupling to the BFF render
// module. The emitter only knows about the typed blocks that Convert
// produces, so any drift between them is by definition out of scope here.
func serializeForRoundTrip(doc *domain.VisualDoc) string {
	var sb strings.Builder
	for _, n := range doc.Nodes {
		emitBlock(&sb, n)
	}
	return sb.String()
}

func emitBlock(sb *strings.Builder, n domain.Node) {
	switch v := n.(type) {
	case domain.Paragraph:
		sb.WriteString("<p>")
		emitInlines(sb, v.Children)
		sb.WriteString("</p>")
	case domain.Heading:
		tag := "h" + string(rune('0'+v.Level))
		sb.WriteString("<" + tag + ">")
		emitInlines(sb, v.Children)
		sb.WriteString("</" + tag + ">")
	case domain.BulletList:
		sb.WriteString("<ul>")
		for _, it := range v.Items {
			sb.WriteString("<li>")
			for _, child := range it.Children {
				emitBlock(sb, child)
			}
			sb.WriteString("</li>")
		}
		sb.WriteString("</ul>")
	case domain.OrderedList:
		sb.WriteString("<ol>")
		for _, it := range v.Items {
			sb.WriteString("<li>")
			for _, child := range it.Children {
				emitBlock(sb, child)
			}
			sb.WriteString("</li>")
		}
		sb.WriteString("</ol>")
	case domain.Quote:
		sb.WriteString("<blockquote>")
		for _, child := range v.Children {
			emitBlock(sb, child)
		}
		sb.WriteString("</blockquote>")
	case domain.Divider:
		sb.WriteString("<hr>")
	}
}

func emitInlines(sb *strings.Builder, items []domain.Inline) {
	for _, in := range items {
		switch v := in.(type) {
		case domain.Text:
			if v.Marks.Bold {
				sb.WriteString("<strong>")
			}
			if v.Marks.Italic {
				sb.WriteString("<em>")
			}
			sb.WriteString(v.Text)
			if v.Marks.Italic {
				sb.WriteString("</em>")
			}
			if v.Marks.Bold {
				sb.WriteString("</strong>")
			}
		}
	}
}

func TestConvert_EmptyInput(t *testing.T) {
	t.Parallel()
	doc, warnings, err := visualrender.Convert("", visualrender.ConvertOptions{})
	require.NoError(t, err)
	require.Empty(t, warnings)
	assert.Empty(t, doc.Nodes)
}

func TestConvert_ValidatesAgainstFreshDoc(t *testing.T) {
	t.Parallel()
	// Convert's output for the recognized block vocabulary should satisfy
	// Validate without a FieldSet (the converter only produces
	// MergeTag nodes that came from literal `{{ subscriber.foo }}` text;
	// we pass a permissive FieldSet that accepts every slug).
	html := `<h1>Hello</h1><p>body <strong>bold</strong></p><hr>`
	doc, _, err := visualrender.Convert(html, visualrender.ConvertOptions{})
	require.NoError(t, err)
	err = domain.Validate(doc, domain.ValidateContext{Fields: allowAllFields{}})
	require.NoError(t, err)
}

type allowAllFields struct{}

func (allowAllFields) HasSlug(string) bool { return true }
