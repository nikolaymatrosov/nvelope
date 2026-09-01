package api

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListCRUD(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodPost, base+"/lists", map[string]any{
		"name": "Newsletter", "description": "weekly", "tags": []string{"vip"},
	})
	require.Equal(t, http.StatusCreated, status)
	listID, _ := body["id"].(string)
	require.NotEmpty(t, listID)

	status, body = ts.request(http.MethodGet, base+"/lists", nil)
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 1, body["total"])

	status, body = ts.request(http.MethodGet, base+"/lists/"+listID, nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "Newsletter", body["name"])

	status, _ = ts.request(http.MethodPut, base+"/lists/"+listID, map[string]any{
		"name": "Renamed", "visibility": "public", "optIn": "double",
	})
	require.Equal(t, http.StatusNoContent, status)

	status, _ = ts.request(http.MethodDelete, base+"/lists/"+listID, nil)
	require.Equal(t, http.StatusNoContent, status)

	status, _ = ts.request(http.MethodGet, base+"/lists/"+listID, nil)
	require.Equal(t, http.StatusNotFound, status)
}

// TestListsWireShape pins the contract shape the generated list operations
// serve: the page is an {items,total} envelope, a single list comes back
// unwrapped, and every property is lowerCamelCase. None of that was true of the
// hand-written handlers, which serialized the untagged query view directly.
func TestListsWireShape(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodPost, base+"/lists", map[string]any{
		"name": "Newsletter", "description": "weekly", "tags": []string{"a"},
	})
	require.Equal(t, http.StatusCreated, status)
	id, ok := body["id"].(string)
	require.True(t, ok, "the 201 body is {id}, not a wrapper")
	require.NotEmpty(t, id)

	status, body = ts.request(http.MethodGet, base+"/lists?limit=10&offset=0", nil)
	require.Equal(t, http.StatusOK, status)
	items, ok := body["items"].([]any)
	require.True(t, ok, "the page envelope is {items,total}, not {lists,total}")
	require.Len(t, items, 1)
	require.Contains(t, body, "total")

	first, _ := items[0].(map[string]any)
	require.Equal(t, "Newsletter", first["name"], "properties are lowerCamelCase")
	require.Equal(t, "weekly", first["description"])
	require.Equal(t, []any{"a"}, first["tags"])
	require.Equal(t, "private", first["visibility"], "the spec's default is applied")
	require.Equal(t, "single", first["optIn"])
	require.Contains(t, first, "createdAt")
	require.NotContains(t, first, "CreatedAt", "no PascalCase leakage")

	status, body = ts.request(http.MethodGet, base+"/lists/"+id, nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "Newsletter", body["name"], "a single list is unwrapped")
	require.NotContains(t, body, "list")
}

func TestListDuplicateNameConflicts(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, _ := ts.request(http.MethodPost, base+"/lists", map[string]any{"name": "Dup"})
	require.Equal(t, http.StatusCreated, status)
	status, _ = ts.request(http.MethodPost, base+"/lists", map[string]any{"name": "Dup"})
	require.Equal(t, http.StatusConflict, status)
}

func TestListBlankNameRejected(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	status, _ := ts.request(http.MethodPost, "/t/"+slug+"/api/lists", map[string]any{"name": "  "})
	require.Equal(t, http.StatusUnprocessableEntity, status)
}

func TestSubscriberCRUD(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodPost, base+"/subscribers", map[string]any{
		"email": "pat@example.com", "name": "Pat",
		"attributes": map[string]any{"plan": "pro"},
	})
	require.Equal(t, http.StatusCreated, status)
	subID, _ := body["id"].(string)
	require.NotEmpty(t, subID)

	status, body = ts.request(http.MethodGet, base+"/subscribers?q=pat", nil)
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 1, body["total"])

	status, body = ts.request(http.MethodGet, base+"/subscribers/"+subID, nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "pat@example.com", body["email"], "a single subscriber is unwrapped and lowerCamelCase")

	status, _ = ts.request(http.MethodPut, base+"/subscribers/"+subID, map[string]any{
		"name": "Patricia", "state": "disabled",
	})
	require.Equal(t, http.StatusNoContent, status)

	status, _ = ts.request(http.MethodDelete, base+"/subscribers/"+subID, nil)
	require.Equal(t, http.StatusNoContent, status)
}

