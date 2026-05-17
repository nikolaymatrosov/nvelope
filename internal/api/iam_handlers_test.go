package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// doBearer performs a JSON request authenticated with an API key bearer token.
func (ts *testServer) doBearer(client *http.Client, method, path string, body any,
	bearer string) (int, map[string]any) {
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
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := client.Do(req)
	require.NoError(ts.t, err)
	defer func() { _ = resp.Body.Close() }()

	var decoded map[string]any
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &decoded)
	}
	return resp.StatusCode, decoded
}

// addMember invites a new account to the tenant, accepts the invitation, and
// returns the invitee's client and email.
func (ts *testServer) addMember(slug string) (*http.Client, string) {
	ts.t.Helper()
	email := dbtest.RandString() + "@example.com"
	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/invitations",
		map[string]string{"email": email})
	require.Equal(ts.t, http.StatusCreated, status)
	token := tokenFromAcceptURL(body["accept_url"].(string))

	invitee := ts.newClient()
	status, _ = ts.do(invitee, http.MethodPost, "/api/platform/invitations/"+token+"/accept",
		map[string]string{"password": "a-good-password", "name": "Member"})
	require.Equal(ts.t, http.StatusOK, status)
	return invitee, email
}

func TestRBACOwnerAllowPath(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	// The first user is the Owner — every action is allowed.
	status, body := ts.request(http.MethodPost, base+"/roles", map[string]any{
		"name": "Editor", "permissions": []string{"lists:manage", "lists:get"},
	})
	require.Equal(t, http.StatusCreated, status)
	require.NotEmpty(t, body["id"])

	status, body = ts.request(http.MethodGet, base+"/roles", nil)
	require.Equal(t, http.StatusOK, status)
	roles, _ := body["roles"].([]any)
	require.Len(t, roles, 2, "the bootstrap Owner role and the new Editor role")
}

func TestRBACMemberDeniedWithoutRole(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	member, _ := ts.addMember(slug)
	ts.enterWorkspaceOn(member, slug)

	// A member with no role holds no permissions.
	status, _ := ts.do(member, http.MethodGet, base+"/roles", nil)
	require.Equal(t, http.StatusForbidden, status, "no roles:get permission")

	status, _ = ts.do(member, http.MethodPost, base+"/lists", map[string]any{"name": "X"})
	require.Equal(t, http.StatusForbidden, status, "no lists:manage permission")
}

func TestRBACRoleAssignmentGrantsAccess(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	member, memberEmail := ts.addMember(slug)
	ts.enterWorkspaceOn(member, slug)
	memberID := ts.workspaceUserID(slug, memberEmail)

	// Owner creates a read-only role and assigns it to the member.
	_, body := ts.request(http.MethodPost, base+"/roles", map[string]any{
		"name": "Viewer", "permissions": []string{"lists:get"},
	})
	roleID := body["id"].(string)
	status, _ := ts.request(http.MethodPut, base+"/users/"+memberID+"/role",
		map[string]any{"role_id": roleID})
	require.Equal(t, http.StatusNoContent, status)

	// The change takes effect on the member's next request.
	status, _ = ts.do(member, http.MethodGet, base+"/lists", nil)
	require.Equal(t, http.StatusOK, status, "the Viewer role grants lists:get")

	status, _ = ts.do(member, http.MethodPost, base+"/lists", map[string]any{"name": "X"})
	require.Equal(t, http.StatusForbidden, status, "the Viewer role does not grant lists:manage")
}

func TestRBACPerListRoleWidensAccessForOneList(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	member, memberEmail := ts.addMember(slug)
	ts.enterWorkspaceOn(member, slug)
	memberID := ts.workspaceUserID(slug, memberEmail)

	// Owner creates two lists and an editor role, and grants the member that
	// role on listA only.
	_, a := ts.request(http.MethodPost, base+"/lists", map[string]any{"name": "ListA"})
	listA := a["id"].(string)
	_, b := ts.request(http.MethodPost, base+"/lists", map[string]any{"name": "ListB"})
	listB := b["id"].(string)
	_, role := ts.request(http.MethodPost, base+"/roles", map[string]any{
		"name": "ListEditor", "permissions": []string{"lists:manage"},
	})
	roleID := role["id"].(string)
	status, _ := ts.request(http.MethodPut, base+"/users/"+memberID+"/lists/"+listA+"/role",
		map[string]any{"role_id": roleID})
	require.Equal(t, http.StatusNoContent, status)

	// The per-list role lets the member edit listA but not listB.
	status, _ = ts.do(member, http.MethodPut, base+"/lists/"+listA,
		map[string]any{"name": "ListA-edited"})
	require.Equal(t, http.StatusNoContent, status, "per-list role grants lists:manage on listA")

	status, _ = ts.do(member, http.MethodPut, base+"/lists/"+listB,
		map[string]any{"name": "ListB-edited"})
	require.Equal(t, http.StatusForbidden, status, "the per-list role does not widen listB")
}

func TestRBACGuardedRouteRejectsNoWorkspaceSession(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	// No enterWorkspace — the caller has no Principal.

	status, _ := ts.request(http.MethodGet, "/t/"+slug+"/api/lists", nil)
	require.Equal(t, http.StatusUnauthorized, status,
		"a guarded route needs a workspace session")
}

