package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// --- T088: convert-to-visual and opt-out-visual integration tests -----------

// TestConvertCampaignToVisualHappyPath covers the basic flow: a legacy
// raw-HTML campaign's body_html is fed through the converter and the
// candidate VisualDoc + warnings come back without persisting anything.
// The fixture's body_html is a single `<p>placeholder</p>` so the
// converter produces one paragraph block and no warnings.
func TestConvertCampaignToVisualHappyPath(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, _ := ts.createDraftCampaign(slug)

	status, body := ts.request(http.MethodPost, base+"/campaigns/"+campaignID+"/convert-to-visual", nil)
	require.Equal(t, http.StatusOK, status)

	// The bodyDoc is the JSON pass-through of the converter's output. The
	// shape matches the contract: { version, type: "doc", content: [...] }.
	raw, ok := body["bodyDoc"]
	require.True(t, ok, "response carries a bodyDoc")
	require.NotNil(t, raw)
	// Round-trip back through json to assert the doc shape.
	encoded, err := json.Marshal(raw)
	require.NoError(t, err)
	var doc struct {
		Version int               `json:"version"`
		Type    string            `json:"type"`
		Content []json.RawMessage `json:"content"`
	}
	require.NoError(t, json.Unmarshal(encoded, &doc))
	require.Equal(t, 1, doc.Version)
	require.Equal(t, "doc", doc.Type)
	require.Len(t, doc.Content, 1, "single <p> converts to one block")

	warnings, _ := body["warnings"].([]any)
	require.Empty(t, warnings, "no rawhtml fallbacks for a clean <p>")

	// Convert is non-persisting — body_doc still NULL on the row.
	status, body = ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
	require.Equal(t, http.StatusOK, status)
	require.Nil(t, body["body_doc"], "convert does not persist; body_doc stays NULL")
}

// TestConvertCampaignToVisualRefusesWhenAlreadyVisual covers the
// "already_visual" 409 branch from contracts/tenant-api.md. A campaign
// whose body_doc is already set must reject conversion so the operator
// opens the visual editor directly instead of overwriting their structured
// edits with a re-conversion.
func TestConvertCampaignToVisualRefusesWhenAlreadyVisual(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, updatedAt := ts.createDraftCampaign(slug)

	// Promote the row to "visual" via the existing PUT /visual endpoint
	// (empty-doc payload is enough — the gate is body_doc IS NOT NULL).
	status, _ := ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "x",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>x</p>",
		"bodyText":          "x",
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)

	status, body := ts.request(http.MethodPost, base+"/campaigns/"+campaignID+"/convert-to-visual", nil)
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "already_visual", body["error"])
}

// TestOptOutCampaignVisualHappyPath covers FR-029: clearing body_doc on a
// row reverts it to a code-only campaign while body_html / body_text stay
// intact so the campaign remains sendable.
func TestOptOutCampaignVisualHappyPath(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, updatedAt := ts.createDraftCampaign(slug)

	// Save a visual document first.
	status, _ := ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "Visual subject",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>persisted html</p>",
		"bodyText":          "persisted text",
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)

	// Opt out — should succeed and return the updated row.
	status, body := ts.request(http.MethodPost, base+"/campaigns/"+campaignID+"/opt-out-visual", nil)
	require.Equal(t, http.StatusOK, status)
	require.Nil(t, body["body_doc"], "body_doc cleared after opt-out")
	require.Equal(t, "<p>persisted html</p>", body["body_html"],
		"body_html survives opt-out so the campaign stays sendable")
	require.Equal(t, "persisted text", body["body_text"])
}

// TestOptOutCampaignVisualIsIdempotent: a second opt-out on the same row
// is a no-op success — the SPA can retry safely without seeing an error.
func TestOptOutCampaignVisualIsIdempotent(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, _ := ts.createDraftCampaign(slug)

	// Row starts with NULL body_doc; opting out twice should both succeed.
	for i := range 2 {
		status, body := ts.request(http.MethodPost, base+"/campaigns/"+campaignID+"/opt-out-visual", nil)
		require.Equal(t, http.StatusOK, status, "opt-out attempt %d", i+1)
		require.Nil(t, body["body_doc"])
	}
}

// TestConvertTemplateToVisualHappyPath is the templates counterpart of the
// campaign convert test.
func TestConvertTemplateToVisualHappyPath(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	templateID, _ := ts.createTemplate(slug)

	status, body := ts.request(http.MethodPost, base+"/templates/"+templateID+"/convert-to-visual", nil)
	require.Equal(t, http.StatusOK, status)
	require.NotNil(t, body["bodyDoc"])
	warnings, _ := body["warnings"].([]any)
	require.Empty(t, warnings)

	// Non-persisting — body_doc still NULL on the row.
	status, body = ts.request(http.MethodGet, base+"/templates/"+templateID, nil)
	require.Equal(t, http.StatusOK, status)
	require.Nil(t, body["body_doc"])
}

// TestConvertTemplateToVisualRefusesWhenAlreadyVisual mirrors the campaign
// already_visual case.
func TestConvertTemplateToVisualRefusesWhenAlreadyVisual(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	templateID, updatedAt := ts.createTemplate(slug)

	// Promote the template to visual.
	status, _ := ts.request(http.MethodPut, base+"/templates/"+templateID+"/visual", map[string]any{
		"subject":           "x",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>x</p>",
		"bodyText":          "x",
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)

	status, body := ts.request(http.MethodPost, base+"/templates/"+templateID+"/convert-to-visual", nil)
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "already_visual", body["error"])
}

// TestOptOutTemplateVisualHappyPath: clearing body_doc on a template
// preserves body_html and body_text.
func TestOptOutTemplateVisualHappyPath(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	templateID, updatedAt := ts.createTemplate(slug)

	// Save visual content first.
	status, _ := ts.request(http.MethodPut, base+"/templates/"+templateID+"/visual", map[string]any{
		"subject":           "Visual subject",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>persisted html</p>",
		"bodyText":          "persisted text",
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)

	status, body := ts.request(http.MethodPost, base+"/templates/"+templateID+"/opt-out-visual", nil)
	require.Equal(t, http.StatusOK, status)
	require.Nil(t, body["body_doc"], "body_doc cleared after opt-out")
	require.Equal(t, "<p>persisted html</p>", body["body_html"])
	require.Equal(t, "persisted text", body["body_text"])
}
