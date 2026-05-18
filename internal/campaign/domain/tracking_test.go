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
