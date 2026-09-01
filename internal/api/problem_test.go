package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// problemFor drives writeProblem through a recorder and decodes the body.
func problemFor(t *testing.T, status int, code, detail string, fields []fieldError) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t/acme/api/lists", nil)
	req = req.WithContext(withRequestIDForTest(req.Context(), "req-123"))
	writeProblem(rec, req, status, code, detail, fields)
	var body map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	return rec, body
}

func TestWriteProblemShape(t *testing.T) {
	t.Parallel()
	rec, body := problemFor(t, http.StatusNotFound, "list_not_found", "no such list", nil)

	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "https://nvelope.dev/probs/list_not_found", body["type"])
	require.Equal(t, "List not found", body["title"])
	require.EqualValues(t, 404, body["status"])
	require.Equal(t, "no such list", body["detail"])
	require.Equal(t, "/t/acme/api/lists", body["instance"])
	require.Equal(t, "list_not_found", body["code"])
	require.Equal(t, "req-123", body["requestId"])
	require.NotContains(t, body, "errors", "absent when no field errors")
}

func TestWriteProblemFieldErrors(t *testing.T) {
	t.Parallel()
	_, body := problemFor(t, http.StatusUnprocessableEntity, "validation_failed", "request is invalid",
		[]fieldError{{Field: "/name", Code: "required", Detail: "name is required"}})

	errs, ok := body["errors"].([]any)
	require.True(t, ok, "errors is an array")
	require.Len(t, errs, 1)
	first, _ := errs[0].(map[string]any)
	require.Equal(t, "/name", first["field"])
	require.Equal(t, "required", first["code"])
	require.Equal(t, "name is required", first["detail"])
}

func TestFailRendersProblemPerCategory(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"incorrect input", apperr.NewIncorrectInput("validation_failed", "bad"), 422, "validation_failed"},
		{"conflict", apperr.NewConflict("email_taken", "taken"), 409, "email_taken"},
		{"not found", apperr.NewNotFound("list_not_found", "gone"), 404, "list_not_found"},
		{"authorization", apperr.NewAuthorization("unauthenticated", "nope"), 401, "unauthenticated"},
		{"forbidden", apperr.NewForbidden("forbidden-lists-manage", "nope"), 403, "forbidden-lists-manage"},
		{"slug override", apperr.NewUnknown("payment_failed", "declined"), 402, "payment_failed"},
		{"unknown", apperr.NewUnknown("weird", "oops"), 500, "internal_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &Server{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/t/acme/api/lists", nil)
			s.fail(rec, req, "op", tc.err)

			var body map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
			require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			require.Equal(t, tc.status, rec.Code)
			require.Equal(t, tc.code, body["code"])
			require.Equal(t, "https://nvelope.dev/probs/"+tc.code, body["type"])
			require.NotEmpty(t, body["title"])
		})
	}
}
