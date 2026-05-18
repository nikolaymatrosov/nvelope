package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSuppressionListAddViewRemove(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodPost, base+"/suppressions", map[string]any{
		"email": "blocked@example.com", "note": "added by support",
	})
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, "manual", body["reason"])

	status, body = ts.request(http.MethodGet, base+"/suppressions", nil)
	require.Equal(t, http.StatusOK, status)
	items, _ := body["items"].([]any)
	require.Len(t, items, 1)
	first, _ := items[0].(map[string]any)
	require.Equal(t, "blocked@example.com", first["email"])

	status, _ = ts.request(http.MethodDelete, base+"/suppressions/blocked@example.com", nil)
	require.Equal(t, http.StatusNoContent, status)

	// Removing a second time is a 404 — the entry is gone.
	status, _ = ts.request(http.MethodDelete, base+"/suppressions/blocked@example.com", nil)
	require.Equal(t, http.StatusNotFound, status)
}

func TestSuppressionRejectsInvalidEmail(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, _ := ts.request(http.MethodPost, base+"/suppressions", map[string]any{
		"email": "not-an-email",
	})
	require.Equal(t, http.StatusUnprocessableEntity, status)
}

func TestBounceSettingsGetAndUpdate(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	// No row yet — the defaults (both toggles on) are returned.
	status, body := ts.request(http.MethodGet, base+"/bounce-settings", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, true, body["suppressOnHardBounce"])
	require.Equal(t, true, body["suppressOnComplaint"])

	status, body = ts.request(http.MethodPut, base+"/bounce-settings", map[string]any{
		"suppressOnHardBounce": true, "suppressOnComplaint": false,
	})
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, false, body["suppressOnComplaint"])

	status, body = ts.request(http.MethodGet, base+"/bounce-settings", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, false, body["suppressOnComplaint"])
}

func TestCampaignAnalyticsBeforeAndForMissingCampaign(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodPost, base+"/campaigns", map[string]any{
		"name": "Spring Sale", "subject": "News",
		"body_html": "<p>News</p>", "body_text": "News",
		"from_name": "Acme", "from_local_part": "news", "list_ids": []string{},
	})
	require.Equal(t, http.StatusCreated, status)
	campaignID, _ := body["id"].(string)

	// Before any refresh the analytics view renders zero counts and a null
	// refreshedAt.
	status, body = ts.request(http.MethodGet, base+"/campaigns/"+campaignID+"/analytics", nil)
	require.Equal(t, http.StatusOK, status)
	counts, _ := body["counts"].(map[string]any)
	require.Equal(t, float64(0), counts["sent"])
	require.Nil(t, body["refreshedAt"])

	// A campaign that does not exist is a 404.
	status, _ = ts.request(http.MethodGet,
		base+"/campaigns/00000000-0000-0000-0000-000000000000/analytics", nil)
	require.Equal(t, http.StatusNotFound, status)
}

func TestDashboardRenders(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodGet, base+"/dashboard", nil)
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "totals")
	require.Contains(t, body, "deliverability")
	require.Contains(t, body, "recentCampaigns")
}
