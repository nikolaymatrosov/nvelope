package visualrender_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters/visualrender"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// fixtureTheme is the canonical theme golden tests render against. It is
// stable so byte-for-byte assertions on the produced HTML remain stable.
func fixtureTheme() domain.Theme {
	return domain.DefaultsFromBranding("#0066cc")
}

const mediaPrefix = "https://media.nvelope.example/tenants/abc/"

func mustRender(t *testing.T, doc *domain.VisualDoc) (htmlOut, textOut string, warnings []domain.RenderWarning) {
	t.Helper()
	r := visualrender.NewRenderer(mediaPrefix)
	h, txt, w, err := r.Render(doc, fixtureTheme())
	require.NoError(t, err)
	return h, txt, w
}

// shellSuffix is the closing portion of the outer container the renderer
// always emits. The opening portion is asserted by prefix in TestRender_Shell.
const shellSuffix = `</td></tr></table>`

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain:\n\t%s\n\nfull output:\n%s", want, got)
	}
}

func TestRender_Paragraph(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "Hello, world."}}},
	}}
	got, text, _ := mustRender(t, doc)
	assertContains(t, got, `<p style="margin:0 0 16px 0;line-height:1.5;">Hello, world.</p>`)
	require.Equal(t, "Hello, world.\n", text)
}

func TestRender_HeadingsAllThreeLevels(t *testing.T) {
	t.Parallel()
	for _, lvl := range []int{1, 2, 3} {
		doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
			domain.Heading{Level: lvl, Children: []domain.Inline{domain.Text{Text: "Title"}}},
		}}
		got, _, _ := mustRender(t, doc)
		assertContains(t, got, "<h"+itoa(lvl)+" ")
		assertContains(t, got, ">Title</h"+itoa(lvl)+">")
	}
}

func TestRender_BulletAndOrderedList(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.BulletList{Items: []domain.ListItem{
			{Children: []domain.Node{domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "one"}}}}},
			{Children: []domain.Node{domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "two"}}}}},
		}},
		domain.OrderedList{Items: []domain.ListItem{
			{Children: []domain.Node{domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "first"}}}}},
		}},
	}}
	got, text, _ := mustRender(t, doc)
	assertContains(t, got, `<ul style="margin:0 0 16px 24px;padding:0;">`)
	assertContains(t, got, `<li style="margin:0 0 4px 0;">one</li>`)
	assertContains(t, got, `<ol style="margin:0 0 16px 24px;padding:0;">`)
	require.Contains(t, text, "- one\n")
	require.Contains(t, text, "1. first\n")
}

func TestRender_Quote(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Quote{Children: []domain.Node{
			domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "quoted"}}},
		}},
	}}
	got, _, _ := mustRender(t, doc)
	assertContains(t, got, `<blockquote `)
	assertContains(t, got, `>quoted</p></blockquote>`)
}

func TestRender_Code(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Code{Text: "if x < 1 { return }"},
	}}
	got, text, _ := mustRender(t, doc)
	assertContains(t, got, `<code>if x &lt; 1 { return }</code>`)
	require.Equal(t, "if x < 1 { return }\n", text)
}

func TestRender_LinkMark(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Paragraph{Children: []domain.Inline{
			domain.Text{Text: "click", Marks: domain.Marks{Link: "https://example.com"}},
		}},
	}}
	got, text, _ := mustRender(t, doc)
	assertContains(t, got, `<a href="https://example.com"`)
	assertContains(t, got, ">click</a>")
	require.Contains(t, text, "click (https://example.com)")
}

func TestRender_Image(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Image{MediaRef: mediaPrefix + "logo.png", Alt: "Logo"},
	}}
	got, text, _ := mustRender(t, doc)
	assertContains(t, got, `<img src="`+mediaPrefix+`logo.png" alt="Logo"`)
	require.Contains(t, text, "[image: Logo]")
}

func TestRender_Button(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Button{Label: "Read more", Href: "https://example.com/post/42"},
	}}
	got, text, _ := mustRender(t, doc)
	assertContains(t, got, `<table role="presentation"`)
	assertContains(t, got, `href="https://example.com/post/42"`)
	assertContains(t, got, ">Read more</a>")
	require.Contains(t, text, "[ Read more ] (https://example.com/post/42)")
}

func TestRender_Divider(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{domain.Divider{}}}
	got, text, _ := mustRender(t, doc)
	assertContains(t, got, `<hr style="margin:16px 0;border:0;border-top:1px solid #dddddd;">`)
	require.Contains(t, text, "----")
}

func TestRender_Columns_2_3_4(t *testing.T) {
	t.Parallel()
	for _, count := range []int{2, 3, 4} {
		cols := make([][]domain.Node, count)
		for i := range cols {
			cols[i] = []domain.Node{
				domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "col"}}},
			}
		}
		doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{domain.Columns{Cols: cols}}}
		got, _, _ := mustRender(t, doc)
		// every column should appear with width:N% where N=100/count
		pct := 100 / count
		assertContains(t, got, `width:`+itoa(pct)+`%;`)
	}
}

func TestRender_MergeTag(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Paragraph{Children: []domain.Inline{
			domain.Text{Text: "Hello, "},
			domain.MergeTag{Namespace: domain.MergeTagSubscriber, Key: "first_name"},
			domain.Text{Text: "!"},
		}},
	}}
	got, text, _ := mustRender(t, doc)
	assertContains(t, got, "Hello, {{ subscriber.first_name }}!")
	require.Equal(t, "Hello, {{ subscriber.first_name }}!\n", text)
}

func TestRender_RawHTML_StripsScript(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.RawHTML{HTML: `<p>safe</p><script>alert(1)</script>`},
	}}
	got, _, warnings := mustRender(t, doc)
	require.NotContains(t, got, "<script", "script must be stripped")
	assertContains(t, got, "<p>safe</p>")
	require.NotEmpty(t, warnings)
	require.Equal(t, domain.ErrSanitizationStripped.Slug(), warnings[0].Kind)
}

func TestRender_Shell(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "ok"}}},
	}}
	got, _, _ := mustRender(t, doc)
	require.True(t, strings.HasPrefix(got, `<table role="presentation" width="600"`))
	require.True(t, strings.HasSuffix(got, shellSuffix))
}

func TestRender_TextMarks_Bold_Italic(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Paragraph{Children: []domain.Inline{
			domain.Text{Text: "B", Marks: domain.Marks{Bold: true}},
			domain.Text{Text: "I", Marks: domain.Marks{Italic: true}},
		}},
	}}
	got, _, _ := mustRender(t, doc)
	assertContains(t, got, `<strong>B</strong>`)
	assertContains(t, got, `<em>I</em>`)
}

func TestRender_NilDocReturnsError(t *testing.T) {
	t.Parallel()
	r := visualrender.NewRenderer(mediaPrefix)
	_, _, _, err := r.Render(nil, fixtureTheme())
	require.Error(t, err)
}

func TestRenderer_IsTenantMediaRef(t *testing.T) {
	t.Parallel()
	r := visualrender.NewRenderer(mediaPrefix)
	require.True(t, r.IsTenantMediaRef(mediaPrefix+"logo.png"))
	require.False(t, r.IsTenantMediaRef("https://evil.example.com/x.png"))
	require.False(t, r.IsTenantMediaRef(""))
}

// itoa is a tiny dependency-free int-to-string for tests. avoiding strconv
// import for clarity in the table loop.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
