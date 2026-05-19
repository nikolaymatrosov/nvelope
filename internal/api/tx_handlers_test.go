package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// doWithKey performs a JSON request authenticated by an API-key bearer token
// (an empty key sends no Authorization header) and returns the status and body.
func (ts *testServer) doWithKey(method, path, key string, body any) (int, map[string]any) {
	ts.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(ts.t, err)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, ts.URL+path, reader)
	require.NoError(ts.t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := ts.newClient().Do(req)
	require.NoError(ts.t, err)
	defer func() { _ = resp.Body.Close() }()

	var decoded map[string]any
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) > 0 {
		require.NoError(ts.t, json.Unmarshal(raw, &decoded))
	}
	return resp.StatusCode, decoded
}

// issueAPIKey issues an API key with the given permission scopes and returns
// its raw token.
func (ts *testServer) issueAPIKey(slug string, permissions []string) string {
	ts.t.Helper()
	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/api-keys", map[string]any{
		"name": "tx-key", "permissions": permissions,
	})
	require.Equal(ts.t, http.StatusCreated, status)
	token, _ := body["token"].(string)
	require.NotEmpty(ts.t, token)
	return token
}

// subscribe subscribes the workspace to the first published plan, so billing's
// quota gate permits metered sends.
func (ts *testServer) subscribe(slug string) {
	ts.t.Helper()
	status, body := ts.request(http.MethodGet, "/t/"+slug+"/api/plans", nil)
	require.Equal(ts.t, http.StatusOK, status)
	plans, _ := body["plans"].([]any)
	require.NotEmpty(ts.t, plans, "the seeded plan catalog is published")
	first, _ := plans[0].(map[string]any)
	planID, _ := first["id"].(string)
	require.NotEmpty(ts.t, planID)
	status, _ = ts.request(http.MethodPost, "/t/"+slug+"/api/subscription",
		map[string]any{"planId": planID})
	require.Equal(ts.t, http.StatusCreated, status)
}

// createTxTemplate creates a transactional template and returns its id.
func (ts *testServer) createTxTemplate(slug string) string {
	ts.t.Helper()
	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/templates", map[string]any{
		"name": "Reset", "kind": "transactional", "subject": "Hi {{name}}",
		"body_html": "<p>Reset link: {{url}}</p>", "body_text": "Reset: {{url}}",
	})
	require.Equal(ts.t, http.StatusCreated, status)
	id, _ := body["id"].(string)
	require.NotEmpty(ts.t, id)
	return id
}

func TestTransactionalSendWithValidKey(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	ts.subscribe(slug)
	domainID := ts.seedVerifiedDomain(slug, "mail.acme.com")
	templateID := ts.createTxTemplate(slug)
	key := ts.issueAPIKey(slug, []string{"transactional:send"})

	status, body := ts.doWithKey(http.MethodPost, "/t/"+slug+"/api/tx", key, map[string]any{
		"template_id": templateID, "to": "sam@example.com",
		"sending_domain_id": domainID, "from_name": "Acme", "from_local_part": "noreply",
		"variables": map[string]string{"name": "Sam", "url": "https://acme.com/r"},
	})
	require.Equal(t, http.StatusAccepted, status)
	require.NotEmpty(t, body["message_id"])

	msgs := ts.txMessenger.all()
	require.Len(t, msgs, 1, "exactly one transactional message is delivered")
	require.Equal(t, "sam@example.com", msgs[0].To)
	require.Equal(t, "Hi Sam", msgs[0].Subject)
}

func TestTransactionalSendRejectsBadKeys(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	domainID := ts.seedVerifiedDomain(slug, "mail.acme.com")
	templateID := ts.createTxTemplate(slug)
	payload := map[string]any{
		"template_id": templateID, "to": "sam@example.com",
		"sending_domain_id": domainID, "from_name": "Acme", "from_local_part": "noreply",
	}

	// No key.
	status, _ := ts.doWithKey(http.MethodPost, "/t/"+slug+"/api/tx", "", payload)
	require.Equal(t, http.StatusUnauthorized, status)

	// Invalid key.
	status, _ = ts.doWithKey(http.MethodPost, "/t/"+slug+"/api/tx", "not-a-real-key", payload)
	require.Equal(t, http.StatusUnauthorized, status)

	// A key without the transactional-send scope.
	scopeless := ts.issueAPIKey(slug, []string{"lists:get"})
	status, _ = ts.doWithKey(http.MethodPost, "/t/"+slug+"/api/tx", scopeless, payload)
	require.Equal(t, http.StatusForbidden, status)

	require.Empty(t, ts.txMessenger.all(), "no rejected request sends a message")
}

func TestTransactionalSendRejectsCrossTenantKey(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	// Tenant A issues a transactional key.
	ts.signup()
	slugA := ts.createTenant()
	ts.enterWorkspace(slugA)
	keyA := ts.issueAPIKey(slugA, []string{"transactional:send"})

	// Tenant B has its own verified domain and template.
	clientB := ts.signupClient()
	slugB := ts.createTenantOn(clientB)
	ts.enterWorkspaceOn(clientB, slugB)
	domainB := ts.seedVerifiedDomain(slugB, "mail.beta.com")
	// Tenant B's template, created on tenant B.
	statusB, bodyB := ts.do(clientB, http.MethodPost, "/t/"+slugB+"/api/templates", map[string]any{
		"name": "Reset", "kind": "transactional", "subject": "Hi", "body_html": "<p>b</p>",
	})
	require.Equal(t, http.StatusCreated, statusB)
	templateB, _ := bodyB["id"].(string)

	// Tenant A's key cannot send against tenant B.
	status, _ := ts.doWithKey(http.MethodPost, "/t/"+slugB+"/api/tx", keyA, map[string]any{
		"template_id": templateB, "to": "x@example.com",
		"sending_domain_id": domainB, "from_name": "Beta", "from_local_part": "noreply",
	})
	require.GreaterOrEqual(t, status, 400, "a cross-tenant key is rejected")
	require.Less(t, status, 404)
	require.Empty(t, ts.txMessenger.all())
}
