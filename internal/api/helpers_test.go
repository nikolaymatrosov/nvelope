package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nvelope/nvelope/internal/config"
	"github.com/nvelope/nvelope/internal/dbtest"
)

// testServer runs the full API over TLS against a real database, with a
// cookie-jar client that carries the session cookie across requests. TLS is
// required so the Secure session cookie is stored by the jar.
type testServer struct {
	*httptest.Server
	t      *testing.T
	client *http.Client
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	pool := dbtest.AppPool(t)
	cfg := config.Config{
		SessionTTL: time.Hour,
		InviteTTL:  time.Hour,
		BaseURL:    "https://app.test",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := New(pool, cfg, logger, http.NotFoundHandler()).Handler()

	hs := httptest.NewTLSServer(handler)
	t.Cleanup(hs.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := hs.Client()
	client.Jar = jar

	return &testServer{Server: hs, t: t, client: client}
}

// newClient returns a fresh client with its own empty cookie jar — an
// anonymous caller of the same server.
func (ts *testServer) newClient() *http.Client {
	ts.t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(ts.t, err)
	c := ts.Client()
	c.Jar = jar
	return c
}

// do performs a JSON request with the given client and returns the status and
// decoded body.
func (ts *testServer) do(client *http.Client, method, path string, body any) (int, map[string]any) {
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
	resp, err := client.Do(req)
	require.NoError(ts.t, err)
	defer func() { _ = resp.Body.Close() }()

	var decoded map[string]any
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) > 0 {
		require.NoError(ts.t, json.Unmarshal(raw, &decoded), "response body should be JSON")
	}
	return resp.StatusCode, decoded
}

// request performs a JSON request with the server's session-carrying client.
func (ts *testServer) request(method, path string, body any) (int, map[string]any) {
	return ts.do(ts.client, method, path, body)
}

// signup registers a new account on the session-carrying client and returns
// its email. The jar then holds the session.
func (ts *testServer) signup() string {
	ts.t.Helper()
	email := dbtest.RandString() + "@example.com"
	status, _ := ts.request(http.MethodPost, "/api/platform/signup", map[string]string{
		"email": email, "password": "a-good-password", "name": "Test User",
	})
	require.Equal(ts.t, http.StatusCreated, status)
	return email
}

// createTenant creates a tenant on the session-carrying client and returns its
// slug.
func (ts *testServer) createTenant() string {
	return ts.createTenantOn(ts.client)
}

// createTenantOn creates a tenant on the given client and returns its slug.
func (ts *testServer) createTenantOn(client *http.Client) string {
	ts.t.Helper()
	slug := "ws-" + dbtest.RandString()
	status, _ := ts.do(client, http.MethodPost, "/api/platform/tenants", map[string]string{
		"name": "Workspace", "slug": slug,
	})
	require.Equal(ts.t, http.StatusCreated, status)
	return slug
}

// signupClient registers a new account on a fresh client and returns that
// client — a second, independent authenticated caller.
func (ts *testServer) signupClient() *http.Client {
	ts.t.Helper()
	client := ts.newClient()
	status, _ := ts.do(client, http.MethodPost, "/api/platform/signup", map[string]string{
		"email": dbtest.RandString() + "@example.com", "password": "a-good-password", "name": "User",
	})
	require.Equal(ts.t, http.StatusCreated, status)
	return client
}