// TestSubscriberWireShape pins the contract shape the generated subscriber
// operations serve: the search page is an {items,total} envelope, a single
// subscriber comes back unwrapped, and memberships are lowerCamelCase. None of
// that was true of the hand-written handlers, which wrote {"subscribers":...}
// and serialized the untagged query view (and its PascalCase ListID) directly.
func TestSubscriberWireShape(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, list := ts.request(http.MethodPost, base+"/lists", map[string]any{"name": "L"})
	require.Equal(t, http.StatusCreated, status)
	listID, _ := list["id"].(string)
	require.NotEmpty(t, listID)

	status, created := ts.request(http.MethodPost, base+"/subscribers", map[string]any{
		"email": "a@example.com", "name": "A",
		"attributes": map[string]any{"city": "Belgrade"},
		"listIds":    []string{listID},
	})
	require.Equal(t, http.StatusCreated, status)
	subID, _ := created["id"].(string)
	require.NotEmpty(t, subID)

	status, one := ts.request(http.MethodGet, base+"/subscribers/"+subID, nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "a@example.com", one["email"], "single objects are unwrapped")
	require.NotContains(t, one, "subscriber")
	memberships, ok := one["memberships"].([]any)
	require.True(t, ok)
	require.Len(t, memberships, 1)
	m, _ := memberships[0].(map[string]any)
	require.Equal(t, listID, m["listId"], "memberships are lowerCamelCase")
	require.Equal(t, "unconfirmed", m["status"])

	status, page := ts.request(http.MethodGet, base+"/subscribers?q=a&limit=10&offset=0", nil)
	require.Equal(t, http.StatusOK, status)
	items, ok := page["items"].([]any)
	require.True(t, ok, "search envelope is {items,total}, not {subscribers,total}")
	require.Len(t, items, 1)
}

// TestSubscriberWireShapeNormalizesEmptyCollections pins
// subscriberViewToAPI's nil-to-empty normalization for a subscriber with
// neither custom attributes nor list memberships: the schema declares both
// required, and a nil Go value of either marshals to `null`, which the
// contract forbids. Every other subscriber test in this file creates one
// with at least one of each, so neither would catch a regression to null on
// its own.
func TestSubscriberWireShapeNormalizesEmptyCollections(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, created := ts.request(http.MethodPost, base+"/subscribers",
		map[string]any{"email": "bare@example.com"})
	require.Equal(t, http.StatusCreated, status)
	subID, _ := created["id"].(string)
	require.NotEmpty(t, subID)

	status, body := ts.request(http.MethodGet, base+"/subscribers/"+subID, nil)
	require.Equal(t, http.StatusOK, status)

	attrs, ok := body["attributes"].(map[string]any)
	require.True(t, ok, "attributes is {}, not null, when the subscriber has none")
	require.Empty(t, attrs)

	memberships, ok := body["memberships"].([]any)
	require.True(t, ok, "memberships is [], not null, when the subscriber belongs to no list")
	require.Empty(t, memberships)
}

func TestSubscriberDuplicateEmailConflicts(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, _ := ts.request(http.MethodPost, base+"/subscribers",
		map[string]any{"email": "dup@example.com"})
	require.Equal(t, http.StatusCreated, status)
	status, _ = ts.request(http.MethodPost, base+"/subscribers",
		map[string]any{"email": "dup@example.com"})
	require.Equal(t, http.StatusConflict, status)
}

func TestSubscriberInvalidEmailRejected(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	status, _ := ts.request(http.MethodPost, "/t/"+slug+"/api/subscribers",
		map[string]any{"email": "not-an-email"})
	require.Equal(t, http.StatusUnprocessableEntity, status)
}

func TestSubscriberListMembership(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	_, listBody := ts.request(http.MethodPost, base+"/lists", map[string]any{"name": "L"})
	listID := listBody["id"].(string)
	_, subBody := ts.request(http.MethodPost, base+"/subscribers",
		map[string]any{"email": "m@example.com"})
	subID := subBody["id"].(string)

	status, _ := ts.request(http.MethodPost, base+"/subscribers/"+subID+"/lists",
		map[string]any{"listId": listID})
	require.Equal(t, http.StatusNoContent, status)

	status, _ = ts.request(http.MethodPut, base+"/subscribers/"+subID+"/lists/"+listID,
		map[string]any{"status": "confirmed"})
	require.Equal(t, http.StatusNoContent, status)

	status, body := ts.request(http.MethodGet, base+"/subscribers/"+subID, nil)
	require.Equal(t, http.StatusOK, status)
	memberships, _ := body["memberships"].([]any)
	require.Len(t, memberships, 1)

	status, _ = ts.request(http.MethodDelete, base+"/subscribers/"+subID+"/lists/"+listID, nil)
	require.Equal(t, http.StatusNoContent, status)
}

