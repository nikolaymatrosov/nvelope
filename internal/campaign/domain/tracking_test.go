package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

func TestExtractLinks(t *testing.T) {
	t.Parallel()
	html := `<a href="https://acme.com/a">A</a> <a href="https://acme.com/b">B</a>` +
		` <a href="https://acme.com/a">A again</a> <a href="mailto:x@y.com">mail</a>`
	links := domain.ExtractLinks(html)
	require.Equal(t, []string{"https://acme.com/a", "https://acme.com/b"}, links,
		"distinct http(s) URLs, first-seen order, no mailto")
}

func TestRenderTracked(t *testing.T) {
	t.Parallel()
	html := `<p>Hi <a href="https://acme.com/sale">Shop</a></p>`
	linkIDs := map[string]string{"https://acme.com/sale": "link-1"}

	out := domain.RenderTracked(html, "https://track.test/", "camp-1", "rcpt-1", linkIDs)

	require.Contains(t, out, `href="https://track.test/l/link-1?s=rcpt-1"`,
		"the tracked link is rewritten")
	require.NotContains(t, out, "https://acme.com/sale", "the original URL is gone")
	require.Contains(t, out, `src="https://track.test/o/camp-1?s=rcpt-1"`,
		"the open pixel is appended")
	require.True(t, strings.HasSuffix(strings.TrimSpace(out), "/>"),
		"the pixel closes the body")
}

func TestRenderTrackedLeavesUnknownLinks(t *testing.T) {
	t.Parallel()
	html := `<a href="https://acme.com/unknown">x</a>`
	out := domain.RenderTracked(html, "https://track.test", "camp-1", "rcpt-1", map[string]string{})
	require.Contains(t, out, `href="https://acme.com/unknown"`,
		"a URL with no links row is left untouched")
}

func TestApplyHTMLVariablesEscapesDoubleBraces(t *testing.T) {
	t.Parallel()
	out, raw := domain.ApplyHTMLVariables(`<p>Hi {{name}}</p>`, map[string]string{"name": `<b>Sam</b> & co`})
	require.Equal(t, `<p>Hi &lt;b&gt;Sam&lt;/b&gt; &amp; co</p>`, out)
	require.False(t, raw)
}

func TestApplyHTMLVariablesInsertsTripleBracesRaw(t *testing.T) {
	t.Parallel()
	out, raw := domain.ApplyHTMLVariables(`<table>{{{rows}}}</table>`, map[string]string{"rows": `<tr><td>1</td></tr>`})
	require.Equal(t, `<table><tr><td>1</td></tr></table>`, out)
	require.True(t, raw)
}

func TestApplyVariablesIsSinglePass(t *testing.T) {
	t.Parallel()
	vars := map[string]string{"a": "{{b}}", "b": "leaked"}
	require.Equal(t, "x {{b}} y leaked", domain.ApplyVariables("x {{a}} y {{b}}", vars))
	out, _ := domain.ApplyHTMLVariables("{{a}}", vars)
	require.Equal(t, "{{b}}", out, "an inserted value is never rescanned")
}

func TestApplyVariablesKeepsUnknownKeysAndIsRawInText(t *testing.T) {
	t.Parallel()
	vars := map[string]string{"name": "Tom & Jerry"}
	require.Equal(t, "Hi Tom & Jerry {{missing}}", domain.ApplyVariables("Hi {{name}} {{missing}}", vars))
	require.Equal(t, "Hi Tom & Jerry", domain.ApplyVariables("Hi {{{name}}}", vars))
}

func TestExtractLinksDecodesEntities(t *testing.T) {
	t.Parallel()
	links := domain.ExtractLinks(`<a href="https://a.test/?x=1&amp;y=2">A</a>`)
	require.Equal(t, []string{"https://a.test/?x=1&y=2"}, links)
}

func TestRenderTrackedMatchesDecodedURL(t *testing.T) {
	t.Parallel()
	out := domain.RenderTracked(`<a href="https://a.test/?x=1&amp;y=2">A</a>`, "https://t.test",
		"c", "r", map[string]string{"https://a.test/?x=1&y=2": "link-1"})
	require.Contains(t, out, `href="https://t.test/l/link-1?s=r"`)
}
