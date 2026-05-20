package visualrender

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// Negative tests for sanitizeHTML — for every disallowed construct in
// research.md § R5, the sanitizer must strip the construct (or refuse the
// attribute) AND surface the stripped=true signal so the renderer can warn.

func TestSanitize_StripsScriptTag(t *testing.T) {
	t.Parallel()
	out, stripped := sanitizeHTML(`<p>ok</p><script>alert(1)</script>`)
	require.True(t, stripped)
	require.NotContains(t, out, "<script")
	require.NotContains(t, out, "alert")
}

func TestSanitize_StripsStyleBlock(t *testing.T) {
	t.Parallel()
	out, stripped := sanitizeHTML(`<style>body{display:none}</style><p>ok</p>`)
	require.True(t, stripped)
	require.NotContains(t, out, "<style")
}

func TestSanitize_StripsIframeAndObject(t *testing.T) {
	t.Parallel()
	cases := []string{
		`<iframe src="https://evil.example.com"></iframe>`,
		`<object data="evil.swf"></object>`,
		`<embed src="evil.swf">`,
		`<form action="/x"><input name="x"></form>`,
		`<link rel="stylesheet" href="evil.css">`,
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			t.Parallel()
			out, stripped := sanitizeHTML(c)
			require.True(t, stripped, "must report stripped for %q", c)
			for _, tag := range []string{"<iframe", "<object", "<embed", "<form", "<input", "<link"} {
				require.NotContains(t, out, tag)
			}
		})
	}
}

func TestSanitize_StripsOnEventHandlers(t *testing.T) {
	t.Parallel()
	out, stripped := sanitizeHTML(`<a href="https://example.com" onclick="bad()">click</a>`)
	require.True(t, stripped)
	require.NotContains(t, out, "onclick")
}

func TestSanitize_StripsJavaScriptScheme(t *testing.T) {
	t.Parallel()
	out, stripped := sanitizeHTML(`<a href="javascript:alert(1)">click</a>`)
	require.True(t, stripped)
	require.NotContains(t, out, "javascript:")
}

func TestSanitize_StripsDataAndVBScript(t *testing.T) {
	t.Parallel()
	cases := []string{
		`<a href="vbscript:msgbox(1)">x</a>`,
		`<img src="data:image/png;base64,iVBOR…">`,
		`<a href="file:///etc/passwd">x</a>`,
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			t.Parallel()
			out, stripped := sanitizeHTML(c)
			require.True(t, stripped)
			require.NotRegexp(t, `(?i)(javascript|vbscript|data|file):`, out)
		})
	}
}

func TestSanitize_PreservesSafeAnchors(t *testing.T) {
	t.Parallel()
	in := `<a href="https://example.com">click</a>`
	out, stripped := sanitizeHTML(in)
	require.False(t, stripped, "clean input must not flag stripped")
	require.Contains(t, out, `href="https://example.com"`)
}

func TestSanitize_PreservesTables(t *testing.T) {
	t.Parallel()
	in := `<table role="presentation"><tr><td>x</td></tr></table>`
	out, _ := sanitizeHTML(in)
	require.Contains(t, out, "<table")
	require.Contains(t, out, "<td>x</td>")
}

func TestSanitize_NestedScriptInColumn(t *testing.T) {
	t.Parallel()
	// Defense in depth — disallowed constructs nested inside otherwise-OK
	// containers are still stripped.
	in := `<table><tr><td><script>x</script><p>visible</p></td></tr></table>`
	out, stripped := sanitizeHTML(in)
	require.True(t, stripped)
	require.NotContains(t, out, "<script")
	require.Contains(t, out, "<p>visible</p>")
}

func TestSanitize_NestedScriptInLink(t *testing.T) {
	t.Parallel()
	in := `<a href="https://example.com"><script>x</script>click</a>`
	out, stripped := sanitizeHTML(in)
	require.True(t, stripped)
	require.NotContains(t, out, "<script")
}

func TestSanitize_EmptyInput(t *testing.T) {
	t.Parallel()
	out, stripped := sanitizeHTML("")
	require.False(t, stripped)
	require.Equal(t, "", out)
}

func TestRawHTMLToText(t *testing.T) {
	t.Parallel()
	in := `<p>hello &amp; goodbye</p><br><span>more</span>`
	out := rawHTMLToText(in)
	require.Equal(t, "hello & goodbyemore", out)
}

// Ensure the renderer's whole-output guarantee holds: a RawHTML block
// containing a script never produces script in the final HTML, regardless
// of where it sits.
func TestRenderer_StripsScriptFromAnyRawHTMLPosition(t *testing.T) {
	t.Parallel()
	r := NewRenderer("https://media.nvelope.example/tenants/abc/")
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "before"}}},
		domain.RawHTML{HTML: `<p>safe</p><script>alert(1)</script>`},
		domain.Columns{Cols: [][]domain.Node{
			{domain.RawHTML{HTML: `<iframe src="evil"></iframe>`}},
			{domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "right"}}}},
		}},
	}}
	html, _, warnings, err := r.Render(doc, domain.DefaultsFromBranding("#0066cc"))
	require.NoError(t, err)
	require.NotContains(t, html, "<script")
	require.NotContains(t, strings.ToLower(html), "<iframe")
	require.NotEmpty(t, warnings)
}
