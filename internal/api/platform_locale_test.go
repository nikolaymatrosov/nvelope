package api

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// withLocaleCookie returns a fresh anonymous client whose jar already carries
// an nv_locale cookie — a signed-out visitor who previously picked a language.
func withLocaleCookie(ts *testServer, value string) *http.Client {
	ts.t.Helper()
	client := ts.newClient()
	u, err := url.Parse(ts.URL)
	require.NoError(ts.t, err)
	client.Jar.SetCookies(u, []*http.Cookie{{Name: "nv_locale", Value: value}})
	return client
}

func localeCookieValue(ts *testServer, client *http.Client) string {
	ts.t.Helper()
	u, err := url.Parse(ts.URL)
	require.NoError(ts.t, err)
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == "nv_locale" {
			return c.Value
		}
	}
	return ""
}

func TestMeIncludesLocale(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()

	status, body := ts.request(http.MethodGet, "/api/platform/me", nil)
	require.Equal(t, http.StatusOK, status)
	user := body["user"].(map[string]any)
	require.Nil(t, user["locale"], "a freshly signed-up user has no locale")
}

func TestUpdateMeLocalePersistsAndMirrorsToCookie(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()

	status, body := ts.request(http.MethodPut, "/api/platform/me",
		map[string]string{"locale": "ru"})
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "ru", body["user"].(map[string]any)["locale"])

	// The choice is persisted — a later GET /me reflects it.
	status, body = ts.request(http.MethodGet, "/api/platform/me", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "ru", body["user"].(map[string]any)["locale"])

	// The nv_locale cookie mirrors the effective locale for SSR.
	require.Equal(t, "ru", localeCookieValue(ts, ts.client))
}

func TestUpdateMeRejectsUnsupportedLocale(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()

	status, body := ts.request(http.MethodPut, "/api/platform/me",
		map[string]string{"locale": "de"})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	require.Equal(t, "unsupported_locale", body["error"])
}

func TestUpdateMeRequiresAuthentication(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	anon := ts.newClient()

	status, _ := ts.do(anon, http.MethodPut, "/api/platform/me",
		map[string]string{"locale": "ru"})
	require.Equal(t, http.StatusUnauthorized, status)
}

func TestUpdateMeIsScopedToCaller(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()                // user A on ts.client
	other := ts.signupClient() // user B on its own client

	status, _ := ts.request(http.MethodPut, "/api/platform/me",
		map[string]string{"locale": "ru"})
	require.Equal(t, http.StatusOK, status)

	// User B's locale is untouched — a locale write only affects the caller.
	status, body := ts.do(other, http.MethodGet, "/api/platform/me", nil)
	require.Equal(t, http.StatusOK, status)
	require.Nil(t, body["user"].(map[string]any)["locale"])
}

func TestSignupAdoptsLocaleCookie(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	client := withLocaleCookie(ts, "ru")

	status, body := ts.do(client, http.MethodPost, "/api/platform/signup",
		map[string]string{
			"email":    dbtest.RandString() + "@example.com",
			"password": "a-good-password",
			"name":     "New User",
		})
	require.Equal(t, http.StatusCreated, status)

	// FR-008: a brand-new account adopts the signed-out visitor's choice.
	require.Equal(t, "ru", body["user"].(map[string]any)["locale"])
	status, body = ts.do(client, http.MethodGet, "/api/platform/me", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "ru", body["user"].(map[string]any)["locale"])
}

func TestSignupIgnoresUnsupportedLocaleCookie(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	client := withLocaleCookie(ts, "de")

	status, body := ts.do(client, http.MethodPost, "/api/platform/signup",
		map[string]string{
			"email":    dbtest.RandString() + "@example.com",
			"password": "a-good-password",
			"name":     "New User",
		})
	require.Equal(t, http.StatusCreated, status)
	// An unsupported cookie value is not adopted.
	require.Nil(t, body["user"].(map[string]any)["locale"])
}

func TestLoginKeepsExistingPreferenceOverCookie(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	// User A signs up and explicitly chooses Russian.
	email := dbtest.RandString() + "@example.com"
	status, _ := ts.request(http.MethodPost, "/api/platform/signup",
		map[string]string{"email": email, "password": "a-good-password", "name": "A"})
	require.Equal(t, http.StatusCreated, status)
	status, _ = ts.request(http.MethodPut, "/api/platform/me",
		map[string]string{"locale": "ru"})
	require.Equal(t, http.StatusOK, status)

	// Logging in from a new browser whose cookie says English must not
	// overwrite the stored Russian preference (FR-008).
	client := withLocaleCookie(ts, "en")
	status, body := ts.do(client, http.MethodPost, "/api/platform/login",
		map[string]string{"email": email, "password": "a-good-password"})
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "ru", body["user"].(map[string]any)["locale"])
	require.Equal(t, "ru", localeCookieValue(ts, client))
}
