package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nvelope/nvelope/internal/dbtest"
)

// tokenFromAcceptURL extracts the invitation token from an accept_url.
func tokenFromAcceptURL(url string) string {
	return url[strings.LastIndex(url, "/")+1:]
}

// --- User Story 1: sign up and create a workspace ---------------------------

func TestSignupCreatesAccountAndSession(t *testing.T) {
	ts := newTestServer(t)
	email := dbtest.RandString() + "@example.com"

	status, body := ts.request(http.MethodPost, "/api/platform/signup", map[string]string{
		"email": email, "password": "a-good-password", "name": "Ada Lovelace",
	})
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, email, body["user"].(map[string]any)["email"])

	// The session cookie set by signup now authenticates /me.
	status, body = ts.request(http.MethodGet, "/api/platform/me", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, email, body["user"].(map[string]any)["email"])
}

func TestSignupRejectsDuplicateEmail(t *testing.T) {
	ts := newTestServer(t)
	email := dbtest.RandString() + "@example.com"

	status, _ := ts.request(http.MethodPost, "/api/platform/signup", map[string]string{
		"email": email, "password": "a-good-password", "name": "Ada",
	})
	require.Equal(t, http.StatusCreated, status)

	status, body := ts.request(http.MethodPost, "/api/platform/signup", map[string]string{
		"email": email, "password": "another-password", "name": "Imposter",
	})
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "email_taken", body["error"])
}

func TestSignupRejectsShortPassword(t *testing.T) {
	ts := newTestServer(t)
	status, body := ts.request(http.MethodPost, "/api/platform/signup", map[string]string{
		"email": dbtest.RandString() + "@example.com", "password": "short", "name": "Ada",
	})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	require.Equal(t, "validation_failed", body["error"])
}

func TestLoginSucceedsAndFails(t *testing.T) {
	ts := newTestServer(t)
	email := dbtest.RandString() + "@example.com"
	status, _ := ts.request(http.MethodPost, "/api/platform/signup", map[string]string{
		"email": email, "password": "a-good-password", "name": "Ada",
	})
	require.Equal(t, http.StatusCreated, status)

	anon := ts.newClient()

	status, body := ts.do(anon, http.MethodPost, "/api/platform/login", map[string]string{
		"email": email, "password": "the-wrong-password",
	})
	require.Equal(t, http.StatusUnauthorized, status)
	require.Equal(t, "invalid_credentials", body["error"])

	status, _ = ts.do(anon, http.MethodPost, "/api/platform/login", map[string]string{
		"email": email, "password": "a-good-password",
	})
	require.Equal(t, http.StatusOK, status)

	status, _ = ts.do(anon, http.MethodGet, "/api/platform/me", nil)
	require.Equal(t, http.StatusOK, status, "login established a session")
}

func TestLogoutEndsSession(t *testing.T) {
	ts := newTestServer(t)
	ts.signup()

	status, _ := ts.request(http.MethodPost, "/api/platform/logout", nil)
	require.Equal(t, http.StatusNoContent, status)

	status, _ = ts.request(http.MethodGet, "/api/platform/me", nil)
	require.Equal(t, http.StatusUnauthorized, status, "the session is no longer valid")
}

func TestMeRequiresAuthentication(t *testing.T) {
	ts := newTestServer(t)
	status, body := ts.request(http.MethodGet, "/api/platform/me", nil)
	require.Equal(t, http.StatusUnauthorized, status)
	require.Equal(t, "unauthenticated", body["error"])
}

func TestCreateAndListTenants(t *testing.T) {
	ts := newTestServer(t)
	ts.signup()

	slug := "acme-" + dbtest.RandString()
	status, body := ts.request(http.MethodPost, "/api/platform/tenants", map[string]string{
		"name": "Acme Newsletters", "slug": slug,
	})
	require.Equal(t, http.StatusCreated, status)
	tenant := body["tenant"].(map[string]any)
	require.Equal(t, slug, tenant["slug"])
	require.Equal(t, "active", tenant["status"])

	status, body = ts.request(http.MethodGet, "/api/platform/tenants", nil)
	require.Equal(t, http.StatusOK, status)
	tenants := body["tenants"].([]any)
	require.Len(t, tenants, 1)
	require.Equal(t, "owner", tenants[0].(map[string]any)["role"])
}