// TestAddSubscriberToListChecksBodyListIDNotPath pins the one detail
// AddSubscriberToList gets right that its siblings (RemoveSubscriberFromList,
// ChangeSubscription) do not need to: the list id it authorizes against comes
// from the request body (req.Body.ListId), not the path — the path only ever
// carries the subscriber id here. A regression that checked req.Id instead
// would compile (both are strings) and would fail closed for the wrong
// reason on the negative case below, but it would also wrongly *deny* the
// positive case, since a subscriber id is never a valid list id. Both
// assertions together are what catch that flip; either alone would not.
func TestAddSubscriberToListChecksBodyListIDNotPath(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	member, memberEmail := ts.addMember(slug)
	ts.enterWorkspaceOn(member, slug)
	memberID := ts.workspaceUserID(slug, memberEmail)

	_, a := ts.request(http.MethodPost, base+"/lists", map[string]any{"name": "ListA"})
	listA := a["id"].(string)
	_, b := ts.request(http.MethodPost, base+"/lists", map[string]any{"name": "ListB"})
	listB := b["id"].(string)
	_, sub := ts.request(http.MethodPost, base+"/subscribers", map[string]any{"email": "s@example.com"})
	subID := sub["id"].(string)

	// The member gets a subscribers:manage role, but only on listA.
	_, role := ts.request(http.MethodPost, base+"/roles", map[string]any{
		"name": "SubscriberEditor", "permissions": []string{"subscribers:manage"},
	})
	roleID := role["id"].(string)
	status, _ := ts.request(http.MethodPut, base+"/users/"+memberID+"/lists/"+listA+"/role",
		map[string]any{"role_id": roleID})
	require.Equal(t, http.StatusNoContent, status)

	// Adding to listA (the granted list, named in the body) succeeds.
	status, _ = ts.do(member, http.MethodPost, base+"/subscribers/"+subID+"/lists",
		map[string]any{"listId": listA})
	require.Equal(t, http.StatusNoContent, status, "the per-list grant on listA authorizes adding to listA")

	// Adding to listB (named in the body, not granted) is forbidden — even
	// though the path parameter (the subscriber id) is identical to the call
	// above. Checking the path instead of the body would authorize this.
	status, _ = ts.do(member, http.MethodPost, base+"/subscribers/"+subID+"/lists",
		map[string]any{"listId": listB})
	require.Equal(t, http.StatusForbidden, status, "the per-list grant on listA does not widen to listB")
}

func TestSegmentQuery(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	for _, sub := range []map[string]any{
		{"email": "pro@example.com", "attributes": map[string]any{"plan": "pro"}},
		{"email": "free@example.com", "attributes": map[string]any{"plan": "free"}},
	} {
		status, _ := ts.request(http.MethodPost, base+"/subscribers", sub)
		require.Equal(t, http.StatusCreated, status)
	}

	query := map[string]any{
		"segment": map[string]any{
			"attr": map[string]any{"key": "plan", "op": "eq", "value": "pro"},
		},
	}
	status, body := ts.request(http.MethodPost, base+"/subscribers/query", query)
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 1, body["total"])
	subs, _ := body["items"].([]any)
	require.Len(t, subs, 1)
	require.Equal(t, "pro@example.com", subs[0].(map[string]any)["email"])

	status, body = ts.request(http.MethodPost, base+"/subscribers/query/count", query)
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 1, body["total"])
}

