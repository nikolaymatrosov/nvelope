package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// --- T038: subscriber-fields CRUD integration tests --------------------------

func TestSubscriberFieldsCRUD(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	// Built-in pseudo-rows are present from the start.
	status, body := ts.request(http.MethodGet, base+"/subscriber-fields", nil)
	require.Equal(t, http.StatusOK, status)
	fields := body["fields"].([]any)
	require.GreaterOrEqual(t, len(fields), 5, "built-in pseudo-rows are always listed")
	builtInSlugs := make(map[string]bool)
	for _, f := range fields {
		fv := f.(map[string]any)
		if fv["builtIn"].(bool) {
			builtInSlugs[fv["slug"].(string)] = true
		}
	}
	for _, want := range []string{"email", "name", "first_name", "last_name", "state"} {
		require.True(t, builtInSlugs[want], "built-in slug %q is exposed by the picker", want)
	}

	// Create a custom field; the response carries the persisted row.
	status, body = ts.request(http.MethodPost, base+"/subscriber-fields", map[string]any{
		"slug": "country", "displayName": "Country", "type": "text", "defaultValue": "US",
	})
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, "country", body["slug"])
	require.Equal(t, false, body["builtIn"])
	require.Equal(t, "US", body["defaultValue"])
	fieldID := body["id"].(string)
	require.NotEmpty(t, fieldID)

	// PATCH applies a partial change; slug stays immutable.
	status, body = ts.request(http.MethodPatch, base+"/subscriber-fields/"+fieldID, map[string]any{
		"displayName": "Country / region",
	})
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "Country / region", body["displayName"])
	require.Equal(t, "country", body["slug"], "slug is immutable")
	require.Equal(t, "US", body["defaultValue"], "unchanged on partial patch")

	// DELETE removes the field.
	status, _ = ts.request(http.MethodDelete, base+"/subscriber-fields/"+fieldID, nil)
	require.Equal(t, http.StatusNoContent, status)

	// Subsequent GET no longer lists it.
	status, body = ts.request(http.MethodGet, base+"/subscriber-fields", nil)
	require.Equal(t, http.StatusOK, status)
	for _, f := range body["fields"].([]any) {
		fv := f.(map[string]any)
		require.NotEqual(t, "country", fv["slug"], "deleted slug is gone")
	}
}

func TestSubscriberFieldsRejectsBuiltinSlug(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodPost, base+"/subscriber-fields", map[string]any{
		"slug": "email", "displayName": "Email", "type": "text",
	})
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "subscriber_field_builtin_slug", body["error"])
}

func TestSubscriberFieldsRejectsDuplicateSlug(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, _ := ts.request(http.MethodPost, base+"/subscriber-fields", map[string]any{
		"slug": "country", "displayName": "Country", "type": "text",
	})
	require.Equal(t, http.StatusCreated, status)
	status, body := ts.request(http.MethodPost, base+"/subscriber-fields", map[string]any{
		"slug": "country", "displayName": "Country 2", "type": "text",
	})
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "subscriber_field_slug_taken", body["error"])
}

func TestSubscriberFieldsRejectsInvalidSlug(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/subscriber-fields", map[string]any{
		"slug": "Bad-Slug", "displayName": "Bad", "type": "text",
	})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	require.Equal(t, "subscriber_field_invalid_slug", body["error"])
}

func TestSubscriberFieldsReorder(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	create := func(s string) string {
		status, body := ts.request(http.MethodPost, base+"/subscriber-fields", map[string]any{
			"slug": s, "displayName": s, "type": "text",
		})
		require.Equal(t, http.StatusCreated, status)
		return body["id"].(string)
	}
	a := create("alpha")
	b := create("bravo")
	c := create("charlie")

	status, body := ts.request(http.MethodPatch, base+"/subscriber-fields/order", map[string]any{
		"order": []string{c, a, b},
	})
	require.Equal(t, http.StatusOK, status)
	// The response includes built-ins first, then the custom fields in the
	// requested order.
	customs := []string{}
	for _, f := range body["fields"].([]any) {
		fv := f.(map[string]any)
		if !fv["builtIn"].(bool) {
			customs = append(customs, fv["slug"].(string))
		}
	}
	require.Equal(t, []string{"charlie", "alpha", "bravo"}, customs)

	// Incomplete reorder is rejected.
	status, body = ts.request(http.MethodPatch, base+"/subscriber-fields/order", map[string]any{
		"order": []string{a, b}, // missing c
	})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	require.Equal(t, "subscriber_field_reorder_incomplete", body["error"])
}

