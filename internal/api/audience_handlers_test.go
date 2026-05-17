package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListCRUD(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodPost, base+"/lists", map[string]any{
		"name": "Newsletter", "description": "weekly", "tags": []string{"vip"},
	})
	require.Equal(t, http.StatusCreated, status)
	listID, _ := body["id"].(string)
	require.NotEmpty(t, listID)

	status, body = ts.request(http.MethodGet, base+"/lists", nil)
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 1, body["total"])

	status, body = ts.request(http.MethodGet, base+"/lists/"+listID, nil)
	require.Equal(t, http.StatusOK, status)
	list, _ := body["list"].(map[string]any)
	require.Equal(t, "Newsletter", list["Name"])

	status, _ = ts.request(http.MethodPut, base+"/lists/"+listID, map[string]any{
		"name": "Renamed", "visibility": "public", "optin": "double",
	})
	require.Equal(t, http.StatusNoContent, status)

	status, _ = ts.request(http.MethodDelete, base+"/lists/"+listID, nil)
	require.Equal(t, http.StatusNoContent, status)

	status, _ = ts.request(http.MethodGet, base+"/lists/"+listID, nil)
	require.Equal(t, http.StatusNotFound, status)
}

func TestListDuplicateNameConflicts(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, _ := ts.request(http.MethodPost, base+"/lists", map[string]any{"name": "Dup"})
	require.Equal(t, http.StatusCreated, status)
	status, _ = ts.request(http.MethodPost, base+"/lists", map[string]any{"name": "Dup"})
	require.Equal(t, http.StatusConflict, status)
}

func TestListBlankNameRejected(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	status, _ := ts.request(http.MethodPost, "/t/"+slug+"/api/lists", map[string]any{"name": "  "})
	require.Equal(t, http.StatusUnprocessableEntity, status)
}

func TestSubscriberCRUD(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodPost, base+"/subscribers", map[string]any{
		"email": "pat@example.com", "name": "Pat",
		"attributes": map[string]any{"plan": "pro"},
	})
	require.Equal(t, http.StatusCreated, status)
	subID, _ := body["id"].(string)
	require.NotEmpty(t, subID)

	status, body = ts.request(http.MethodGet, base+"/subscribers?q=pat", nil)
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 1, body["total"])

	status, body = ts.request(http.MethodGet, base+"/subscribers/"+subID, nil)
	require.Equal(t, http.StatusOK, status)
	sub, _ := body["subscriber"].(map[string]any)
	require.Equal(t, "pat@example.com", sub["Email"])

	status, _ = ts.request(http.MethodPut, base+"/subscribers/"+subID, map[string]any{
		"name": "Patricia", "state": "disabled",
	})
	require.Equal(t, http.StatusNoContent, status)

	status, _ = ts.request(http.MethodDelete, base+"/subscribers/"+subID, nil)
	require.Equal(t, http.StatusNoContent, status)
}

func TestSubscriberDuplicateEmailConflicts(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, _ := ts.request(http.MethodPost, base+"/subscribers",
		map[string]any{"email": "dup@example.com"})
	require.Equal(t, http.StatusCreated, status)
	status, _ = ts.request(http.MethodPost, base+"/subscribers",
		map[string]any{"email": "dup@example.com"})
	require.Equal(t, http.StatusConflict, status)
}

func TestSubscriberInvalidEmailRejected(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	status, _ := ts.request(http.MethodPost, "/t/"+slug+"/api/subscribers",
		map[string]any{"email": "not-an-email"})
	require.Equal(t, http.StatusUnprocessableEntity, status)
}

func TestSubscriberListMembership(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	_, listBody := ts.request(http.MethodPost, base+"/lists", map[string]any{"name": "L"})
	listID := listBody["id"].(string)
	_, subBody := ts.request(http.MethodPost, base+"/subscribers",
		map[string]any{"email": "m@example.com"})
	subID := subBody["id"].(string)

	status, _ := ts.request(http.MethodPost, base+"/subscribers/"+subID+"/lists",
		map[string]any{"list_id": listID})
	require.Equal(t, http.StatusNoContent, status)

	status, _ = ts.request(http.MethodPut, base+"/subscribers/"+subID+"/lists/"+listID,
		map[string]any{"status": "confirmed"})
	require.Equal(t, http.StatusNoContent, status)

	status, body := ts.request(http.MethodGet, base+"/subscribers/"+subID, nil)
	require.Equal(t, http.StatusOK, status)
	sub := body["subscriber"].(map[string]any)
	memberships, _ := sub["Memberships"].([]any)
	require.Len(t, memberships, 1)

	status, _ = ts.request(http.MethodDelete, base+"/subscribers/"+subID+"/lists/"+listID, nil)
	require.Equal(t, http.StatusNoContent, status)
}

func TestAudienceIsolatedAcrossTenants(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slugA := ts.createTenant()
	slugB := ts.createTenant()
	ts.enterWorkspace(slugA)
	ts.enterWorkspace(slugB)

	status, body := ts.request(http.MethodPost, "/t/"+slugA+"/api/lists",
		map[string]any{"name": "A-only"})
	require.Equal(t, http.StatusCreated, status)
	listID := body["id"].(string)

	// The same user, in tenant B, cannot see tenant A's list.
	status, _ = ts.request(http.MethodGet, "/t/"+slugB+"/api/lists/"+listID, nil)
	require.Equal(t, http.StatusNotFound, status)

	status, body = ts.request(http.MethodGet, "/t/"+slugB+"/api/lists", nil)
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 0, body["total"], "tenant B sees none of tenant A's lists")
}