// TestSegmentQueryWireShape pins the same shape as TestSubscriberWireShape for
// the segment-query path: the count envelope is {total}, and a lowerCamelCase
// segment tree round-trips through segmentNodeFromAPI and the domain's own
// validation.
func TestSegmentQueryWireShape(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, _ := ts.request(http.MethodPost, base+"/subscribers",
		map[string]any{"email": "b@example.com", "name": "B"})
	require.Equal(t, http.StatusCreated, status)

	status, res := ts.request(http.MethodPost, base+"/subscribers/query/count", map[string]any{
		"segment": map[string]any{
			"conj": "and",
			"children": []any{
				map[string]any{
					"field": map[string]any{"field": "email", "op": "contains", "value": "b@"},
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 1, res["total"], "segment nodes are lowerCamelCase")
}

// TestSegmentQueryWireShapeNestedGroups goes one level deeper than
// TestSegmentQueryWireShape: the outer group's one child is itself a group,
// not a leaf. segmentNodeFromAPI recurses by calling itself on every element
// of Children, so this is the shape that would catch a future hand-rolled,
// non-recursive expansion (one that flattened only the first level).
func TestSegmentQueryWireShapeNestedGroups(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	for _, email := range []string{"b@example.com", "c@example.com", "d@example.com"} {
		status, _ := ts.request(http.MethodPost, base+"/subscribers", map[string]any{"email": email})
		require.Equal(t, http.StatusCreated, status)
	}

	// Outer AND has one child: an inner OR group of two field leaves.
	status, res := ts.request(http.MethodPost, base+"/subscribers/query", map[string]any{
		"segment": map[string]any{
			"conj": "and",
			"children": []any{
				map[string]any{
					"conj": "or",
					"children": []any{
						map[string]any{"field": map[string]any{"field": "email", "op": "eq", "value": "b@example.com"}},
						map[string]any{"field": map[string]any{"field": "email", "op": "eq", "value": "c@example.com"}},
					},
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, status)
	items, ok := res["items"].([]any)
	require.True(t, ok)
	got := map[string]bool{}
	for _, it := range items {
		got[it.(map[string]any)["email"].(string)] = true
	}
	require.Equal(t, map[string]bool{"b@example.com": true, "c@example.com": true}, got,
		"the inner OR group's own children round-trip: both its leaves match, and d@example.com (outside both groups) does not")
}

// TestSegmentQueryMalformedRejected drives an unknown segment field ("phone")
// through NewSegment's own validation (internal/audience/domain/segment.go),
// not the wire-format check — the body is well-formed lowerCamelCase JSON, so
// the 422 below is segmentNodeFromAPI round-tripping into a domain.Node that
// the domain itself then rejects.
func TestSegmentQueryMalformedRejected(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	status, _ := ts.request(http.MethodPost, "/t/"+slug+"/api/subscribers/query", map[string]any{
		"segment": map[string]any{
			"field": map[string]any{"field": "phone", "op": "eq", "value": "x"},
		},
	})
	require.Equal(t, http.StatusUnprocessableEntity, status)
}

func TestSegmentDrivenExport(t *testing.T) {
	ts := newTestServer(t)
	ts.startWorker()
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	for _, sub := range []map[string]any{
		{"email": "keep@example.com", "attributes": map[string]any{"plan": "pro"}},
		{"email": "drop@example.com", "attributes": map[string]any{"plan": "free"}},
	} {
		status, _ := ts.request(http.MethodPost, base+"/subscribers", sub)
		require.Equal(t, http.StatusCreated, status)
	}

	status, body := ts.request(http.MethodPost, base+"/export", map[string]any{
		"selection": "segment",
		"segment": map[string]any{
			"Attr": map[string]any{"Key": "plan", "Op": "eq", "Value": "pro"},
		},
	})
	require.Equal(t, http.StatusAccepted, status)
	jobID := body["job_id"].(string)

	job := ts.waitForJobStatus(base, jobID)
	require.Equal(t, "completed", job["Status"])
	require.EqualValues(t, 1, job["RowCount"])

	req, err := http.NewRequest(http.MethodGet, ts.URL+base+"/jobs/"+jobID+"/download", nil)
	require.NoError(t, err)
	resp, err := ts.client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	csv, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(csv), "keep@example.com")
	require.NotContains(t, string(csv), "drop@example.com")
}

func TestAudienceIsolatedAcrossTenants(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slugA := ts.createTenant()
	slugB := ts.createTenant()
	ts.enterWorkspace(slugA)
	ts.enterWorkspace(slugB)

	status, body := ts.request(http.MethodPost, "/t/"+slugA+"/api/lists",
		map[string]any{"name": "A-only"})
	require.Equal(t, http.StatusCreated, status)
	listID := body["id"].(string)

	// The same user, in tenant B, cannot see tenant A's list.
	status, _ = ts.request(http.MethodGet, "/t/"+slugB+"/api/lists/"+listID, nil)
	require.Equal(t, http.StatusNotFound, status)

	status, body = ts.request(http.MethodGet, "/t/"+slugB+"/api/lists", nil)
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 0, body["total"], "tenant B sees none of tenant A's lists")
}
