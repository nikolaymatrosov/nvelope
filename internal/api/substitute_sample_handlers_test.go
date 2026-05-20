package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Sample-data substitution feeds rendered HTML/text + sample values through
// the canonical send-pipeline substituter. The BFF's render-preview route
// side-calls this endpoint so preview matches inbox.
func TestSubstituteSampleSubstitutesSubscriberAndCampaign(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	html := "<p>Hi {{ subscriber.first_name }}, visit {{ campaign.archive_url }}.</p>"
	text := "Hi {{ subscriber.first_name }}, visit {{ campaign.archive_url }}."

	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/substitute-sample", map[string]any{
		"html": html,
		"text": text,
		"sample": map[string]any{
			"subscriber": map[string]any{
				"first_name": "Sam",
				"email":      "sam@example.test",
			},
			"campaign": map[string]any{
				"archive_url": "https://example.test/a/abc",
			},
		},
	})
	require.Equal(t, http.StatusOK, status)
	outHTML, _ := body["html"].(string)
	outText, _ := body["text"].(string)
	require.True(t, strings.Contains(outHTML, "Hi Sam"), "subscriber placeholder substituted in html: %q", outHTML)
	require.True(t, strings.Contains(outHTML, "https://example.test/a/abc"),
		"campaign placeholder substituted in html: %q", outHTML)
	require.True(t, strings.Contains(outText, "Hi Sam"), "subscriber placeholder substituted in text: %q", outText)
}

func TestSubstituteSampleRejectsMissingBody(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/substitute-sample", map[string]any{
		"html":   "",
		"text":   "",
		"sample": map[string]any{},
	})
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "invalid_body", body["error"])
}

// Unknown subscriber slugs in the sample are silently ignored on this path —
// the doc itself was already validated by the BFF before render; this
// endpoint only resolves known placeholders against the supplied sample
// values (per contracts/tenant-api.md).
func TestSubstituteSampleLeavesUnknownSlugLiteral(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	html := "<p>{{ subscriber.unknown_slug }}</p>"
	text := "{{ subscriber.unknown_slug }}"

	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/substitute-sample", map[string]any{
		"html":   html,
		"text":   text,
		"sample": map[string]any{},
	})
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, html, body["html"], "unknown subscriber slug stays as the literal placeholder")
	require.Equal(t, text, body["text"])
}