// FR-016e: deleting a registry definition MUST NOT delete underlying
// subscriber attribute data. Phase 7 spec calls this out explicitly because
// subscribers carry custom attributes in a JSONB column independent of the
// field-definition rows.
func TestDeleteFieldKeepsSubscriberAttribute(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	// Create the registry entry.
	status, body := ts.request(http.MethodPost, base+"/subscriber-fields", map[string]any{
		"slug": "country", "displayName": "Country", "type": "text",
	})
	require.Equal(t, http.StatusCreated, status)
	fieldID := body["id"].(string)

	// Seed a subscriber whose attributes include `country`.
	status, body = ts.request(http.MethodPost, base+"/subscribers", map[string]any{
		"email": "pat@example.com", "name": "Pat",
		"attributes": map[string]any{"country": "DE"},
	})
	require.Equal(t, http.StatusCreated, status)
	subID := body["id"].(string)

	// Delete the registry entry.
	status, _ = ts.request(http.MethodDelete, base+"/subscriber-fields/"+fieldID, nil)
	require.Equal(t, http.StatusNoContent, status)

	// Subscriber row still carries the attribute (the column is independent
	// of the registry).
	status, body = ts.request(http.MethodGet, base+"/subscribers/"+subID, nil)
	require.Equal(t, http.StatusOK, status)
	sub := body["subscriber"].(map[string]any)
	attrs, _ := sub["Attributes"].(map[string]any)
	require.Equal(t, "DE", attrs["country"], "deleting a field definition does not erase subscriber attribute data")
}

// Tenants must not see each other's registry rows. Belt-and-suspenders for
// Constitution I (tenant isolation by RLS) — the application layer also
// filters by tenant_id, but the test pretends the filter is absent by
// switching workspace sessions.
func TestSubscriberFieldsTenantIsolation(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slugA := ts.createTenant()
	slugB := ts.createTenant()
	ts.enterWorkspace(slugA)

	status, body := ts.request(http.MethodPost, "/t/"+slugA+"/api/subscriber-fields", map[string]any{
		"slug": "country", "displayName": "Country", "type": "text",
	})
	require.Equal(t, http.StatusCreated, status)
	fieldID := body["id"].(string)

	// Same user enters tenant B; the field from A must not appear.
	ts.enterWorkspace(slugB)
	status, body = ts.request(http.MethodGet, "/t/"+slugB+"/api/subscriber-fields", nil)
	require.Equal(t, http.StatusOK, status)
	for _, f := range body["fields"].([]any) {
		fv := f.(map[string]any)
		require.NotEqual(t, "country", fv["slug"], "tenant B never sees tenant A's custom field")
		require.NotEqual(t, fieldID, fv["id"])
	}
	// Direct fetch via tenant B's id-scoped route is 404.
	status, _ = ts.request(http.MethodDelete, "/t/"+slugB+"/api/subscriber-fields/"+fieldID, nil)
	require.Equal(t, http.StatusNotFound, status)
}

// --- T039: GET /merge-tags integration test ----------------------------------

func TestMergeTagsListIncludesSubscriberAndCampaign(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	// Seed one custom field so the response covers all three sources
	// (built-in pseudo-rows, custom row, campaign-namespace allow-list).
	status, _ := ts.request(http.MethodPost, base+"/subscriber-fields", map[string]any{
		"slug": "country", "displayName": "Country", "type": "text",
	})
	require.Equal(t, http.StatusCreated, status)

	status, body := ts.request(http.MethodGet, base+"/merge-tags", nil)
	require.Equal(t, http.StatusOK, status)

	subscriber := body["subscriber"].([]any)
	gotSlugs := map[string]bool{}
	gotBuiltIn := map[string]bool{}
	for _, f := range subscriber {
		fv := f.(map[string]any)
		gotSlugs[fv["slug"].(string)] = true
		gotBuiltIn[fv["slug"].(string)] = fv["builtIn"].(bool)
	}
	for _, want := range []string{"email", "name", "first_name", "last_name", "state"} {
		require.True(t, gotSlugs[want], "built-in %q present", want)
		require.True(t, gotBuiltIn[want], "built-in marker carried through")
	}
	require.True(t, gotSlugs["country"], "custom field surfaced")
	require.False(t, gotBuiltIn["country"], "custom field is not flagged as built-in")

	campaign := body["campaign"].([]any)
	gotKeys := map[string]bool{}
	for _, f := range campaign {
		fv := f.(map[string]any)
		gotKeys[fv["key"].(string)] = true
	}
	for _, want := range []string{
		"unsubscribe_url", "preference_url", "archive_url",
		"view_in_browser_url", "tenant_name", "current_date",
	} {
		require.True(t, gotKeys[want], "campaign-namespace key %q is listed", want)
	}
}

func TestMergeTagsRequiresWorkspaceSession(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	// Note: no enterWorkspace — the workspace cookie is absent.

	status, _ := ts.request(http.MethodGet, "/t/"+slug+"/api/merge-tags", nil)
	require.Equal(t, http.StatusUnauthorized, status)
}