func TestAuditTrailRecordsPrivilegedActions(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	// A privileged action — creating a role — is recorded.
	status, _ := ts.request(http.MethodPost, base+"/roles", map[string]any{
		"name": "Editor", "permissions": []string{"lists:get"},
	})
	require.Equal(t, http.StatusCreated, status)

	status, body := ts.request(http.MethodGet, base+"/audit", nil)
	require.Equal(t, http.StatusOK, status)
	require.GreaterOrEqual(t, body["total"], float64(1))
	records, _ := body["records"].([]any)
	require.NotEmpty(t, records)
	require.Equal(t, "role.create", records[0].(map[string]any)["Action"])
}

func TestAuditTrailDeniedWithoutPermission(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	member, _ := ts.addMember(slug)
	ts.enterWorkspaceOn(member, slug)

	status, _ := ts.do(member, http.MethodGet, base+"/audit", nil)
	require.Equal(t, http.StatusForbidden, status, "the audit trail needs audit:get")
}

func TestAPIKeyScopedAccess(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	// The Owner issues a read-only API key.
	status, body := ts.request(http.MethodPost, base+"/api-keys", map[string]any{
		"name": "CI", "permissions": []string{"subscribers:get"},
	})
	require.Equal(t, http.StatusCreated, status)
	token := body["token"].(string)
	keyID := body["id"].(string)
	require.NotEmpty(t, token)

	// A bare client — no workspace cookie — authenticates with the bearer key.
	bare := ts.newClient()
	status, _ = ts.doBearer(bare, http.MethodGet, base+"/subscribers", nil, token)
	require.Equal(t, http.StatusOK, status, "the key grants subscribers:get")

	status, _ = ts.doBearer(bare, http.MethodPost, base+"/subscribers",
		map[string]any{"email": "x@example.com"}, token)
	require.Equal(t, http.StatusForbidden, status, "the key does not grant subscribers:manage")

	// Revoking the key closes the door.
	status, _ = ts.request(http.MethodDelete, base+"/api-keys/"+keyID, nil)
	require.Equal(t, http.StatusNoContent, status)

	status, _ = ts.doBearer(bare, http.MethodGet, base+"/subscribers", nil, token)
	require.Equal(t, http.StatusUnauthorized, status, "a revoked key authenticates nothing")
}

func TestAPIKeyListShowsMetadataOnly(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	ts.request(http.MethodPost, base+"/api-keys", map[string]any{"name": "K"})
	status, body := ts.request(http.MethodGet, base+"/api-keys", nil)
	require.Equal(t, http.StatusOK, status)
	keys, _ := body["api_keys"].([]any)
	require.Len(t, keys, 1)
	_, hasToken := keys[0].(map[string]any)["token"]
	require.False(t, hasToken, "listing keys never exposes the token")
}

// enrollTOTP enrols the session-carrying client in TOTP and returns the shared
// secret and the one-time recovery codes.
func (ts *testServer) enrollTOTP(base string) (string, []string) {
	ts.t.Helper()
	status, body := ts.request(http.MethodPost, base+"/me/totp", nil)
	require.Equal(ts.t, http.StatusOK, status)
	secret := body["secret"].(string)

	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(ts.t, err)
	status, body = ts.request(http.MethodPost, base+"/me/totp/confirm",
		map[string]any{"secret": secret, "code": code})
	require.Equal(ts.t, http.StatusOK, status)

	rawCodes, _ := body["recovery_codes"].([]any)
	codes := make([]string, 0, len(rawCodes))
	for _, c := range rawCodes {
		codes = append(codes, c.(string))
	}
	return secret, codes
}

func TestTOTPEnrolChallengeActivate(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	secret, codes := ts.enrollTOTP(base)
	require.Len(t, codes, 10, "enrolment returns one-time recovery codes")

	// Opening a fresh session now requires the two-factor challenge.
	status, body := ts.request(http.MethodPost, base+"/session", nil)
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, "totp-pending", body["state"])

	// A guarded route stays closed until the challenge is met.
	status, _ = ts.request(http.MethodGet, base+"/lists", nil)
	require.Equal(t, http.StatusUnauthorized, status)

	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	status, _ = ts.request(http.MethodPost, base+"/session/totp", map[string]any{"code": code})
	require.Equal(t, http.StatusOK, status)

	status, _ = ts.request(http.MethodGet, base+"/lists", nil)
	require.Equal(t, http.StatusOK, status, "the session is active after the challenge")
}

func TestTOTPWrongCodeRefused(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	ts.enrollTOTP(base)
	status, _ := ts.request(http.MethodPost, base+"/session", nil)
	require.Equal(t, http.StatusCreated, status)

	status, _ = ts.request(http.MethodPost, base+"/session/totp", map[string]any{"code": "000000"})
	require.Equal(t, http.StatusUnauthorized, status, "a wrong code does not activate the session")
}

func TestTOTPRecoveryCodeActivatesSession(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	_, codes := ts.enrollTOTP(base)
	status, _ := ts.request(http.MethodPost, base+"/session", nil)
	require.Equal(t, http.StatusCreated, status)

	status, _ = ts.request(http.MethodPost, base+"/session/totp", map[string]any{"code": codes[0]})
	require.Equal(t, http.StatusOK, status, "a recovery code meets the challenge")

	status, _ = ts.request(http.MethodGet, base+"/lists", nil)
	require.Equal(t, http.StatusOK, status)
}

func TestTOTPDisableRestoresDirectSignIn(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	ts.enrollTOTP(base)
	status, _ := ts.request(http.MethodDelete, base+"/me/totp", nil)
	require.Equal(t, http.StatusNoContent, status)

	// With TOTP off, a new session is immediately active.
	status, body := ts.request(http.MethodPost, base+"/session", nil)
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, "active", body["state"])
}
