package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// T106 — theme inheritance vs. pinned-override persistence (FR-022 / FR-024).
//
// Two complementary assertions, both verified end-to-end through the HTTP
// boundary because that's where the BFF → Go round-trip lands:
//
//   1. Saving a visual campaign with `theme: null` stores NULL in the row's
//      theme column. The read view reflects that as an absent `theme` field
//      (the read-model JSON tag is `omitempty`), so a future render at
//      branding-change time picks up the new tenant defaults — that's the
//      "unpinned campaigns track tenant branding" behaviour from FR-022.
//
//   2. Saving with an explicit theme override stores the operator's pinned
//      bytes verbatim. The read view returns those same bytes; a subsequent
//      tenant-branding change does NOT clobber them. That's the
//      "pinned overrides survive branding changes" half of FR-023 / FR-024.
//
// The Go API does no branding resolution itself (per plan.md US3 Constitution
// re-check) — that lives in the BFF. The Go-side guarantee under test here
// is only the persistence shape; the BFF integration is covered separately
// in frontend/src/server/routes/visual-save.test.ts.

func TestSaveVisualCampaignPersistsNullTheme(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, updatedAt := ts.createDraftCampaign(slug)

	status, _ := ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "Inherits branding",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>x</p>",
		"bodyText":          "x",
		"theme":             nil,
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)

	// Read the row back. The `theme` field is `omitempty` on the read view,
	// so a nil column drops the key entirely — that's exactly the marker the
	// BFF observes on a future save to decide it must fetch branding again.
	status, body := ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
	require.Equal(t, http.StatusOK, status)
	_, present := body["theme"]
	require.False(t, present,
		"a null theme must persist as NULL so the row tracks tenant branding on the next render")
}

func TestSaveVisualCampaignPersistsPinnedThemeBytes(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, updatedAt := ts.createDraftCampaign(slug)

	pinned := map[string]any{
		"textColor":       "#222222",
		"linkColor":       "#cc3366",
		"buttonColor":     "#cc3366",
		"buttonTextColor": "#ffffff",
		"fontFamily":      "'Inter', sans-serif",
		"containerWidth":  600,
	}

	status, saveBody := ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "Pinned theme",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>x</p>",
		"bodyText":          "x",
		"theme":             pinned,
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)
	require.NotNil(t, saveBody["updatedAt"], "the save response echoes the new updated_at")

	// Read back and assert the pinned bytes survived round-trip.
	status, body := ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
	require.Equal(t, http.StatusOK, status)
	theme, ok := body["theme"].(map[string]any)
	require.True(t, ok, "theme is present and an object: %#v", body["theme"])
	require.Equal(t, pinned["textColor"], theme["textColor"])
	require.Equal(t, pinned["linkColor"], theme["linkColor"])
	require.Equal(t, pinned["buttonColor"], theme["buttonColor"])
	require.Equal(t, pinned["buttonTextColor"], theme["buttonTextColor"])
	require.Equal(t, pinned["fontFamily"], theme["fontFamily"])
	require.EqualValues(t, 600, theme["containerWidth"])
}

// A round-trip from pinned → unpinned must clear the theme bytes. Otherwise
// an operator who pinned an override and then chose "use tenant defaults"
// again would silently keep the old override (the editor would show an
// inherit indicator but the renderer would still see the JSON).
func TestSaveVisualCampaignClearingPinnedThemeReturnsToInheritedState(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, updatedAt := ts.createDraftCampaign(slug)

	// First save: pin an override.
	pinned := map[string]any{
		"textColor":       "#222222",
		"linkColor":       "#aa0044",
		"buttonColor":     "#aa0044",
		"buttonTextColor": "#ffffff",
		"fontFamily":      "'Inter', sans-serif",
		"containerWidth":  640,
	}
	status, _ := ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "Pinned",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>x</p>",
		"bodyText":          "x",
		"theme":             pinned,
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)

	// Re-fetch updated_at for the second save's optimistic-concurrency gate.
	status, body := ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
	require.Equal(t, http.StatusOK, status)
	updatedRaw, _ := body["updated_at"].(string)
	updated, err := time.Parse(time.RFC3339Nano, updatedRaw)
	if err != nil {
		updated, err = time.Parse(time.RFC3339, updatedRaw)
	}
	require.NoError(t, err)

	// Second save: theme back to null. The row's theme column must drop the
	// pinned bytes so the BFF resolves branding again on the next render.
	status, _ = ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "Unpinned again",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>x</p>",
		"bodyText":          "x",
		"theme":             nil,
		"ifUnmodifiedSince": updated.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)

	status, body = ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
	require.Equal(t, http.StatusOK, status)
	_, present := body["theme"]
	require.False(t, present, "unpinning the theme must drop the persisted bytes")
}
