package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// emptyVisualDoc is the minimal doc shape that round-trips through Go's
// default JSON decoder for VisualDoc — interface-typed Nodes can't currently
// decode arbitrary block JSON without a custom UnmarshalJSON, so these
// integration tests cover the *handler*'s contract (validation, the
// FR-009 stale-row gate, permission gating) using the empty-doc payload.
// Render fidelity is asserted in the BFF render tests; the Go-side
// revalidation pass treats an empty Nodes slice as valid.
func emptyVisualDoc() map[string]any {
	return map[string]any{"version": 1, "nodes": []any{}}
}

// createDraftCampaign creates a draft campaign on the session-carrying client
// and returns its id and current updated_at.
func (ts *testServer) createDraftCampaign(slug string) (string, time.Time) {
	ts.t.Helper()
	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/campaigns", map[string]any{
		"name": "Visual draft", "subject": "Subject",
		"body_html": "<p>placeholder</p>", "body_text": "placeholder",
		"from_name": "Acme", "from_local_part": "news",
		"list_ids": []string{},
	})
	require.Equal(ts.t, http.StatusCreated, status)
	id := body["id"].(string)
	updatedRaw, _ := body["updated_at"].(string)
	updated, err := time.Parse(time.RFC3339Nano, updatedRaw)
	if err != nil {
		// Fall back to RFC3339 — Postgres timestamptz dropped trailing zeros.
		updated, err = time.Parse(time.RFC3339, updatedRaw)
	}
	require.NoError(ts.t, err, "campaign updated_at should be an RFC3339 timestamp: %q", updatedRaw)
	return id, updated
}

// --- T037: PUT /campaigns/{id}/visual integration tests ----------------------

func TestSaveVisualCampaignHappyPath(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, updatedAt := ts.createDraftCampaign(slug)

	status, body := ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "New subject",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>hello</p>",
		"bodyText":          "hello",
		"theme":             nil,
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)
	camp := body["campaign"].(map[string]any)
	require.Equal(t, "New subject", camp["subject"])
	require.Equal(t, "<p>hello</p>", camp["body_html"])
	require.Equal(t, "hello", camp["body_text"])
}

func TestSaveVisualCampaignRejectsMissingBodyHTML(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, updatedAt := ts.createDraftCampaign(slug)

	status, body := ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "Subject",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "",
		"bodyText":          "x",
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "invalid_body", body["error"])
}

func TestSaveVisualCampaignRejectsMissingIfUnmodifiedSince(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, _ := ts.createDraftCampaign(slug)

	status, body := ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":  "Subject",
		"bodyDoc":  emptyVisualDoc(),
		"bodyHtml": "<p>x</p>",
		"bodyText": "x",
		// ifUnmodifiedSince omitted.
	})
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "invalid_body", body["error"])
}

func TestSaveVisualCampaignRejectsMissingDoc(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, updatedAt := ts.createDraftCampaign(slug)

	status, body := ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "Subject",
		"bodyDoc":           nil,
		"bodyHtml":          "<p>x</p>",
		"bodyText":          "x",
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "invalid_body", body["error"])
}

// FR-009 stale-row gate: a save whose ifUnmodifiedSince does not match the
// row's current updated_at returns 409 stale_row with the row's
// currentUpdatedAt so the SPA can show the Reload / Force overwrite
// affordance.
func TestSaveVisualCampaignReturnsStaleRowOnMismatchedTimestamp(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, _ := ts.createDraftCampaign(slug)

	// A timestamp that cannot match the row's actual updated_at.
	stale := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	status, body := ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "Subject",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>x</p>",
		"bodyText":          "x",
		"ifUnmodifiedSince": stale.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "stale_row", body["error"])
	require.Equal(t, "stale_row", body["kind"])
	require.NotEmpty(t, body["currentUpdatedAt"], "current updated_at echoed for the Force-overwrite path")
}

// FR-034 (permission gating): a caller without campaigns:manage cannot save.
// Owner has campaigns:manage by default, so the test invites a viewer-role
// teammate (only campaigns:get) and asserts the save is refused.
func TestSaveVisualCampaignRejectsCallerWithoutManagePermission(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, updatedAt := ts.createDraftCampaign(slug)

	// Owner creates a read-only role and grants it to a new teammate.
	_, body := ts.request(http.MethodPost, base+"/roles", map[string]any{
		"name": "VisualViewer", "permissions": []string{"campaigns:get"},
	})
	roleID := body["id"].(string)

	member, memberEmail := ts.addMember(slug)
	// First workspace open provisions the tenant-plane user row.
	ts.enterWorkspaceOn(member, slug)
	memberID := ts.workspaceUserID(slug, memberEmail)
	status, _ := ts.request(http.MethodPut, base+"/users/"+memberID+"/role",
		map[string]any{"role_id": roleID})
	require.Equal(t, http.StatusNoContent, status)
	status, body = ts.do(member, http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "Subject",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>x</p>",
		"bodyText":          "x",
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusForbidden, status)
	require.Contains(t, body["error"], "forbidden", "error slug is a forbidden variant")
}

// --- T128: two-client concurrent-save flow test ------------------------------

// Open row R from client 1, open from client 2, save from client 2 (succeeds,
// updated_at advances), save from client 1 with the stale ifUnmodifiedSince
// — assert 409 stale_row with the new currentUpdatedAt in the payload, assert
// client 1's save did not change the row, then re-issue client 1's save with
// the response's currentUpdatedAt (the Force-overwrite path) and assert
// success.
func TestSaveVisualCampaignConcurrentSavesForceOverwriteFlow(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	campaignID, initialUpdatedAt := ts.createDraftCampaign(slug)

	// Both editor sessions load the row at the same updated_at — simulated
	// here by both saves using initialUpdatedAt for their first try.
	client1Stamp := initialUpdatedAt
	client2Stamp := initialUpdatedAt

	// Client 2 saves first and wins.
	status, _ := ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "From client 2",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>client 2</p>",
		"bodyText":          "client 2",
		"ifUnmodifiedSince": client2Stamp.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)

	// Client 1 races in with its stale timestamp — must be rejected.
	status, body := ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "From client 1",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>client 1</p>",
		"bodyText":          "client 1",
		"ifUnmodifiedSince": client1Stamp.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "stale_row", body["error"])
	currentRaw, _ := body["currentUpdatedAt"].(string)
	require.NotEmpty(t, currentRaw, "currentUpdatedAt is returned so the SPA can adopt it")
	current, err := time.Parse(time.RFC3339Nano, currentRaw)
	if err != nil {
		current, err = time.Parse(time.RFC3339, currentRaw)
	}
	require.NoError(t, err)

	// The row still holds client 2's content (client 1's first save did not
	// reach the database).
	status, body = ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "From client 2", body["subject"])
	require.Equal(t, "<p>client 2</p>", body["body_html"])

	// Force-overwrite: client 1 retries with the new currentUpdatedAt and
	// wins this time.
	status, body = ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "From client 1 (forced)",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>client 1 forced</p>",
		"bodyText":          "client 1 forced",
		"ifUnmodifiedSince": current.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)
	camp := body["campaign"].(map[string]any)
	require.Equal(t, "From client 1 (forced)", camp["subject"])
	require.Equal(t, "<p>client 1 forced</p>", camp["body_html"])
}
