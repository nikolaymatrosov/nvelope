package visualrender

import (
	"testing"

	"github.com/stretchr/testify/require"
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

// T009 — per-block style (feature 017). Every BlockStyle-producible inline-CSS
// property survives the sanitizer, while a hostile property is dropped.

func TestSanitize_PreservesBlockStyleProperties(t *testing.T) {
	t.Parallel()
	in := `<td style="background-color:#1a73e8;color:#ffffff;` +
		`font-family:Arial, Helvetica, sans-serif;font-size:16px;font-weight:700;` +
		`line-height:1.5;text-align:center;padding-top:12px;padding-right:20px;` +
		`padding-bottom:12px;padding-left:20px;border-radius:8px;border-width:1px;` +
		`border-style:solid;border-color:#0b57d0;padding:4px">hi</td>`
	out, _ := sanitizeHTML(in)
	for _, prop := range []string{
		"background-color", "color", "font-family", "font-size", "font-weight",
		"line-height", "text-align", "padding-top", "padding-right",
		"padding-bottom", "padding-left", "border-radius", "border-width",
		"border-style", "border-color",
	} {
		require.Contains(t, out, prop, "BlockStyle property %q must survive", prop)
	}
}

func TestSanitize_DropsHostileStyleProperties(t *testing.T) {
	t.Parallel()
	cases := []string{
		`<td style="position:absolute;color:#fff">x</td>`,
		`<td style="behavior:url(x.htc);color:#fff">x</td>`,
		`<td style="width:expression(alert(1));color:#fff">x</td>`,
		`<td style="-moz-binding:url(x);color:#fff">x</td>`,
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			t.Parallel()
			out, _ := sanitizeHTML(c)
			// The allow-listed declaration survives…
			require.Contains(t, out, "color")
			// …but the hostile property is dropped.
			require.NotRegexp(t, `(?i)(position|behavior|expression|-moz-binding)`, out)
		})
	}
}

func TestRawHTMLToText(t *testing.T) {
	t.Parallel()
	in := `<p>hello &amp; goodbye</p><br><span>more</span>`
	out := rawHTMLToText(in)
	require.Equal(t, "hello & goodbyemore", out)
}
