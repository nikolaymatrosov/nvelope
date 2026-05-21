package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// TestPhase7AuditEventsLandInAuditLog covers T115: every Phase 7 mutating
// endpoint (visual save for campaign + template, subscriber_field
// create/update/delete/reorder) must append a row to audit_log with the
// contracted action string and a non-empty target. The audit trail is read
// back through the existing /audit endpoint so we exercise the exact same
// path the operator UI uses.
func TestPhase7AuditEventsLandInAuditLog(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	// --- subscriber-field create / update / delete ---
	status, body := ts.request(http.MethodPost, base+"/subscriber-fields", map[string]any{
		"slug": "country", "displayName": "Country", "type": "text",
	})
	require.Equal(t, http.StatusCreated, status, "create field: %v", body)
	fieldID := body["id"].(string)

	status, _ = ts.request(http.MethodPatch, base+"/subscriber-fields/"+fieldID, map[string]any{
		"displayName": "Country / region",
	})
	require.Equal(t, http.StatusOK, status)

	status, _ = ts.request(http.MethodDelete, base+"/subscriber-fields/"+fieldID, nil)
	require.Equal(t, http.StatusNoContent, status)

	// Create one more field so a reorder can hit a non-empty registry.
	status, body = ts.request(http.MethodPost, base+"/subscriber-fields", map[string]any{
		"slug": "tier", "displayName": "Tier", "type": "text",
	})
	require.Equal(t, http.StatusCreated, status)
	tierID := body["id"].(string)

	status, _ = ts.request(http.MethodPatch, base+"/subscriber-fields/order", map[string]any{
		"order": []string{tierID},
	})
	require.Equal(t, http.StatusOK, status)

	// --- visual campaign + template save ---
	sendingDomainID := ts.seedVerifiedDomain(slug, "audit-"+dbtest.RandString()+".test")
	listID := ts.seedSubscribersOnList(slug, []string{"audit@example.test"})

	status, body = ts.request(http.MethodPost, base+"/campaigns", map[string]any{
		"name":              "Audit campaign",
		"list_ids":          []string{listID},
		"sending_domain_id": sendingDomainID,
		"from_local_part":   "hello",
		"from_name":         "Audit Co",
	})
	require.Equal(t, http.StatusCreated, status, "create campaign: %v", body)
	campaignID := body["id"].(string)

	// Read the row to capture its updated_at for the optimistic-concurrency gate.
	status, body = ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
	require.Equal(t, http.StatusOK, status, "get campaign: %v", body)
	campaignUpdatedAt := body["updated_at"].(string)

	status, body = ts.request(http.MethodPut, base+"/campaigns/"+campaignID+"/visual", map[string]any{
		"subject":           "Hi",
		"bodyDoc":           map[string]any{"version": 1, "type": "doc", "nodes": []any{}, "theme": nil},
		"bodyHtml":          "<p>Hi</p>",
		"bodyText":          "Hi",
		"theme":             nil,
		"ifUnmodifiedSince": campaignUpdatedAt,
	})
	require.Equal(t, http.StatusOK, status, "save visual campaign: %v", body)

	status, body = ts.request(http.MethodPost, base+"/templates", map[string]any{
		"name": "Audit template", "kind": "campaign", "subject": "Hi",
		"body_html": "<p>seed</p>", "body_text": "seed",
	})
	require.Equal(t, http.StatusCreated, status, "create template: %v", body)
	templateID := body["id"].(string)

	status, body = ts.request(http.MethodGet, base+"/templates/"+templateID, nil)
	require.Equal(t, http.StatusOK, status, "get template: %v", body)
	templateUpdatedAt := body["updated_at"].(string)

	status, body = ts.request(http.MethodPut, base+"/templates/"+templateID+"/visual", map[string]any{
		"subject":           "Hi",
		"bodyDoc":           map[string]any{"version": 1, "type": "doc", "nodes": []any{}, "theme": nil},
		"bodyHtml":          "<p>Hi</p>",
		"bodyText":          "Hi",
		"theme":             nil,
		"ifUnmodifiedSince": templateUpdatedAt,
	})
	require.Equal(t, http.StatusOK, status, "save visual template: %v", body)

	// --- Read the audit trail and assert every expected action appears once. ---
	status, body = ts.request(http.MethodGet, base+"/audit?limit=100", nil)
	require.Equal(t, http.StatusOK, status)
	records, _ := body["records"].([]any)

	got := map[string]int{}
	for _, r := range records {
		rv := r.(map[string]any)
		got[rv["Action"].(string)]++
	}
	for _, action := range []string{
		"subscriber_field.create",
		"subscriber_field.update",
		"subscriber_field.delete",
		"subscriber_field.reorder",
		"campaign.save_visual",
		"template.save_visual",
	} {
		require.GreaterOrEqual(t, got[action], 1,
			"audit_log is missing %q (saw: %v)", action, got)
	}
}