func TestCreateTenantRejectsDuplicateSlug(t *testing.T) {
	ts := newTestServer(t)
	ts.signup()
	slug := "dup-" + dbtest.RandString()

	status, _ := ts.request(http.MethodPost, "/api/platform/tenants", map[string]string{
		"name": "First", "slug": slug,
	})
	require.Equal(t, http.StatusCreated, status)

	status, body := ts.request(http.MethodPost, "/api/platform/tenants", map[string]string{
		"name": "Second", "slug": slug,
	})
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "slug_taken", body["error"])
}

func TestCreateTenantRejectsReservedSlug(t *testing.T) {
	ts := newTestServer(t)
	ts.signup()
	status, body := ts.request(http.MethodPost, "/api/platform/tenants", map[string]string{
		"name": "Acme", "slug": "api",
	})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	require.Equal(t, "validation_failed", body["error"])
}

func TestCreateTenantRequiresAuthentication(t *testing.T) {
	ts := newTestServer(t)
	status, _ := ts.request(http.MethodPost, "/api/platform/tenants", map[string]string{
		"name": "Acme", "slug": "acme",
	})
	require.Equal(t, http.StatusUnauthorized, status)
}

// --- User Story 3: tenant data stays isolated -------------------------------

func TestTenantInfoForMember(t *testing.T) {
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()

	status, body := ts.request(http.MethodGet, "/t/"+slug+"/api/tenant", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, slug, body["tenant"].(map[string]any)["slug"])
	members := body["members"].([]any)
	require.Len(t, members, 1)
	require.Equal(t, "owner", members[0].(map[string]any)["role"])
}

func TestNonMemberGetsOpaque404(t *testing.T) {
	ts := newTestServer(t)
	ts.signup()
	realSlug := ts.createTenant()

	stranger := ts.signupClient()
	// A real tenant the stranger is not a member of.
	memberStatus, memberBody := ts.do(stranger, http.MethodGet, "/t/"+realSlug+"/api/tenant", nil)
	// A tenant that does not exist at all.
	unknownStatus, unknownBody := ts.do(stranger, http.MethodGet,
		"/t/no-such-"+dbtest.RandString()+"/api/tenant", nil)

	require.Equal(t, http.StatusNotFound, memberStatus)
	require.Equal(t, http.StatusNotFound, unknownStatus)
	require.Equal(t, "tenant_not_found", memberBody["error"])
	require.Equal(t, memberBody, unknownBody,
		"a non-member and an unknown tenant must be indistinguishable")
}

func TestGetAndUpdateSettings(t *testing.T) {
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()

	status, body := ts.request(http.MethodGet, "/t/"+slug+"/api/settings", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "Workspace", body["settings"].(map[string]any)["display_name"])

	status, _ = ts.request(http.MethodPut, "/t/"+slug+"/api/settings", map[string]string{
		"display_name": "Renamed", "timezone": "Europe/Madrid",
	})
	require.Equal(t, http.StatusOK, status)

	status, body = ts.request(http.MethodGet, "/t/"+slug+"/api/settings", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "Renamed", body["settings"].(map[string]any)["display_name"])
	require.Equal(t, "Europe/Madrid", body["settings"].(map[string]any)["timezone"])
}

func TestUpdateSettingsRejectsEmptyDisplayName(t *testing.T) {
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()

	status, body := ts.request(http.MethodPut, "/t/"+slug+"/api/settings", map[string]string{
		"display_name": "  ", "timezone": "UTC",
	})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	require.Equal(t, "validation_failed", body["error"])
}

