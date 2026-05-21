package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// createTemplate creates a campaign-kind template via the existing
// raw-HTML path and returns its id and current updated_at — the
// pre-Phase-7 fixture every visual-save test starts from.
func (ts *testServer) createTemplate(slug string) (string, time.Time) {
	ts.t.Helper()
	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/templates", map[string]any{
		"name": "Welcome series", "kind": "campaign", "subject": "Subject",
		"body_html": "<p>placeholder</p>", "body_text": "placeholder",
	})
	require.Equal(ts.t, http.StatusCreated, status)
	id := body["id"].(string)
	updatedRaw, _ := body["updated_at"].(string)
	updated, err := time.Parse(time.RFC3339Nano, updatedRaw)
	if err != nil {
		// Fall back to RFC3339 — Postgres timestamptz dropped trailing zeros.
		updated, err = time.Parse(time.RFC3339, updatedRaw)
	}
	require.NoError(ts.t, err, "template updated_at should be an RFC3339 timestamp: %q", updatedRaw)
	return id, updated
}

// --- T075: PUT /templates/{id}/visual integration tests ---------------------

func TestSaveVisualTemplateHappyPath(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	templateID, updatedAt := ts.createTemplate(slug)

	status, body := ts.request(http.MethodPut, base+"/templates/"+templateID+"/visual", map[string]any{
		"subject":           "New subject",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>hello</p>",
		"bodyText":          "hello",
		"theme":             nil,
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)
	tpl := body["template"].(map[string]any)
	require.Equal(t, "New subject", tpl["subject"])
	require.Equal(t, "<p>hello</p>", tpl["body_html"])
	require.Equal(t, "hello", tpl["body_text"])
	// body_doc is the JSON pass-through of what the BFF sent.
	require.NotNil(t, tpl["body_doc"], "body_doc echoed for visual reload")
}

func TestSaveVisualTemplateRejectsMissingBodyHTML(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	templateID, updatedAt := ts.createTemplate(slug)

	status, body := ts.request(http.MethodPut, base+"/templates/"+templateID+"/visual", map[string]any{
		"subject":           "Subject",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "",
		"bodyText":          "x",
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "invalid_body", body["error"])
}

func TestSaveVisualTemplateRejectsMissingIfUnmodifiedSince(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	templateID, _ := ts.createTemplate(slug)

	status, body := ts.request(http.MethodPut, base+"/templates/"+templateID+"/visual", map[string]any{
		"subject":  "Subject",
		"bodyDoc":  emptyVisualDoc(),
		"bodyHtml": "<p>x</p>",
		"bodyText": "x",
		// ifUnmodifiedSince omitted.
	})
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "invalid_body", body["error"])
}

func TestSaveVisualTemplateRejectsMissingDoc(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	templateID, updatedAt := ts.createTemplate(slug)

	status, body := ts.request(http.MethodPut, base+"/templates/"+templateID+"/visual", map[string]any{
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
func TestSaveVisualTemplateReturnsStaleRowOnMismatchedTimestamp(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	templateID, _ := ts.createTemplate(slug)

	// A timestamp that cannot match the row's actual updated_at.
	stale := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	status, body := ts.request(http.MethodPut, base+"/templates/"+templateID+"/visual", map[string]any{
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

// FR-034 (permission gating): a caller without templates:manage cannot save.
// Owner has the permission by default, so the test invites a viewer-role
// teammate (only campaigns:get) and asserts the save is refused.
func TestSaveVisualTemplateRejectsCallerWithoutManagePermission(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	templateID, updatedAt := ts.createTemplate(slug)

	_, body := ts.request(http.MethodPost, base+"/roles", map[string]any{
		"name": "VisualTemplateViewer", "permissions": []string{"campaigns:get"},
	})
	roleID := body["id"].(string)

	member, memberEmail := ts.addMember(slug)
	ts.enterWorkspaceOn(member, slug)
	memberID := ts.workspaceUserID(slug, memberEmail)
	status, _ := ts.request(http.MethodPut, base+"/users/"+memberID+"/role",
		map[string]any{"role_id": roleID})
	require.Equal(t, http.StatusNoContent, status)
	status, body = ts.do(member, http.MethodPut, base+"/templates/"+templateID+"/visual", map[string]any{
		"subject":           "Subject",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>x</p>",
		"bodyText":          "x",
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusForbidden, status)
	require.Contains(t, body["error"], "forbidden", "error slug is a forbidden variant")
}

// T074 — GET /templates/{id} surfaces body_doc + theme after a visual save
// so the SPA can decide between visual and code editor without a second
// request.
func TestGetTemplateExposesBodyDocAfterVisualSave(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	templateID, updatedAt := ts.createTemplate(slug)

	// Initial GET — body_doc absent for a pre-Phase-7 raw-HTML template.
	status, body := ts.request(http.MethodGet, base+"/templates/"+templateID, nil)
	require.Equal(t, http.StatusOK, status)
	require.Nil(t, body["body_doc"], "legacy raw-HTML template has no body_doc")
	require.Nil(t, body["theme"], "legacy raw-HTML template has no theme")

	// Visual save populates body_doc.
	status, _ = ts.request(http.MethodPut, base+"/templates/"+templateID+"/visual", map[string]any{
		"subject":           "Visual subject",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>visual</p>",
		"bodyText":          "visual",
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)

	// Re-GET — body_doc is now non-null.
	status, body = ts.request(http.MethodGet, base+"/templates/"+templateID, nil)
	require.Equal(t, http.StatusOK, status)
	require.NotNil(t, body["body_doc"], "visually-saved template carries body_doc")
}

// --- T076: creating a campaign from a visual template copies body_doc ------

// A campaign built from a visually-authored template inherits the template's
// structured document so the campaign editor opens visually rather than
// falling back to the raw-HTML view.
func TestCreateCampaignFromVisualTemplateCopiesBodyDoc(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	templateID, updatedAt := ts.createTemplate(slug)

	// Make the template visually-authored.
	status, _ := ts.request(http.MethodPut, base+"/templates/"+templateID+"/visual", map[string]any{
		"subject":           "From template subject",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>template body</p>",
		"bodyText":          "template body",
		"ifUnmodifiedSince": updatedAt.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)

	// Create a campaign citing that template — omit body fields so the
	// inheritance path runs.
	status, camp := ts.request(http.MethodPost, base+"/campaigns", map[string]any{
		"name":            "Spring promo",
		"template_id":     templateID,
		"from_name":       "Acme",
		"from_local_part": "news",
		"list_ids":        []string{},
	})
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, "<p>template body</p>", camp["body_html"], "raw-HTML inheritance still works")
	require.NotNil(t, camp["body_doc"], "campaign inherits the template's structured doc")

	// And the body_doc on disk is the same shape the template carries (the
	// canonical empty-Nodes doc the BFF sent on save).
	want, err := json.Marshal(emptyVisualDoc())
	require.NoError(t, err)
	got, err := json.Marshal(camp["body_doc"])
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got))
}

// A transactional template carries body_doc through symmetrically. The
// create-campaign-from-template path refuses transactional templates, but
// the GET response must still expose body_doc.
func TestSaveVisualTemplateTransactionalKind(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	// Create a transactional template.
	status, body := ts.request(http.MethodPost, base+"/templates", map[string]any{
		"name": "Receipt", "kind": "transactional", "subject": "Your receipt",
		"body_html": "<p>placeholder</p>", "body_text": "placeholder",
	})
	require.Equal(t, http.StatusCreated, status)
	templateID := body["id"].(string)
	updatedRaw, _ := body["updated_at"].(string)
	updated, err := time.Parse(time.RFC3339Nano, updatedRaw)
	if err != nil {
		updated, err = time.Parse(time.RFC3339, updatedRaw)
	}
	require.NoError(t, err)

	status, _ = ts.request(http.MethodPut, base+"/templates/"+templateID+"/visual", map[string]any{
		"subject":           "Visual receipt",
		"bodyDoc":           emptyVisualDoc(),
		"bodyHtml":          "<p>visual receipt</p>",
		"bodyText":          "visual receipt",
		"ifUnmodifiedSince": updated.Format(time.RFC3339Nano),
	})
	require.Equal(t, http.StatusOK, status)
}
