package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/api/openapitenant"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestRequirePermWithoutPrincipal(t *testing.T) {
	t.Parallel()
	s := &Server{}
	_, err := s.requirePerm(context.Background(), iamdomain.PermListsGet)
	require.Error(t, err)

	rec := httptest.NewRecorder()
	s.fail(rec, httptest.NewRequest(http.MethodGet, "/t/acme/api/lists", nil), "authorize", err)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Equal(t, "unauthenticated", body["code"])
}

// TestRequirePermDeniesAndGrants covers the two outcomes that matter once a
// principal is present: a missing permission is a 403 Problem carrying the
// permission's own slug, and a held permission returns the principal.
func TestRequirePermDeniesAndGrants(t *testing.T) {
	t.Parallel()
	s := &Server{}
	ctx := context.WithValue(context.Background(), principalCtxKey,
		iamdomain.NewPrincipal(iamdomain.PrincipalSession, "tenant-1", "user-1",
			[]iamdomain.Permission{iamdomain.PermListsGet},
			map[string][]iamdomain.Permission{"list-1": {iamdomain.PermListsManage}}))

	_, err := s.requirePerm(ctx, iamdomain.PermListsManage)
	require.Error(t, err)
	rec := httptest.NewRecorder()
	s.fail(rec, httptest.NewRequest(http.MethodPut, "/t/acme/api/lists/1", nil), "authorize", err)
	require.Equal(t, http.StatusForbidden, rec.Code)

	p, err := s.requirePerm(ctx, iamdomain.PermListsGet)
	require.NoError(t, err)
	require.Equal(t, "user-1", p.ActorID())

	// requireListPerm additionally honours the per-list grant, so the same
	// permission that was denied tenant-wide is allowed on list-1.
	_, err = s.requireListPerm(ctx, iamdomain.PermListsManage, "list-2")
	require.Error(t, err)
	p, err = s.requireListPerm(ctx, iamdomain.PermListsManage, "list-1")
	require.NoError(t, err)
	require.Equal(t, "user-1", p.ActorID())

	// With no principal at all it is a 401, not a 403 — the same distinction
	// the ResponseWriter-based helpers draw.
	_, err = s.requireListPerm(context.Background(), iamdomain.PermListsManage, "list-1")
	require.Error(t, err)
	rec = httptest.NewRecorder()
	s.fail(rec, httptest.NewRequest(http.MethodPut, "/t/acme/api/lists/1", nil), "authorize", err)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestValidationMiddlewareRejectsUnknownField proves that additionalProperties:
// false in the spec preserves today's DisallowUnknownFields behaviour, and that
// the rejection is rendered as a problem document with a populated errors array.
func TestValidationMiddlewareRejectsUnknownField(t *testing.T) {
	t.Parallel()

	srv := newTestServerForWiring(t)

	req := httptest.NewRequest(http.MethodPost, "/t/acme/api/lists",
		strings.NewReader(`{"name":"ok","bogus":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, problemContentType, rec.Header().Get("Content-Type"))

	var doc struct {
		Code   string       `json:"code"`
		Errors []fieldError `json:"errors"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Equal(t, "validation_failed", doc.Code)
	require.Len(t, doc.Errors, 1, "errors array is populated")

	// additionalProperties: false is a constraint on the *object*, not on any
	// one property, so kin-openapi reports it against the body root: an empty
	// JSON Pointer and the "properties" keyword. The SPA must therefore not
	// branch on `field` to locate a 422 — an unknown-property rejection is
	// unlocatable by construction, and all a form can say is that the payload
	// carried a field the contract does not define.
	//
	// These two values are kin-openapi's, not ours: this is a characterization
	// test. A library bump that renamed the keyword would fail it loudly here,
	// which is the point — the wire contract would have changed under us.
	require.Equal(t, "", doc.Errors[0].Field,
		"an unknown property is reported against the object, not the property")
	require.Equal(t, "properties", doc.Errors[0].Code)
	require.NotEmpty(t, doc.Errors[0].Detail)
}

// TestGeneratedBinderRejectionLocatesTheParameter pins the taxonomy for a
// parameter the generated wrapper cannot bind at all. The wrapper runs before
// the request validator, so this rejection never reaches kin-openapi — but a
// caller cannot see that and must not have to: it renders as the same
// `validation_failed` document, with the same populated errors array, as the
// schema violation TestGeneratedMountDoesNotEnforceDeclaredSecurity asserts.
//
// Before this was reconciled the two produced different codes (`invalid_request`
// vs `validation_failed`) for one class of mistake, and the binder path carried
// no errors array at all, so a form could not tell which parameter was bad.
func TestGeneratedBinderRejectionLocatesTheParameter(t *testing.T) {
	t.Parallel()

	srv := newTestServerForWiring(t)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/acme/api/lists?limit=not-a-number", nil))

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, problemContentType, rec.Header().Get("Content-Type"))

	var doc struct {
		Code   string       `json:"code"`
		Errors []fieldError `json:"errors"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Equal(t, "validation_failed", doc.Code,
		"the same code the validator uses for an out-of-range value")
	require.Len(t, doc.Errors, 1)
	require.Equal(t, "limit", doc.Errors[0].Field)
	require.Equal(t, "invalid", doc.Errors[0].Code)
	require.Contains(t, doc.Errors[0].Detail, "not-a-number",
		"the detail echoes the caller's own input, not server internals")
}

// TestParameterFieldErrorClassifies covers parameterFieldError directly,
// because two of its three branches cannot be driven over HTTP against this
// document: RequiredParamError needs a required query parameter (the tenant
// document has none — a missing *path* parameter means the route did not
// match), and the fallthrough needs a binder error type the generated code
// never constructs here.
func TestParameterFieldErrorClassifies(t *testing.T) {
	t.Parallel()

	fe, ok := parameterFieldError(&openapitenant.InvalidParamFormatError{
		ParamName: "offset", Err: errors.New("strconv.ParseInt: invalid syntax"),
	})
	require.True(t, ok)
	require.Equal(t, "offset", fe.Field)
	require.Equal(t, "invalid", fe.Code)
	require.Equal(t, "strconv.ParseInt: invalid syntax", fe.Detail,
		"the wrapper's redundant 'Invalid format for parameter x:' prefix is dropped")

	fe, ok = parameterFieldError(&openapitenant.RequiredParamError{ParamName: "q"})
	require.True(t, ok)
	require.Equal(t, "q", fe.Field)
	require.Equal(t, "required", fe.Code)
	require.NotEmpty(t, fe.Detail)

	// An unrecognised error keeps the generic 400 path rather than claiming a
	// field it cannot name.
	_, ok = parameterFieldError(errors.New("something else entirely"))
	require.False(t, ok)
}

// TestGeneratedMountDoesNotEnforceDeclaredSecurity is the end-to-end companion
// to TestValidatorDoesNotEnforceDeclaredSecurity below. That test hand-builds a
// router around a probe operation because when it was written the spec's paths
// were all parked; it therefore proves the validator *factory* is configured
// correctly, but would stay green if mountGeneratedTenantAPI stopped calling
// that factory at all.
//
// POST/GET /lists are now real generated routes, so this drives the production
// mount with no nv_workspace cookie and asserts the request is not refused for
// a security reason.
//
// The discriminator is the rejection's *kind*, not its status: kin-openapi
// validates security requirements before any parameter or body (openapi3filter
// ValidateRequest) and, with MultiError off, returns on the first failure. A
// `limit` over the schema's maximum is a violation only the validator can
// report — the generated wrapper binds the parameter but never range-checks it
// — so this 422 is reachable only once the security check has passed. A
// regression yields 401 "unauthenticated" instead.
func TestGeneratedMountDoesNotEnforceDeclaredSecurity(t *testing.T) {
	t.Parallel()

	srv := newTestServerForWiring(t)

	// No nv_workspace cookie: the security requirement tenant.yaml declares is
	// unsatisfiable here, and must not be what decides the outcome.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/acme/api/lists?limit=99999", nil))

	require.NotEqual(t, http.StatusUnauthorized, rec.Code,
		"the validator must not reject a generated route on security grounds")
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	var doc struct {
		Code   string       `json:"code"`
		Errors []fieldError `json:"errors"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Equal(t, "validation_failed", doc.Code,
		"parameter validation ran, which only happens after the security check passes")
	require.Len(t, doc.Errors, 1)
	require.Equal(t, "limit", doc.Errors[0].Field)
	require.Equal(t, "maximum", doc.Errors[0].Code)
}

// TestValidatorDoesNotEnforceDeclaredSecurity is the guard on the single
// riskiest decision in this wiring. tenant.yaml declares a document-level
// `security` requirement for the nv_workspace cookie, and kin-openapi enforces
// declared security: without an AuthenticationFunc it rejects *every* request
// with 401 "security requirements failed: missing AuthenticationFunc",
// regardless of whether the cookie is present. Authentication is chi
// middleware's job and has already run by the time the validator sees the
// request, so openapiRequestValidator installs NoopAuthenticationFunc.
//
// If that ever regresses, every generated operation 401s and every later task
// fails in a confusing way, so this test drives a request through a validator
// built exactly like production's and asserts it reaches the handler.
//
// It also carries the assertion the deleted openapi_spike_test.go used to make:
// a path the router serves but the document does not declare is rejected, which
// is what proves the pass above is a real match against the templated
// /t/{slug}/api server base rather than a validator that waves everything
// through.
func TestValidatorDoesNotEnforceDeclaredSecurity(t *testing.T) {
	t.Parallel()

	spec, err := openapitenant.GetSpec()
	require.NoError(t, err)
	require.NotEmpty(t, spec.Security,
		"the document declares a security requirement — that is the condition this test guards")
	require.NotNil(t, spec.Components.SecuritySchemes["WorkspaceSession"])

	// The spec's own paths are parked (see api/openapi/tenant.yaml), so the
	// probe supplies the one operation this test needs. Everything else —
	// the servers: base, the security block, the validator options — is the
	// real thing.
	if spec.Paths == nil {
		spec.Paths = openapi3.NewPaths()
	}
	spec.Paths.Set("/wiring-probe", &openapi3.PathItem{
		Get: &openapi3.Operation{
			OperationID: "wiringProbe",
			Responses:   openapi3.NewResponses(),
		},
	})

	s := &Server{}
	validator := s.openapiRequestValidator(spec)

	r := chi.NewRouter()
	r.Use(s.requestID)
	r.Route("/t/{slug}/api", func(tr chi.Router) {
		reached := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
		tr.With(validator).Get("/wiring-probe", reached)
		tr.With(validator).Get("/not-in-the-spec", reached)
	})

	// No nv_workspace cookie: the validator must not be the thing that decides.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/acme/api/wiring-probe", nil))
	require.Equal(t, http.StatusNoContent, rec.Code,
		"the request reached the handler instead of being 401'd by security validation")

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/acme/api/not-in-the-spec", nil))
	require.Equal(t, http.StatusNotFound, rec.Code,
		"the validator rejected a path the router serves but the document does not declare")
	require.Equal(t, problemContentType, rec.Header().Get("Content-Type"),
		"even a routing miss is rendered as a problem document")
}

// TestValidationFailureRendersAsProblem covers the renderer that
// TestValidationMiddlewareRejectsUnknownField cannot reach while the spec's
// paths are parked: a schema violation must come back as a problem document
// whose errors array locates the offending property.
//
// It drives a real request through the production validator so the field error
// is built from a genuine *openapi3.SchemaError — a hand-constructed one has an
// empty JSONPointer (the path is tracked in unexported state), which would make
// the assertion on "/name" vacuous.
func TestValidationFailureRendersAsProblem(t *testing.T) {
	t.Parallel()

	spec, err := openapitenant.GetSpec()
	require.NoError(t, err)
	if spec.Paths == nil {
		spec.Paths = openapi3.NewPaths()
	}
	body := openapi3.NewObjectSchema().
		WithProperty("name", openapi3.NewStringSchema().WithMinLength(1))
	body.Required = []string{"name"}
	spec.Paths.Set("/wiring-probe", &openapi3.PathItem{
		Post: &openapi3.Operation{
			OperationID: "wiringProbe",
			RequestBody: &openapi3.RequestBodyRef{
				Value: openapi3.NewRequestBody().WithJSONSchema(body),
			},
			Responses: openapi3.NewResponses(),
		},
	})

	s := &Server{}
	r := chi.NewRouter()
	r.Use(s.requestID)
	r.Route("/t/{slug}/api", func(tr chi.Router) {
		tr.With(s.openapiRequestValidator(spec)).Post("/wiring-probe",
			func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	})

	req := httptest.NewRequest(http.MethodPost, "/t/acme/api/wiring-probe",
		strings.NewReader(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, problemContentType, rec.Header().Get("Content-Type"))

	var doc struct {
		Code   string       `json:"code"`
		Errors []fieldError `json:"errors"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Equal(t, "validation_failed", doc.Code)
	require.Len(t, doc.Errors, 1,
		"kin-openapi reports one violation at a time unless MultiError is set")
	require.Equal(t, "/name", doc.Errors[0].Field)
	require.Equal(t, "minLength", doc.Errors[0].Code)
	require.NotEmpty(t, doc.Errors[0].Detail)
}

// newTestServerForWiring mounts the generated tenant API with the request
// validator, bypassing authentication — this test exercises validation and
// error rendering only.
func newTestServerForWiring(t *testing.T) http.Handler {
	t.Helper()
	s := &Server{}
	r := chi.NewRouter()
	r.Use(s.requestID)
	r.Route("/t/{slug}/api", func(tr chi.Router) {
		s.mountGeneratedTenantAPI(tr)
	})
	return r
}
