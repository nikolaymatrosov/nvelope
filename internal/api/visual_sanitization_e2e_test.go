package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSaveVisualCampaignStripsScriptTagAndEmitsWarning (T118 — Go side)
// exercises the Phase 7 save-time sanitization gate end-to-end through the
// HTTP API. The BFF's job is to render a VisualDoc with a RawHTML block to
// HTML and forward the rendered bytes to Go; this test stands in for the BFF
// by submitting an already-rendered bodyHtml that includes a `<script>` tag
// inside RawHTML content. Go's bluemonday policy MUST strip the tag before
// persistence, AND the save response MUST surface a `sanitizer_stripped`
// warning so the operator can audit what was removed.
func TestSaveVisualCampaignStripsScriptTagAndEmitsWarning(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	domainID := ts.seedVerifiedDomain(slug, "sanitize.example.test")
	listID := ts.seedSubscribersOnList(slug, []string{"sanitize@example.com"})

	status, body := ts.request(http.MethodPost, base+"/campaigns", map[string]any{
		"name":              "Sanitized",
		"list_ids":          []string{listID},
		"sending_domain_id": domainID,
		"from_local_part":   "hello",
		"from_name":         "Sanitize Co",
	})
	require.Equal(t, http.StatusCreated, status, "create campaign: %v", body)
	campaignID := body["id"].(string)

	status, body = ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
	require.Equal(t, http.StatusOK, status)
	updatedAt := body["updated_at"].(string)

	// Emulate the BFF tier: it renders the operator's visual doc (which
	// contains a RawHTML block with a malicious `<script>` inside) to
	// finished HTML and forwards that HTML to Go. The Go-side sanitizer
	// must strip the script before persisting and warn the operator.
	rawHTML := `<p>Hello there.</p>` +
		`<script>document.cookie='stolen'</script>` +
		`<a href="https://example.com">Read more</a>`

	status, body = ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "Sanitize me",
		"bodyDoc":           map[string]any{"version": 1, "type": "doc", "nodes": []any{}, "theme": nil},
		"bodyHtml":          rawHTML,
		"bodyText":          "Hello there. Read more.",
		"theme":             nil,
		"ifUnmodifiedSince": updatedAt,
	})
	require.Equal(t, http.StatusOK, status, "save visual campaign: %v", body)

	// The response includes the structured warning so the SPA can render
	// an "X removed" notice next to the save toast.
	warnings, _ := body["warnings"].([]any)
	require.NotEmpty(t, warnings, "sanitizer warnings are surfaced to the caller")
	foundStrip := false
	for _, w := range warnings {
		wv, _ := w.(map[string]any)
		if kind, _ := wv["kind"].(string); kind == "sanitizer_stripped" {
			foundStrip = true
			break
		}
	}
	require.True(t, foundStrip,
		"a sanitizer_stripped warning is emitted (got: %v)", warnings)

	// The persisted campaign view's body_html no longer contains the script.
	status, body = ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
	require.Equal(t, http.StatusOK, status)
	persistedHTML, _ := body["body_html"].(string)
	require.NotContains(t, strings.ToLower(persistedHTML), "<script",
		"<script> is stripped from the persisted body")
	require.NotContains(t, persistedHTML, "document.cookie",
		"the script payload is gone too")
	require.Contains(t, persistedHTML, "Read more",
		"the safe `<a>` content survives sanitization")
}
