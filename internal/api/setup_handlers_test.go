package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/config"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

const testSetupToken = "setup-token-0123456789abcdef-0123456789"

// newSingleTenantServer is a single-tenant install whose workspace slug is
// unique to the test, so first-run setup is open on the shared database.
func newSingleTenantServer(t *testing.T) (*testServer, string) {
	t.Helper()
	slug := "solo-" + dbtest.RandString()
	ts := newTestServerWith(t, func(c *config.Config) {
		c.Runtime = config.RuntimeServerless
		c.TenantSlug = slug
		c.SetupToken = testSetupToken
	})
	return ts, slug
}

func setupBody(email, token string) map[string]string {
	return map[string]string{
		"email": email, "password": "correct-horse-battery", "name": "Owner", "setup_token": token,
	}
}

func TestPlatformConfigSaaS(t *testing.T) {
	ts := newTestServer(t)

	status, body := ts.do(ts.newClient(), http.MethodGet, "/api/platform/config", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, map[string]any{"billing": true, "multi_tenant": true}, body["features"])
	require.Equal(t, false, body["setup_required"])
}

func TestSingleTenantSetup(t *testing.T) {
	ts, slug := newSingleTenantServer(t)
	anon := ts.newClient()

	status, body := ts.do(anon, http.MethodGet, "/api/platform/config", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, map[string]any{"billing": false, "multi_tenant": false}, body["features"])
	require.Equal(t, true, body["setup_required"])

	// No token and a wrong token are refused, and create nothing: the same
	// email signs up below.
	email := dbtest.RandString() + "@example.com"
	for _, token := range []string{"", "wrong-token"} {
		status, body = ts.do(anon, http.MethodPost, "/api/platform/signup", setupBody(email, token))
		require.Equal(t, http.StatusForbidden, status, "token %q", token)
		require.Equal(t, "setup_token_invalid", body["code"])
	}

	// The right token creates the owner, signed in, and the workspace.
	status, body = ts.request(http.MethodPost, "/api/platform/signup", setupBody(email, testSetupToken))
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, slug, body["tenant"].(map[string]any)["slug"])
	require.Equal(t, email, body["user"].(map[string]any)["email"])

	status, body = ts.request(http.MethodGet, "/api/platform/me", nil)
	require.Equal(t, http.StatusOK, status)
	tenants := body["tenants"].([]any)
	require.Len(t, tenants, 1)
	require.Equal(t, slug, tenants[0].(map[string]any)["slug"])
	require.Equal(t, "owner", tenants[0].(map[string]any)["role"])
	ts.enterWorkspace(slug)

	// Setup is closed now, even with the right token.
	status, body = ts.do(anon, http.MethodGet, "/api/platform/config", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, false, body["setup_required"])
	status, body = ts.do(anon, http.MethodPost, "/api/platform/signup",
		setupBody("second-"+email, testSetupToken))
	require.Equal(t, http.StatusForbidden, status)
	require.Equal(t, "setup_complete", body["code"])

	// The one workspace exists, so another cannot be created.
	status, body = ts.request(http.MethodPost, "/api/platform/tenants", map[string]string{
		"name": "Other", "slug": "other-" + dbtest.RandString(),
	})
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "slug_taken", body["code"])

	// The owner was created verified, so a fresh login works.
	status, _ = ts.do(ts.newClient(), http.MethodPost, "/api/platform/login", map[string]string{
		"email": email, "password": "correct-horse-battery",
	})
	require.Equal(t, http.StatusOK, status)
}

// TestSingleTenantCreateTenantPinsSlug covers recovery from a setup that
// created the owner but not the workspace: the owner creates it from the
// landing page and gets the configured slug, whatever they typed.
func TestSingleTenantCreateTenantPinsSlug(t *testing.T) {
	ts, slug := newSingleTenantServer(t)
	// A verified account with no workspace. A SaaS server on the same
	// database makes it, since single-tenant signup always creates both.
	email := newTestServer(t).signup()
	status, _ := ts.request(http.MethodPost, "/api/platform/login", map[string]string{
		"email": email, "password": "a-good-password",
	})
	require.Equal(t, http.StatusOK, status)

	status, body := ts.request(http.MethodPost, "/api/platform/tenants", map[string]string{
		"name": "Mine", "slug": "typed-" + dbtest.RandString(),
	})
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, slug, body["tenant"].(map[string]any)["slug"])

	status, body = ts.request(http.MethodPost, "/api/platform/tenants", map[string]string{
		"name": "Again", "slug": "again-" + dbtest.RandString(),
	})
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "slug_taken", body["code"])
}