func TestTenantRoutesRequireAuthentication(t *testing.T) {
	ts := newTestServer(t)
	status, _ := ts.request(http.MethodGet, "/t/anything/api/tenant", nil)
	require.Equal(t, http.StatusUnauthorized, status)
}

// --- User Story 2: invite a teammate ----------------------------------------

func TestInviteAndAcceptByNewUser(t *testing.T) {
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()

	inviteeEmail := dbtest.RandString() + "@example.com"
	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/invitations",
		map[string]string{"email": inviteeEmail})
	require.Equal(t, http.StatusCreated, status)
	token := tokenFromAcceptURL(body["accept_url"].(string))
	require.NotEmpty(t, token)

	// An anonymous visitor opens the link and sees who it is for.
	invitee := ts.newClient()
	status, body = ts.do(invitee, http.MethodGet, "/api/platform/invitations/"+token, nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, inviteeEmail, body["email"])

	// Accepting creates their account and joins them to the tenant.
	status, body = ts.do(invitee, http.MethodPost,
		"/api/platform/invitations/"+token+"/accept",
		map[string]string{"password": "a-good-password", "name": "Grace"})
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, slug, body["tenant"].(map[string]any)["slug"])

	// The invitee can now reach the shared workspace.
	status, _ = ts.do(invitee, http.MethodGet, "/t/"+slug+"/api/tenant", nil)
	require.Equal(t, http.StatusOK, status)
}

func TestAcceptInvitationByExistingUser(t *testing.T) {
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()

	inviteeEmail := dbtest.RandString() + "@example.com"
	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/invitations",
		map[string]string{"email": inviteeEmail})
	require.Equal(t, http.StatusCreated, status)
	token := tokenFromAcceptURL(body["accept_url"].(string))

	// The invitee already has an account and is logged in.
	invitee := ts.newClient()
	status, _ = ts.do(invitee, http.MethodPost, "/api/platform/signup",
		map[string]string{"email": inviteeEmail, "password": "a-good-password", "name": "Grace"})
	require.Equal(t, http.StatusCreated, status)

	status, _ = ts.do(invitee, http.MethodPost,
		"/api/platform/invitations/"+token+"/accept", nil)
	require.Equal(t, http.StatusOK, status)

	status, _ = ts.do(invitee, http.MethodGet, "/t/"+slug+"/api/tenant", nil)
	require.Equal(t, http.StatusOK, status, "the existing user joined the tenant")
}

func TestInviteRejectsExistingMember(t *testing.T) {
	ts := newTestServer(t)
	ownerEmail := ts.signup()
	slug := ts.createTenant()

	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/invitations",
		map[string]string{"email": ownerEmail})
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "already_member", body["error"])
}

func TestInviteRejectsDuplicatePending(t *testing.T) {
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	email := dbtest.RandString() + "@example.com"

	status, _ := ts.request(http.MethodPost, "/t/"+slug+"/api/invitations",
		map[string]string{"email": email})
	require.Equal(t, http.StatusCreated, status)

	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/invitations",
		map[string]string{"email": email})
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "invitation_exists", body["error"])
}

func TestGetInvitationUnknownToken(t *testing.T) {
	ts := newTestServer(t)
	status, body := ts.request(http.MethodGet, "/api/platform/invitations/no-such-token", nil)
	require.Equal(t, http.StatusNotFound, status)
	require.Equal(t, "invitation_not_found", body["error"])
}

func TestRevokeInvitationEndpoint(t *testing.T) {
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()

	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/invitations",
		map[string]string{"email": dbtest.RandString() + "@example.com"})
	require.Equal(t, http.StatusCreated, status)
	id := body["invitation"].(map[string]any)["id"].(string)

	status, _ = ts.request(http.MethodDelete, "/t/"+slug+"/api/invitations/"+id, nil)
	require.Equal(t, http.StatusNoContent, status)

	status, body = ts.request(http.MethodGet, "/t/"+slug+"/api/invitations", nil)
	require.Equal(t, http.StatusOK, status)
	require.Empty(t, body["invitations"], "the revoked invitation is no longer pending")
}
