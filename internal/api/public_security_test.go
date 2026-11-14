package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicSecurityHeaders(t *testing.T) {
	t.Parallel()
	h := publicSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/acme/subscribe/news", nil))

	require.Equal(t, publicCSP, rec.Header().Get("Content-Security-Policy"))
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	require.NotContains(t, publicCSP, "script-src", "scripts fall back to default-src 'none'")
	require.Contains(t, archiveCSP, "sandbox ")
}

func TestPublicPagesCarrySecurityHeaders(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	anon := ts.newClient()

	get := func(path string) *http.Response {
		t.Helper()
		resp, err := anon.Get(ts.URL + path)
		require.NoError(t, err)
		_ = resp.Body.Close()
		return resp
	}

	require.Equal(t, publicCSP, get("/t/"+slug+"/archive").Header.Get("Content-Security-Policy"))
	require.Equal(t, publicCSP, get("/p/not-a-token").Header.Get("Content-Security-Policy"))

	archived := get("/t/" + slug + "/archive/00000000-0000-0000-0000-000000000000")
	require.Equal(t, http.StatusNotFound, archived.StatusCode)
	require.Equal(t, archiveCSP, archived.Header.Get("Content-Security-Policy"),
		"the archive page is sandboxed even when it renders an error")
}
