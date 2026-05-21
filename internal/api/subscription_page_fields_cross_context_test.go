package api

import (
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// TestSubscriptionPageFieldsShareSubscriberFieldsRegistry verifies the
// cross-context contract from Phase 8 / T113 (FR-016b): the subscription-
// page "visible profile fields" picker and the visual editor's merge-tag
// picker read from one canonical source — the subscriber_fields registry
// (built-in pseudo-rows + tenant custom rows). A registry entry that exists
// is acceptable as a subscription-page form-field key; deleting the
// registry entry takes it out of both surfaces in the same transaction
// boundary.
func TestSubscriptionPageFieldsShareSubscriberFieldsRegistry(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	// Seed prerequisites for a saveable subscription page.
	status, body := ts.request(http.MethodPost, base+"/lists", map[string]any{"name": "Newsletter"})
	require.Equal(t, http.StatusCreated, status, "create list: %v", body)
	listID := body["id"].(string)
	require.NotEmpty(t, listID)

	sendingDomainID := ts.seedVerifiedDomain(slug, "acme-"+dbtest.RandString()+".test")

	// Create a custom field. It appears in /subscriber-fields immediately.
	status, body = ts.request(http.MethodPost, base+"/subscriber-fields", map[string]any{
		"slug": "country", "displayName": "Country", "type": "text",
	})
	require.Equal(t, http.StatusCreated, status, "create field: %v", body)
	fieldID := body["id"].(string)

	status, body = ts.request(http.MethodGet, base+"/subscriber-fields", nil)
	require.Equal(t, http.StatusOK, status)
	require.True(t, fieldsListContainsSlug(body, "country"),
		"/subscriber-fields exposes the new entry")

	// The subscription-page form accepts the same slug as a form-field key.
	pageReq := map[string]any{
		"slug":              "newsletter-" + dbtest.RandString(),
		"title":             "Newsletter",
		"target_list_ids":   []string{listID},
		"fields":            []map[string]any{{"key": "country", "label": "Country", "required": false}},
		"sending_domain_id": sendingDomainID,
		"from_name":         "Acme",
		"from_local_part":   "hello",
		"active":            true,
	}
	status, body = ts.request(http.MethodPost, base+"/subscription-pages", pageReq)
	require.Equal(t, http.StatusCreated, status, "save with registered slug: %v", body)
	pageID := body["ID"].(string)
	require.NotEmpty(t, pageID)

	// Defense in depth: the same save command rejects an unknown slug.
	badReq := cloneStringAnyMap(pageReq)
	badReq["slug"] = "newsletter-" + dbtest.RandString()
	badReq["fields"] = []map[string]any{{"key": "favorite_color", "label": "Color", "required": false}}
	status, body = ts.request(http.MethodPost, base+"/subscription-pages", badReq)
	require.Equal(t, http.StatusUnprocessableEntity, status, "unknown slug rejected: %v", body)
	require.Equal(t, "validation_failed", body["error"])

	// Delete the registry entry. It disappears from /subscriber-fields …
	status, _ = ts.request(http.MethodDelete, base+"/subscriber-fields/"+fieldID, nil)
	require.Equal(t, http.StatusNoContent, status)

	status, body = ts.request(http.MethodGet, base+"/subscriber-fields", nil)
	require.Equal(t, http.StatusOK, status)
	require.False(t, fieldsListContainsSlug(body, "country"),
		"/subscriber-fields no longer exposes the deleted entry")

	// … and the next subscription-page save that references the slug fails.
	updateReq := cloneStringAnyMap(pageReq)
	status, body = ts.request(http.MethodPut, base+"/subscription-pages/"+pageID, updateReq)
	require.Equal(t, http.StatusUnprocessableEntity, status,
		"update referencing a deleted registry slug is rejected: %v", body)
	require.Equal(t, "validation_failed", body["error"])
}

func fieldsListContainsSlug(body map[string]any, slug string) bool {
	fields, ok := body["fields"].([]any)
	if !ok {
		return false
	}
	for _, f := range fields {
		fv, ok := f.(map[string]any)
		if !ok {
			continue
		}
		if fv["slug"] == slug {
			return true
		}
	}
	return false
}

func cloneStringAnyMap(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	maps.Copy(out, src)
	return out
}
