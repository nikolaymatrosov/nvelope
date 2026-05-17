package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

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
